package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/verification"
)

// These tests need a disposable PostgreSQL database, for example:
//
//	docker compose up -d postgres
//	VALIDLY_TEST_DATABASE_URL=postgres://validly:validly@localhost:5432/validly_test?sslmode=disable go test ./internal/store/
func open(t *testing.T) *Postgres {
	t.Helper()
	url := os.Getenv("VALIDLY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("VALIDLY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	p, err := Open(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if _, err := p.pool.Exec(ctx, "TRUNCATE verification_tokens, disposable_list_entries"); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMigrationsAreIdempotent(t *testing.T) {
	p := open(t)
	if err := p.migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func token(email string, n byte, now time.Time) verification.Token {
	return verification.Token{
		Hash:      sha256.Sum256([]byte{n}),
		EmailHash: sha256.Sum256([]byte(email)),
		Email:     email,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Minute),
	}
}

func TestTokens(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	ok, err := p.CreateToken(ctx, token("a@example.com", 1, now), 2, now.Add(-time.Hour))
	if err != nil || !ok {
		t.Fatalf("create: %v %v", ok, err)
	}
	email, err := p.ConsumeToken(ctx, sha256.Sum256([]byte{1}), now)
	if err != nil || email != "a@example.com" {
		t.Fatalf("consume: %q %v", email, err)
	}
	if _, err := p.ConsumeToken(ctx, sha256.Sum256([]byte{1}), now); !errors.Is(err, verification.ErrInvalidToken) {
		t.Fatalf("replay: %v", err)
	}
	if _, err := p.ConsumeToken(ctx, sha256.Sum256([]byte{99}), now); !errors.Is(err, verification.ErrInvalidToken) {
		t.Fatalf("unknown: %v", err)
	}

	_, _ = p.CreateToken(ctx, token("a@example.com", 2, now), 2, now.Add(-time.Hour))
	if _, err := p.ConsumeToken(ctx, sha256.Sum256([]byte{2}), now.Add(time.Minute)); !errors.Is(err, verification.ErrInvalidToken) {
		t.Fatalf("expired: %v", err)
	}

	// Two tokens exist for the address now, so a third is refused.
	ok, err = p.CreateToken(ctx, token("a@example.com", 3, now), 2, now.Add(-time.Hour))
	if err != nil || ok {
		t.Fatalf("limit: %v %v", ok, err)
	}

	n, err := p.DeleteExpired(ctx, now.Add(2*time.Minute))
	if err != nil || n != 2 {
		t.Fatalf("delete expired: %d %v", n, err)
	}
}

func TestConcurrentIssuanceRespectsLimit(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	now := time.Now().UTC()
	var stored atomic.Int64
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			ok, err := p.CreateToken(ctx, token("race@example.com", byte(100+i), now), 3, now.Add(-time.Hour))
			if err != nil {
				t.Error(err)
			}
			if ok {
				stored.Add(1)
			}
		})
	}
	wg.Wait()
	if n := stored.Load(); n != 3 {
		t.Fatalf("stored %d tokens, limit is 3", n)
	}
}

func TestConcurrentConsumeOnlyOneWins(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, _ = p.CreateToken(ctx, token("a@example.com", 7, now), 5, now.Add(-time.Hour))
	var wins atomic.Int64
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := p.ConsumeToken(ctx, sha256.Sum256([]byte{7}), now); err == nil {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d consumers won", wins.Load())
	}
}

func TestDomainLists(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	if err := p.AddEntry(ctx, disposable.Blocklist, "bad.example"); err != nil {
		t.Fatal(err)
	}
	if err := p.AddEntry(ctx, disposable.Blocklist, "bad.example"); err != nil {
		t.Fatal("duplicate insert should be a no-op:", err)
	}
	if err := p.AddEntry(ctx, disposable.Allowlist, "good.example"); err != nil {
		t.Fatal(err)
	}
	entries, err := p.ListEntries(ctx)
	if err != nil || len(entries) != 2 || entries[0].List != disposable.Allowlist || entries[1].Domain != "bad.example" {
		t.Fatalf("%+v %v", entries, err)
	}
	removed, err := p.RemoveEntry(ctx, disposable.Blocklist, "bad.example")
	if err != nil || !removed {
		t.Fatalf("%v %v", removed, err)
	}
	removed, _ = p.RemoveEntry(ctx, disposable.Blocklist, "bad.example")
	if removed {
		t.Fatal("removed twice")
	}
}

func TestOpenToleratesUnreachableDatabase(t *testing.T) {
	start := time.Now()
	p, err := Open(context.Background(), "postgres://validly@127.0.0.1:1/validly?sslmode=disable&connect_timeout=1", nil)
	if err != nil {
		t.Fatalf("an unreachable database must not fail startup: %v", err)
	}
	if err := p.Ping(context.Background()); err == nil {
		t.Fatal("expected ping to fail")
	}
	p.Close()
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("took %v", d)
	}
	if _, err := Open(context.Background(), "not a url", nil); err == nil {
		t.Fatal("expected an error for an invalid URL")
	}
}
