// Package store implements PostgreSQL persistence for the optional features
// that need it: ownership-verification tokens and administrator-managed
// domain lists. Basic validation never touches the database.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/verification"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Postgres implements verification.Store and disposable.AdminStore.
type Postgres struct {
	pool *pgxpool.Pool
	stop context.CancelFunc
	done chan struct{} // closed once migrations succeeded or retrying stopped
}

var (
	_ verification.Store    = (*Postgres)(nil)
	_ disposable.AdminStore = (*Postgres)(nil)
)

// Open creates a connection pool for url and applies pending migrations.
//
// Only an invalid URL is an error. If the database is unreachable, Open
// logs the problem and keeps retrying the migrations in the background, so a
// database outage at startup degrades the database-backed features instead
// of taking the whole service (and plain validation) down.
func Open(ctx context.Context, url string, log *slog.Logger) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if cfg.MaxConns == 0 || cfg.MaxConns > 10 {
		cfg.MaxConns = 10
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	bg, cancel := context.WithCancel(context.Background())
	p := &Postgres{pool: pool, stop: cancel, done: make(chan struct{})}

	mctx, mcancel := context.WithTimeout(ctx, 10*time.Second)
	err = p.migrate(mctx)
	mcancel()
	if err == nil {
		close(p.done)
		return p, nil
	}
	log.Error("database unavailable; validation continues and database features will recover when it is reachable", "error", err)
	go p.retryMigrations(bg, log)
	return p, nil
}

func (p *Postgres) retryMigrations(ctx context.Context, log *slog.Logger) {
	defer close(p.done)
	delay := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := p.migrate(mctx)
		cancel()
		if err == nil {
			log.Info("database reachable; migrations applied")
			return
		}
		delay = min(delay*2, 30*time.Second)
	}
}

// Close stops background work and releases the connection pool.
func (p *Postgres) Close() {
	p.stop()
	<-p.done
	p.pool.Close()
}

// Ping checks connectivity.
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// migrationLockID is an arbitrary constant identifying Validly's migration
// advisory lock.
const migrationLockID = 0x76616c69646c79 // "validly"

func (p *Postgres) migrate(ctx context.Context) error {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return err
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockID) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS validly_schema_migrations (
		version    integer     PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		name := strings.TrimPrefix(f, "migrations/")
		version, err := strconv.Atoi(name[:strings.IndexByte(name, '_')])
		if err != nil {
			return fmt.Errorf("migration %s: bad version", name)
		}
		var applied bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM validly_schema_migrations WHERE version = $1)", version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO validly_schema_migrations (version) VALUES ($1)", version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func (p *Postgres) CreateToken(ctx context.Context, t verification.Token, maxRecent int, since time.Time) (bool, error) {
	stored := false
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		// Serialize issuance per address so concurrent requests cannot
		// exceed the limit.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(encode($1::bytea, 'hex'), 0))", t.EmailHash[:]); err != nil {
			return err
		}
		var recent int
		if err := tx.QueryRow(ctx,
			"SELECT count(*) FROM verification_tokens WHERE email_hash = $1 AND created_at >= $2",
			t.EmailHash[:], since).Scan(&recent); err != nil {
			return err
		}
		if recent >= maxRecent {
			return nil
		}
		_, err := tx.Exec(ctx,
			"INSERT INTO verification_tokens (token_hash, email_hash, email, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)",
			t.Hash[:], t.EmailHash[:], t.Email, t.CreatedAt, t.ExpiresAt)
		stored = err == nil
		return err
	})
	return stored, err
}

func (p *Postgres) ConsumeToken(ctx context.Context, hash verification.Hash, now time.Time) (string, error) {
	var email string
	err := p.pool.QueryRow(ctx,
		`UPDATE verification_tokens SET used_at = $2
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
		 RETURNING email`,
		hash[:], now).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", verification.ErrInvalidToken
	}
	return email, err
}

func (p *Postgres) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx, "DELETE FROM verification_tokens WHERE expires_at < $1", before)
	return tag.RowsAffected(), err
}

func (p *Postgres) ListEntries(ctx context.Context) ([]disposable.Entry, error) {
	rows, err := p.pool.Query(ctx, "SELECT list, domain, created_at FROM disposable_list_entries ORDER BY list, domain")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (disposable.Entry, error) {
		var e disposable.Entry
		err := r.Scan(&e.List, &e.Domain, &e.CreatedAt)
		return e, err
	})
}

func (p *Postgres) AddEntry(ctx context.Context, list disposable.ListName, domain string) error {
	_, err := p.pool.Exec(ctx,
		"INSERT INTO disposable_list_entries (list, domain) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		string(list), domain)
	return err
}

func (p *Postgres) RemoveEntry(ctx context.Context, list disposable.ListName, domain string) (bool, error) {
	tag, err := p.pool.Exec(ctx, "DELETE FROM disposable_list_entries WHERE list = $1 AND domain = $2", string(list), domain)
	return tag.RowsAffected() > 0, err
}
