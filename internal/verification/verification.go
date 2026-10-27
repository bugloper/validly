// Package verification implements email ownership verification: Validly
// emails a single-use link to an address, and whoever presents the token from
// that link has shown they can read mail sent to it.
//
// Security properties:
//   - tokens are 256-bit values from crypto/rand; only their SHA-256 hash is
//     stored, so a database leak does not reveal usable tokens;
//   - tokens expire and are consumed atomically, so each works at most once;
//   - issuance is rate-limited per address, and RequestVerification behaves
//     identically whether a message was sent, suppressed by the rate limit,
//     or failed, so callers cannot learn anything about an address from it;
//   - raw tokens and full addresses are never logged.
package verification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/validly/validly/internal/syntax"
)

var (
	// ErrInvalidToken covers unknown, expired and already-used tokens alike.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrInvalidEmail is returned when the address fails syntax validation.
	ErrInvalidEmail = errors.New("invalid email address")
)

// Hash is a SHA-256 digest.
type Hash [sha256.Size]byte

// Token is a stored verification token.
type Token struct {
	Hash      Hash
	EmailHash Hash
	Email     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Store persists tokens.
type Store interface {
	// CreateToken stores t unless maxRecent tokens were already created for
	// t.EmailHash since `since`; it reports whether the token was stored.
	CreateToken(ctx context.Context, t Token, maxRecent int, since time.Time) (bool, error)
	// ConsumeToken atomically marks the token used and returns its email. It
	// returns ErrInvalidToken if the token is unknown, expired or used.
	ConsumeToken(ctx context.Context, hash Hash, now time.Time) (string, error)
	// DeleteExpired removes tokens that expired before `before`.
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// Message is an outgoing email.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Mailer delivers messages.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// Config configures a Service.
type Config struct {
	TokenTTL time.Duration
	// MaxSends tokens may be issued per address per SendWindow.
	MaxSends   int
	SendWindow time.Duration
	// LinkTemplate is the URL emailed to the user; "{token}" is replaced with
	// the token. It is operator configuration, never caller input, so the
	// service cannot be used to send links to arbitrary sites.
	LinkTemplate string
	// QueueSize bounds pending sends; requests beyond it are dropped.
	QueueSize int
	Workers   int
}

// Observer receives verification events for metrics.
type Observer interface {
	VerificationEvent(event string)
}

type nopObserver struct{}

func (nopObserver) VerificationEvent(string) {}

// Service issues and confirms tokens.
type Service struct {
	store    Store
	mailer   Mailer
	cfg      Config
	log      *slog.Logger
	observer Observer
	queue    chan string
	now      func() time.Time
}

// NewService returns a Service. Call Run to start delivery workers.
func NewService(store Store, mailer Mailer, cfg Config, obs Observer, log *slog.Logger) *Service {
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = 30 * time.Minute
	}
	if cfg.MaxSends <= 0 {
		cfg.MaxSends = 3
	}
	if cfg.SendWindow <= 0 {
		cfg.SendWindow = time.Hour
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if obs == nil {
		obs = nopObserver{}
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		store:    store,
		mailer:   mailer,
		cfg:      cfg,
		log:      log,
		observer: obs,
		queue:    make(chan string, cfg.QueueSize),
		now:      time.Now,
	}
}

// RequestVerification queues a verification email to address. It returns
// ErrInvalidEmail for malformed input and nil otherwise, regardless of
// whether a message is eventually sent.
func (s *Service) RequestVerification(_ context.Context, email string) error {
	if !syntax.Parse(email).Valid {
		return ErrInvalidEmail
	}
	select {
	case s.queue <- email:
		s.observer.VerificationEvent("queued")
	default:
		s.observer.VerificationEvent("dropped")
		s.log.Warn("verification queue full; request dropped")
	}
	return nil
}

// Confirm consumes token and returns the address it was issued for.
func (s *Service) Confirm(ctx context.Context, token string) (string, error) {
	if !wellFormedToken(token) {
		s.observer.VerificationEvent("confirm_invalid")
		return "", ErrInvalidToken
	}
	email, err := s.store.ConsumeToken(ctx, hashToken(token), s.now())
	if errors.Is(err, ErrInvalidToken) {
		s.observer.VerificationEvent("confirm_invalid")
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	s.observer.VerificationEvent("confirmed")
	return email, nil
}

// Run processes the send queue and periodically deletes expired tokens until
// ctx is cancelled. Queued requests still pending at shutdown are dropped.
func (s *Service) Run(ctx context.Context) {
	for range s.cfg.Workers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case email := <-s.queue:
					s.process(ctx, email)
				}
			}
		}()
	}
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Keep expired tokens for a day so support can reason about
			// recent failures, then remove them along with the addresses.
			n, err := s.store.DeleteExpired(ctx, s.now().Add(-24*time.Hour))
			if err != nil {
				s.log.WarnContext(ctx, "deleting expired verification tokens failed", "error", err)
			} else if n > 0 {
				s.log.InfoContext(ctx, "deleted expired verification tokens", "count", n)
			}
		}
	}
}

