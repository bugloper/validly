package verification

import (
	"bufio"
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type captureMailer struct {
	mu   sync.Mutex
	msgs []Message
	err  error
	sent chan struct{}
}

func newCapture() *captureMailer { return &captureMailer{sent: make(chan struct{}, 100)} }

func (c *captureMailer) Send(_ context.Context, m Message) error {
	c.mu.Lock()
	c.msgs = append(c.msgs, m)
	c.mu.Unlock()
	c.sent <- struct{}{}
	return c.err
}

func (c *captureMailer) last() Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.msgs[len(c.msgs)-1]
}

var tokenRE = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

func setup(t *testing.T, cfg Config) (*Service, *MemoryStore, *captureMailer, *time.Time) {
	t.Helper()
	store, mailer := NewMemoryStore(), newCapture()
	if cfg.LinkTemplate == "" {
		cfg.LinkTemplate = "https://app.example.com/verify?token={token}"
	}
	s := NewService(store, mailer, cfg, nil, nil)
	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	return s, store, mailer, &now
}

func requestAndWait(t *testing.T, s *Service, m *captureMailer, email string) string {
	t.Helper()
	if err := s.RequestVerification(context.Background(), email); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("no email sent")
	}
	match := tokenRE.FindStringSubmatch(m.last().Text)
	if match == nil {
		t.Fatalf("no token in %q", m.last().Text)
	}
	return match[1]
}

func TestSendAndConfirm(t *testing.T) {
	s, _, m, _ := setup(t, Config{})
	ctx := context.Background()

	token := requestAndWait(t, s, m, "Person@Example.com")
	if got := m.last(); got.To != "Person@Example.com" || !strings.Contains(got.Text, "https://app.example.com/verify?token=") {
		t.Fatalf("message %+v", got)
	}
	if len(token) != 43 {
		t.Fatalf("token length %d", len(token))
	}

	email, err := s.Confirm(ctx, token)
	if err != nil || email != "Person@Example.com" {
		t.Fatalf("confirm: %q %v", email, err)
	}
	// Replay is rejected.
	if _, err := s.Confirm(ctx, token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("replay: %v", err)
	}
}

func TestStoresOnlyHashes(t *testing.T) {
	s, store, m, _ := setup(t, Config{})
	token := requestAndWait(t, s, m, "a@example.com")
	store.mu.Lock()
	defer store.mu.Unlock()
	for h := range store.tokens {
		if strings.Contains(string(h[:]), token) {
			t.Fatal("raw token stored")
		}
		if h != hashToken(token) {
			t.Fatal("unexpected hash")
		}
	}
}

