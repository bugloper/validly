package validly

import (
	"errors"
	"fmt"
	"time"
)

// ErrorCode is a machine-readable API error code.
type ErrorCode string

const (
	ErrInvalidRequest       ErrorCode = "invalid_request"
	ErrInvalidEmail         ErrorCode = "invalid_email"
	ErrInvalidDomain        ErrorCode = "invalid_domain"
	ErrInvalidToken         ErrorCode = "invalid_token"
	ErrUnauthorized         ErrorCode = "unauthorized"
	ErrNotFound             ErrorCode = "not_found"
	ErrMethodNotAllowed     ErrorCode = "method_not_allowed"
	ErrPayloadTooLarge      ErrorCode = "payload_too_large"
	ErrUnsupportedMediaType ErrorCode = "unsupported_media_type"
	ErrRateLimited          ErrorCode = "rate_limited"
	ErrVerificationDisabled ErrorCode = "verification_disabled"
	ErrInternal             ErrorCode = "internal_error"
	// ErrUnknown is used when the response carried no Validly error body,
	// for example an error page from a proxy.
	ErrUnknown ErrorCode = "unknown"
)

// APIError is returned when the API responds with an error status.
type APIError struct {
	StatusCode int
	Code       ErrorCode
	Message    string
	// RetryAfter is set from the Retry-After header on 429 responses.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("validly: %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
}

// TransportError wraps a network failure: the request may or may not have
// reached the server.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "validly: transport error: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// DecodeError is returned when a successful response could not be decoded.
type DecodeError struct {
	StatusCode int
	Err        error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("validly: decode response (HTTP %d): %v", e.StatusCode, e.Err)
}
func (e *DecodeError) Unwrap() error { return e.Err }

// IsCode reports whether err is an *APIError with the given code.
func IsCode(err error, code ErrorCode) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == code
}