func (s *Service) process(ctx context.Context, email string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, hash, err := newToken()
	if err != nil {
		s.log.ErrorContext(ctx, "generating verification token failed", "error", err)
		return
	}
	now := s.now()
	stored, err := s.store.CreateToken(ctx, Token{
		Hash:      hash,
		EmailHash: hashEmail(email),
		Email:     email,
		CreatedAt: now,
		ExpiresAt: now.Add(s.cfg.TokenTTL),
	}, s.cfg.MaxSends, now.Add(-s.cfg.SendWindow))
	if err != nil {
		s.observer.VerificationEvent("store_error")
		s.log.ErrorContext(ctx, "storing verification token failed", "error", err)
		return
	}
	if !stored {
		s.observer.VerificationEvent("rate_limited")
		return
	}

	link := strings.ReplaceAll(s.cfg.LinkTemplate, "{token}", token)
	err = s.mailer.Send(ctx, Message{
		To:      email,
		Subject: "Verify your email address",
		Text: "Use the link below to confirm this email address.\r\n\r\n" + link + "\r\n\r\n" +
			"The link expires in " + s.cfg.TokenTTL.Round(time.Minute).String() + " and works once.\r\n" +
			"If you did not request this, you can ignore this message.\r\n",
	})
	if err != nil {
		s.observer.VerificationEvent("send_error")
		s.log.ErrorContext(ctx, "sending verification email failed", "domain", domainOf(email), "error", err)
		return
	}
	s.observer.VerificationEvent("sent")
}

const tokenBytes = 32

var tokenLen = base64.RawURLEncoding.EncodedLen(tokenBytes)

func newToken() (string, Hash, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", Hash{}, err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, hashToken(t), nil
}

func hashToken(t string) Hash { return sha256.Sum256([]byte(t)) }

// hashEmail keys the per-address issuance limit. It folds every variant that
// most providers deliver to the same mailbox (letter case, "+tag" suffixes,
// Unicode forms of the domain) into one key, so variants cannot be used to
// multiply the limit. The address itself is sent exactly as submitted.
func hashEmail(email string) Hash {
	parsed := syntax.Parse(email)
	local, domain := parsed.Local, parsed.Domain
	if !parsed.Valid { // not reachable for queued addresses; be defensive
		at := strings.LastIndexByte(email, '@')
		local, domain = email[:max(at, 0)], email[at+1:]
	}
	local = strings.ToLower(local)
	if i := strings.IndexByte(local, '+'); i > 0 {
		local = local[:i]
	}
	return sha256.Sum256([]byte(local + "@" + strings.ToLower(domain)))
}

func wellFormedToken(t string) bool {
	if len(t) != tokenLen {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(t)
	return err == nil
}

func domainOf(email string) string { return email[strings.LastIndexByte(email, '@')+1:] }
