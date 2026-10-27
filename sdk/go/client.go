// Package validly is the Go client for the Validly API.
//
// Validly — Simple email validation for every stack.
//
//	client := validly.NewClient(validly.WithBaseURL("http://localhost:8080"))
//	result, err := client.Validate(ctx, "person@example.com")
//	if err != nil {
//		// Handle API, transport, or decoding errors.
//	}
//	fmt.Println(result.OverallStatus)
//
// A Client is safe for concurrent use. Keep API keys on the server: never
// ship them to browsers or mobile apps.
package validly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Version is the client version sent in the User-Agent header.
const Version = "0.1.0"

const (
	defaultBaseURL = "http://localhost:8080"
	defaultTimeout = 10 * time.Second
	maxBodyBytes   = 1 << 20
)

// Client calls the Validly API.
type Client struct {
	baseURL    string
	apiKey     string
	http       *http.Client
	maxRetries int
	backoff    time.Duration
	maxBackoff time.Duration
	userAgent  string
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets the API base URL, e.g. "https://validly.example.com".
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithAPIKey authenticates requests with an API key.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithHTTPClient replaces the underlying HTTP client. Its Timeout applies to
// each attempt.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeout sets the per-attempt timeout (default 10s). Use a context
// deadline to bound the total time including retries.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		h := *c.http
		h.Timeout = d
		c.http = &h
	}
}

// WithRetries sets how many times a transient failure is retried (default 2).
func WithRetries(n int) Option { return func(c *Client) { c.maxRetries = max(0, n) } }

// WithRetryBackoff sets the initial backoff between retries (default 200ms);
// it doubles on each attempt, with jitter, up to 5s.
func WithRetryBackoff(d time.Duration) Option { return func(c *Client) { c.backoff = d } }

// NewClient returns a Client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    defaultBaseURL,
		http:       &http.Client{Timeout: defaultTimeout},
		maxRetries: 2,
		backoff:    200 * time.Millisecond,
		maxBackoff: 5 * time.Second,
		userAgent:  "validly-go/" + Version,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Validate validates email with every check enabled.
func (c *Client) Validate(ctx context.Context, email string) (*Result, error) {
	return c.ValidateWithChecks(ctx, email, Checks{})
}

// ValidateWithChecks validates email, skipping the stages disabled in checks.
func (c *Client) ValidateWithChecks(ctx context.Context, email string, checks Checks) (*Result, error) {
	body := struct {
		Email  string     `json:"email"`
		Checks checksBody `json:"checks"`
	}{email, checks.body()}
	var r Result
	if err := c.do(ctx, "/api/v1/validate", body, &r, true); err != nil {
		return nil, err
	}
	return &r, nil
}

// SendVerification asks Validly to email a single-use verification link to
// email. It requires an API key. A nil error means the request was accepted,
// not that a message was delivered.
func (c *Client) SendVerification(ctx context.Context, email string) error {
	body := struct {
		Email string `json:"email"`
	}{email}
	var r struct {
		Status string `json:"status"`
	}
	return c.do(ctx, "/api/v1/verification/send", body, &r, false)
}

// ConfirmVerification consumes token and returns the verified address. An
// invalid, expired or reused token yields an *APIError with code
// ErrInvalidToken.
func (c *Client) ConfirmVerification(ctx context.Context, token string) (string, error) {
	body := struct {
		Token string `json:"token"`
	}{token}
	var r struct {
		Status string `json:"status"`
		Email  string `json:"email"`
	}
	if err := c.do(ctx, "/api/v1/verification/confirm", body, &r, false); err != nil {
		return "", err
	}
	return r.Email, nil
}

// do POSTs body and decodes the response into out. Requests are retried on
// 429, 502, 503 (except verification_disabled) and 504. Transport errors are
// retried only when idempotent is true, because a non-idempotent request may
// already have taken effect.
func (c *Client) do(ctx context.Context, path string, body, out any, idempotent bool) error {
	if _, err := url.ParseRequestURI(c.baseURL); err != nil {
		return fmt.Errorf("validly: invalid base URL %q: %w", c.baseURL, err)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("validly: encode request: %w", err)
	}

	var lastErr error
	for attempt := 0; ; attempt++ {
		lastErr = c.attempt(ctx, path, payload, out)
		if lastErr == nil {
			return nil
		}
		if attempt >= c.maxRetries || !retryable(lastErr, idempotent) {
			return lastErr
		}
		wait := c.backoffFor(attempt, lastErr)
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < wait {
			return lastErr
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return lastErr
		case <-t.C:
		}
	}
}

func (c *Client) attempt(ctx context.Context, path string, payload []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("validly: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &TransportError{Err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return &TransportError{Err: err}
	}

	if resp.StatusCode >= 300 {
		return newAPIError(resp, data)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &DecodeError{StatusCode: resp.StatusCode, Err: err}
	}
	return nil
}

func retryable(err error, idempotent bool) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.StatusCode {
		case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusGatewayTimeout:
			return true
		case http.StatusServiceUnavailable:
			return ae.Code != ErrVerificationDisabled
		}
		return false
	}
	var te *TransportError
	return idempotent && errors.As(err, &te)
}

func (c *Client) backoffFor(attempt int, err error) time.Duration {
	var ae *APIError
	if errors.As(err, &ae) && ae.RetryAfter > 0 {
		return min(ae.RetryAfter, c.maxBackoff)
	}
	d := c.backoff << attempt
	d = min(d, c.maxBackoff)
	// Full jitter in [d/2, d).
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

func newAPIError(resp *http.Response, data []byte) *APIError {
	ae := &APIError{StatusCode: resp.StatusCode}
	var body struct {
		Error struct {
			Code    ErrorCode `json:"code"`
			Message string    `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && body.Error.Code != "" {
		ae.Code, ae.Message = body.Error.Code, body.Error.Message
	} else {
		ae.Code, ae.Message = ErrUnknown, http.StatusText(resp.StatusCode)
	}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			ae.RetryAfter = time.Duration(secs) * time.Second
		}
	}
	return ae
}