func TestTokenExpiry(t *testing.T) {
	s, _, m, now := setup(t, Config{TokenTTL: 10 * time.Minute})
	token := requestAndWait(t, s, m, "a@example.com")
	*now = now.Add(10 * time.Minute)
	if _, err := s.Confirm(context.Background(), token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}

func TestInvalidTokens(t *testing.T) {
	s, _, _, _ := setup(t, Config{})
	for _, tok := range []string{"", "short", strings.Repeat("A", 43), strings.Repeat("!", 43), strings.Repeat("A", 1000)} {
		if _, err := s.Confirm(context.Background(), tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%q: %v", tok, err)
		}
	}
}

func TestIssuanceRateLimit(t *testing.T) {
	s, _, m, now := setup(t, Config{MaxSends: 2, SendWindow: time.Hour})
	requestAndWait(t, s, m, "a@example.com")
	requestAndWait(t, s, m, "a@EXAMPLE.com") // same address, domain case-folded

	// The third request is accepted but nothing is sent.
	if err := s.RequestVerification(context.Background(), "a@example.com"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.sent:
		t.Fatal("rate limit not applied")
	case <-time.After(100 * time.Millisecond):
	}

	// A different address is unaffected.
	requestAndWait(t, s, m, "b@example.com")

	*now = now.Add(time.Hour + time.Second)
	requestAndWait(t, s, m, "a@example.com")
}

func TestIssuanceLimitCoversAddressVariants(t *testing.T) {
	s, _, m, _ := setup(t, Config{MaxSends: 2, SendWindow: time.Hour})
	requestAndWait(t, s, m, "alice@example.com")
	requestAndWait(t, s, m, "ALICE@Example.COM")
	for _, variant := range []string{"Alice@example.com", "alice+x@example.com", "alice@ｅｘａｍｐｌｅ.com"} {
		if err := s.RequestVerification(context.Background(), variant); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-m.sent:
		t.Fatalf("variant bypassed the per-address limit: %q", m.last().To)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestRequestRejectsInvalidEmail(t *testing.T) {
	s, _, _, _ := setup(t, Config{})
	if err := s.RequestVerification(context.Background(), "not an email"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("got %v", err)
	}
}

func TestQueueFullDropsSilently(t *testing.T) {
	store, mailer := NewMemoryStore(), newCapture()
	s := NewService(store, mailer, Config{QueueSize: 1, LinkTemplate: "x{token}"}, nil, nil)
	// No Run: nothing drains the queue.
	for range 5 {
		if err := s.RequestVerification(context.Background(), "a@example.com"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentConfirmOnlyOneWins(t *testing.T) {
	s, _, m, _ := setup(t, Config{})
	token := requestAndWait(t, s, m, "a@example.com")
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for range 20 {
		wg.Go(func() {
			if _, err := s.Confirm(context.Background(), token); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d confirmations succeeded", wins)
	}
}

// fakeSMTP is a minimal SMTP server (without STARTTLS) that records one
// message. If rcptReply is set, RCPT TO is answered with it.
func fakeSMTP(t *testing.T, rcptReply ...string) (string, string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan string, 2)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 localhost ESMTP test")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					got <- data.String()
					write("250 OK")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				write("250 localhost")
			case strings.HasPrefix(cmd, "RCPT TO:") && len(rcptReply) > 0:
				write(rcptReply[0])
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				got <- cmd
				write("250 OK")
			case cmd == "DATA":
				inData = true
				write("354 go ahead")
			case cmd == "QUIT":
				write("221 bye")
				return
			default:
				write("250 OK")
			}
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	return host, port, got
}

func TestSMTPMailer(t *testing.T) {
	host, port, got := fakeSMTP(t)
	m := &SMTPMailer{Host: host, Port: port, From: "Validly <no-reply@validly.test>", Opportunistic: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Send(ctx, Message{To: "a@example.com", Subject: "Verify your email address", Text: "hello\r\n"}); err != nil {
		t.Fatal(err)
	}
	if from := <-got; from != "MAIL FROM:<NO-REPLY@VALIDLY.TEST>" {
		t.Errorf("envelope sender %q", from)
	}
	msg := <-got
	for _, want := range []string{"To: a@example.com\r\n", "Subject: Verify your email address\r\n", "\r\n\r\nhello\r\n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestSMTPMailerRequiresTLSByDefault(t *testing.T) {
	host, port, _ := fakeSMTP(t)
	m := &SMTPMailer{Host: host, Port: port, From: "a@example.com"}
	err := m.Send(context.Background(), Message{To: "b@example.com", Subject: "s", Text: "t"})
	if !errors.Is(err, ErrNoSTARTTLS) {
		t.Fatalf("got %v", err)
	}
}

func TestSMTPErrorsDoNotEchoTheRecipient(t *testing.T) {
	host, port, _ := fakeSMTP(t, "550 5.1.1 <secret.person@example.com>: Recipient address rejected")
	m := &SMTPMailer{Host: host, Port: port, From: "a@example.com", Opportunistic: true}
	err := m.Send(context.Background(), Message{To: "secret.person@example.com", Subject: "s", Text: "t"})
	if err == nil || strings.Contains(err.Error(), "secret.person") || !strings.Contains(err.Error(), "550") {
		t.Fatalf("got %v", err)
	}
}

func TestSMTPMailerRejectsHeaderInjection(t *testing.T) {
	m := &SMTPMailer{Host: "127.0.0.1", Port: "1", From: "a@example.com"}
	err := m.Send(context.Background(), Message{To: "a@example.com\r\nBcc: x@example.com"})
	if err == nil || !strings.Contains(err.Error(), "line breaks") {
		t.Fatalf("got %v", err)
	}
}
