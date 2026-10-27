package validly

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"email":"person@example.com","syntax":{"status":"valid","reason":null},
"domain":{"normalized":"example.com","status":"deliverable","reason":"mx_records","mx_found":true,"fallback_address_found":false},
"disposable":{"status":"not_disposable"},"role":{"status":"not_role"},"suggestion":null,
"mailbox":{"status":"unverified"},"overall_status":"domain_deliverable"}`

func server(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func fastClient(url string, opts ...Option) *Client {
	return NewClient(append([]Option{WithBaseURL(url), WithRetryBackoff(time.Millisecond)}, opts...)...)
}

func TestValidate(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/validate" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("headers %v", r.Header)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["email"] != "person@example.com" || body["checks"].(map[string]any)["dns"] != true {
			t.Errorf("body %v", body)
		}
		_, _ = io.WriteString(w, okBody)
	})
	c := fastClient(srv.URL+"/", WithAPIKey("secret-key"))
	r, err := c.Validate(context.Background(), "person@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if r.OverallStatus != OverallDomainDeliverable || r.Domain.Status != DomainDeliverable || !*r.Domain.MXFound || r.Mailbox.Status != MailboxUnverified {
		t.Fatalf("%+v", r)
	}
}

func TestValidateWithChecks(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Checks map[string]bool }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Checks["dns"] || !body.Checks["typos"] {
			t.Errorf("checks %v", body.Checks)
		}
		_, _ = io.WriteString(w, okBody)
	})
	if _, err := fastClient(srv.URL).ValidateWithChecks(context.Background(), "a@b.co", Checks{SkipDNS: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIError(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"unauthorized","message":"Invalid API key."}}`)
	})
	_, err := fastClient(srv.URL).Validate(context.Background(), "a@b.co")
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 401 || ae.Code != ErrUnauthorized || !IsCode(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "<html>proxy error</html>")
	})
	_, err := fastClient(srv.URL).Validate(context.Background(), "a@b.co")
	if !IsCode(err, ErrUnknown) {
		t.Fatalf("got %v", err)
	}
}

func TestDecodeError(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "{not json") })
	_, err := fastClient(srv.URL).Validate(context.Background(), "a@b.co")
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("got %v", err)
	}
}

func TestRetriesTransientStatuses(t *testing.T) {
	var calls atomic.Int32
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":"rate_limited","message":"slow down"}}`)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = io.WriteString(w, okBody)
		}
	})
	c := fastClient(srv.URL)
	c.maxBackoff = 5 * time.Millisecond // keep the Retry-After wait short
	if _, err := c.Validate(context.Background(), "a@b.co"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls %d", calls.Load())
	}
}

func TestRetriesAreBounded(t *testing.T) {
	var calls atomic.Int32
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := fastClient(srv.URL, WithRetries(3)).Validate(context.Background(), "a@b.co")
	if err == nil || calls.Load() != 4 {
		t.Fatalf("err %v, calls %d", err, calls.Load())
	}
}

func TestDoesNotRetryClientErrorsOrDisabledFeature(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{400, `{"error":{"code":"invalid_request","message":""}}`},
		{500, `{"error":{"code":"internal_error","message":""}}`},
		{503, `{"error":{"code":"verification_disabled","message":""}}`},
	} {
		var calls atomic.Int32
		srv := server(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		})
		_ = fastClient(srv.URL).SendVerification(context.Background(), "a@b.co")
		if calls.Load() != 1 {
			t.Errorf("status %d retried: %d calls", tc.status, calls.Load())
		}
	}
}

func TestTransportErrorsRetriedOnlyForValidate(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepts atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			conn.Close() // drop the connection without a response
		}
	}()
	t.Cleanup(func() { ln.Close() })
	url := "http://" + ln.Addr().String()

	_, err = fastClient(url).Validate(context.Background(), "a@b.co")
	var te *TransportError
	if !errors.As(err, &te) || accepts.Load() != 3 {
		t.Fatalf("validate: %v after %d attempts", err, accepts.Load())
	}

	accepts.Store(0)
	_, err = fastClient(url).ConfirmVerification(context.Background(), "token")
	if !errors.As(err, &te) || accepts.Load() != 1 {
		t.Fatalf("confirm retried a non-idempotent request: %d attempts", accepts.Load())
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	start := time.Now()
	_, err := fastClient(srv.URL, WithTimeout(50*time.Millisecond), WithRetries(0)).Validate(context.Background(), "a@b.co")
	var te *TransportError
	if !errors.As(err, &te) || time.Since(start) > time.Second {
		t.Fatalf("timeout: %v in %v", err, time.Since(start))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = fastClient(srv.URL).Validate(ctx, "a@b.co")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context: %v", err)
	}
}

func TestVerification(t *testing.T) {
	srv := server(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/verification/send":
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"status":"accepted"}`)
		case "/api/v1/verification/confirm":
			_, _ = io.WriteString(w, `{"status":"verified","email":"a@b.co"}`)
		}
	})
	c := fastClient(srv.URL, WithAPIKey("k"))
	if err := c.SendVerification(context.Background(), "a@b.co"); err != nil {
		t.Fatal(err)
	}
	email, err := c.ConfirmVerification(context.Background(), "tok")
	if err != nil || email != "a@b.co" {
		t.Fatalf("%q %v", email, err)
	}
}

func TestInvalidBaseURL(t *testing.T) {
	if _, err := NewClient(WithBaseURL("not a url")).Validate(context.Background(), "a@b.co"); err == nil {
		t.Fatal("expected error")
	}
}
