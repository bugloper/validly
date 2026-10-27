package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// apiError is an error with a stable machine-readable code.
type apiError struct {
	status     int
	code       string
	message    string
	retryAfter time.Duration
}

func (e *apiError) Error() string { return e.code + ": " + e.message }

func newError(status int, code, message string) *apiError {
	return &apiError{status: status, code: code, message: message}
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// toAPIError maps any handler error to the public error contract. Internal
// details never reach the client.
func toAPIError(err error) *apiError {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae
	}
	switch echo.StatusCode(err) {
	case http.StatusNotFound:
		return newError(http.StatusNotFound, "not_found", "Not found.")
	case http.StatusMethodNotAllowed:
		return newError(http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
	case http.StatusRequestEntityTooLarge:
		return newError(http.StatusRequestEntityTooLarge, "payload_too_large", "Request body is too large.")
	case http.StatusUnsupportedMediaType:
		return newError(http.StatusUnsupportedMediaType, "unsupported_media_type", "Request body must be application/json.")
	case http.StatusBadRequest:
		return newError(http.StatusBadRequest, "invalid_request", "Malformed request.")
	case http.StatusUnauthorized:
		return newError(http.StatusUnauthorized, "unauthorized", "Missing or invalid credentials.")
	}
	return newError(http.StatusInternalServerError, "internal_error", "Internal server error.")
}

func (s *server) handleError(c *echo.Context, err error) {
	if resp, uerr := echo.UnwrapResponse(c.Response()); uerr == nil && resp.Committed {
		return
	}
	ae := toAPIError(err)
	if ae.status >= 500 {
		s.Logger.ErrorContext(c.Request().Context(), "request failed", "route", c.Path(), "error", err)
	}
	s.Recorder.APIError(ae.code)
	if ae.retryAfter > 0 {
		c.Response().Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(ae.retryAfter.Seconds()))))
	}
	if c.Request().Method == http.MethodHead {
		_ = c.NoContent(ae.status)
		return
	}
	_ = c.JSON(ae.status, errorBody{Error: errorDetail{Code: ae.code, Message: ae.message}})
}

// observe records metrics, a trace span and a structured log line for each
// request. It never logs request bodies, email addresses or tokens.
func (s *server) observe(next echo.HandlerFunc) echo.HandlerFunc {
	propagator := otel.GetTextMapPropagator()
	return func(c *echo.Context) error {
		start := time.Now()
		req := c.Request()
		ctx := propagator.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
		ctx, span := s.Tracer.Start(ctx, "HTTP "+req.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		c.SetRequest(req.WithContext(ctx))

		err := next(c)
		if err != nil {
			// Render the error now so the recorded status is the real one.
			s.handleError(c, err)
		}

		route := c.Path()
		if route == "" {
			route = "unmatched"
		}
		status := http.StatusOK
		if resp, uerr := echo.UnwrapResponse(c.Response()); uerr == nil {
			status = resp.Status
		}
		dur := time.Since(start)
		s.Recorder.HTTPRequest(route, req.Method, status, dur)

		span.SetName(req.Method + " " + route)
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.String("http.request.method", req.Method),
			attribute.Int("http.response.status_code", status),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}

		s.Logger.InfoContext(ctx, "request",
			"method", req.Method,
			"route", route,
			"status", status,
			"duration_ms", float64(dur.Microseconds())/1000,
			"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
		)
		return nil
	}
}

const ctxKeyAPIKey = "validly.api_key"

// authenticate resolves the caller's API key, if any. A presented key that
// does not match is always rejected, even where keys are optional, so a
// misconfigured client fails loudly instead of being silently rate-limited
// as anonymous.
func (s *server) authenticate(required bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			key := presentedKey(c.Request())
			if key == "" {
				if required {
					return newError(http.StatusUnauthorized, "unauthorized", "An API key is required.")
				}
				return next(c)
			}
			name, ok := s.lookupKey(key)
			if !ok {
				return newError(http.StatusUnauthorized, "unauthorized", "Invalid API key.")
			}
			c.Set(ctxKeyAPIKey, name)
			return next(c)
		}
	}
}

func presentedKey(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	auth := r.Header.Get(echo.HeaderAuthorization)
	if scheme, token, ok := strings.Cut(auth, " "); ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(token)
	}
	return ""
}

// lookupKey compares against every configured key in constant time.
func (s *server) lookupKey(key string) (string, bool) {
	h := sha256.Sum256([]byte(key))
	name := ""
	for _, k := range s.Config.APIKeys {
		if subtle.ConstantTimeCompare(h[:], k.Hash[:]) == 1 {
			name = k.Name
		}
	}
	return name, name != ""
}

func apiKeyName(c *echo.Context) string {
	name, _ := c.Get(ctxKeyAPIKey).(string)
	return name
}

// rateLimit applies the per-key limit to authenticated callers and the
// per-IP limit to everyone else.
func (s *server) rateLimit(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var ok bool
		var wait time.Duration
		if name := apiKeyName(c); name != "" {
			ok, wait = s.keyLimiter.Allow("key:" + name)
		} else {
			ok, wait = s.ipLimiter.Allow("ip:" + clientKey(c.RealIP()))
		}
		if !ok {
			e := newError(http.StatusTooManyRequests, "rate_limited", "Rate limit exceeded. Retry later.")
			e.retryAfter = wait
			return e
		}
		return next(c)
	}
}

// clientKey groups IPv6 clients by /64, the smallest prefix a single
// subscriber is normally assigned, so rotating addresses within it does not
// yield fresh rate-limit buckets.
func clientKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().String()
	}
	return addr.String()
}

func (s *server) adminAuth(next echo.HandlerFunc) echo.HandlerFunc {
	want := sha256.Sum256([]byte(s.Config.AdminToken))
	return func(c *echo.Context) error {
		auth := c.Request().Header.Get(echo.HeaderAuthorization)
		scheme, token, _ := strings.Cut(auth, " ")
		got := sha256.Sum256([]byte(strings.TrimSpace(token)))
		if !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			return newError(http.StatusUnauthorized, "unauthorized", "Invalid admin token.")
		}
		return next(c)
	}
}

func requireJSON(c *echo.Context) error {
	ct := c.Request().Header.Get(echo.HeaderContentType)
	mediaType, _, _ := strings.Cut(ct, ";")
	if !strings.EqualFold(strings.TrimSpace(mediaType), echo.MIMEApplicationJSON) {
		return newError(http.StatusUnsupportedMediaType, "unsupported_media_type", "Request body must be application/json.")
	}
	return nil
}

func invalidRequest(format string, args ...any) *apiError {
	return newError(http.StatusBadRequest, "invalid_request", fmt.Sprintf(format, args...))
}
