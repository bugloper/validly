package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/validation"
	"github.com/validly/validly/internal/verification"
	"github.com/validly/validly/openapi"
)

// decode reads a JSON object from the body into v. Unknown fields are
// ignored so older servers keep working with newer clients.
func decode(c *echo.Context, v any) error {
	if err := requireJSON(c); err != nil {
		return err
	}
	dec := json.NewDecoder(c.Request().Body)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, echo.ErrStatusRequestEntityTooLarge) {
			return err
		}
		if errors.Is(err, io.EOF) {
			return invalidRequest("Request body is empty.")
		}
		return invalidRequest("Request body is not valid JSON for this endpoint.")
	}
	if dec.More() {
		return invalidRequest("Request body must contain a single JSON object.")
	}
	return nil
}

type validateRequest struct {
	Email  *string `json:"email"`
	Checks *struct {
		DNS        *bool `json:"dns"`
		Disposable *bool `json:"disposable"`
		Role       *bool `json:"role"`
		Typos      *bool `json:"typos"`
	} `json:"checks"`
}

func orTrue(b *bool) bool { return b == nil || *b }

func (s *server) validate(c *echo.Context) error {
	var req validateRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	if req.Email == nil {
		return invalidRequest("The email field is required.")
	}
	checks := validation.AllChecks()
	if req.Checks != nil {
		checks = validation.Checks{
			DNS:        orTrue(req.Checks.DNS),
			Disposable: orTrue(req.Checks.Disposable),
			Role:       orTrue(req.Checks.Role),
			Typos:      orTrue(req.Checks.Typos),
		}
	}

	ctx := c.Request().Context()
	start := time.Now()
	result := s.Validator.Validate(ctx, *req.Email, checks)
	s.Recorder.Validation(string(result.OverallStatus), time.Since(start))
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("validly.overall_status", string(result.OverallStatus)),
		attribute.String("validly.domain_status", string(result.Domain.Status)),
	)

	if apiKeyName(c) == "" {
		result.Redact()
	}
	return c.JSON(http.StatusOK, result)
}

func (s *server) sendVerification(c *echo.Context) error {
	if s.Verification == nil {
		return verificationDisabled()
	}
	var req struct {
		Email *string `json:"email"`
	}
	if err := decode(c, &req); err != nil {
		return err
	}
	if req.Email == nil {
		return invalidRequest("The email field is required.")
	}
	if err := s.Verification.RequestVerification(c.Request().Context(), *req.Email); err != nil {
		if errors.Is(err, verification.ErrInvalidEmail) {
			return newError(http.StatusBadRequest, "invalid_email", "The email address is not valid.")
		}
		return err
	}
	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *server) confirmVerification(c *echo.Context) error {
	if s.Verification == nil {
		return verificationDisabled()
	}
	var req struct {
		Token *string `json:"token"`
	}
	if err := decode(c, &req); err != nil {
		return err
	}
	if req.Token == nil {
		return invalidRequest("The token field is required.")
	}
	email, err := s.Verification.Confirm(c.Request().Context(), *req.Token)
	if errors.Is(err, verification.ErrInvalidToken) {
		return newError(http.StatusBadRequest, "invalid_token", "The token is invalid, expired or already used.")
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": string(validation.MailboxVerified), "email": email})
}

func verificationDisabled() error {
	return newError(http.StatusServiceUnavailable, "verification_disabled", "Ownership verification is not configured on this server.")
}

func (s *server) health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()
	checks := map[string]string{"database": "disabled", "cache": "disabled"}
	for name, check := range s.Readiness {
		if err := check(ctx); err != nil {
			checks[name] = "error"
			s.Logger.WarnContext(ctx, "readiness check failed", "check", name, "error", err)
		} else {
			checks[name] = "ok"
		}
	}
	body := map[string]any{"status": "ready", "checks": checks}
	if s.Draining.Load() {
		body["status"] = "not_ready"
		return c.JSON(http.StatusServiceUnavailable, body)
	}
	return c.JSON(http.StatusOK, body)
}

func (s *server) openapi(c *echo.Context) error {
	return c.Blob(http.StatusOK, echo.MIMEApplicationJSON, openapi.Spec)
}

func (s *server) listDomainEntries(c *echo.Context) error {
	entries, err := s.Disposable.Entries(c.Request().Context())
	if err != nil {
		return err
	}
	if entries == nil {
		entries = []disposable.Entry{}
	}
	return c.JSON(http.StatusOK, map[string]any{"entries": entries})
}

func listParam(c *echo.Context) (disposable.ListName, error) {
	list := disposable.ListName(c.Param("list"))
	if !list.Valid() {
		return "", newError(http.StatusNotFound, "not_found", "Unknown list; use blocklist or allowlist.")
	}
	return list, nil
}

func (s *server) addDomainEntry(c *echo.Context) error {
	list, err := listParam(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	domain, err := s.Disposable.AddEntry(ctx, list, c.Param("domain"))
	if errors.Is(err, disposable.ErrInvalidDomain) {
		return newError(http.StatusBadRequest, "invalid_domain", "The domain is not valid.")
	}
	if err != nil {
		return err
	}
	entries, err := s.Disposable.Entries(ctx)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.List == list && e.Domain == domain {
			s.Logger.InfoContext(ctx, "admin list entry added", "list", list, "domain", domain)
			return c.JSON(http.StatusOK, e)
		}
	}
	return errors.New("added entry not found")
}

func (s *server) removeDomainEntry(c *echo.Context) error {
	list, err := listParam(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	removed, err := s.Disposable.RemoveEntry(ctx, list, c.Param("domain"))
	if errors.Is(err, disposable.ErrInvalidDomain) {
		return newError(http.StatusBadRequest, "invalid_domain", "The domain is not valid.")
	}
	if err != nil {
		return err
	}
	if !removed {
		return newError(http.StatusNotFound, "not_found", "The domain is not on that list.")
	}
	s.Logger.InfoContext(ctx, "admin list entry removed", "list", list, "domain", c.Param("domain"))
	return c.NoContent(http.StatusNoContent)
}
