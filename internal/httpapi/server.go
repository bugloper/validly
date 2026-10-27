// Package httpapi exposes Validly over HTTP using Echo.
package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/validly/validly/internal/config"
	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/ratelimit"
	"github.com/validly/validly/internal/validation"
	"github.com/validly/validly/internal/verification"
)

// Recorder receives HTTP-level metrics. *observability.Metrics implements it.
type Recorder interface {
	HTTPRequest(route, method string, status int, d time.Duration)
	Validation(overall string, d time.Duration)
	APIError(code string)
}

type nopRecorder struct{}

func (nopRecorder) HTTPRequest(string, string, int, time.Duration) {}
func (nopRecorder) Validation(string, time.Duration)               {}
func (nopRecorder) APIError(string)                                {}

// Deps are the server's collaborators. Optional fields may be nil.
type Deps struct {
	Config    *config.Config
	Validator *validation.Validator
	// Verification is nil when ownership verification is not configured.
	Verification *verification.Service
	// Disposable backs the admin endpoints, which are registered only when
	// Config.AdminToken is set.
	Disposable *disposable.Detector
	// Readiness reports optional dependencies (database, cache). A failing
	// check is reported but does not make the service unready.
	Readiness map[string]func(context.Context) error
	// Draining makes /readyz fail once shutdown has begun.
	Draining *atomic.Bool

	Recorder Recorder
	Tracer   trace.Tracer
	Logger   *slog.Logger
}

type server struct {
	Deps
	ipLimiter  *ratelimit.Limiter
	keyLimiter *ratelimit.Limiter
}

// New builds the HTTP handler.
func New(d Deps) http.Handler {
	if d.Recorder == nil {
		d.Recorder = nopRecorder{}
	}
	if d.Tracer == nil {
		d.Tracer = noop.NewTracerProvider().Tracer("")
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.Draining == nil {
		d.Draining = &atomic.Bool{}
	}
	cfg := d.Config
	s := &server{
		Deps:       d,
		ipLimiter:  ratelimit.New(cfg.RateLimitIP, time.Minute, cfg.RateLimitIP, 0),
		keyLimiter: ratelimit.New(cfg.RateLimitAPIKey, time.Minute, cfg.RateLimitAPIKey, 0),
	}

	e := echo.NewWithConfig(echo.Config{
		Logger:                       d.Logger,
		HTTPErrorHandler:             s.handleError,
		IPExtractor:                  ipExtractor(cfg),
		NoGroupAutoRegister404Routes: true,
	})

	e.Use(middleware.RequestID())
	e.Use(s.observe)
	e.Use(middleware.Recover())
	e.Use(securityHeaders)
	if len(cfg.CORSOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.CORSOrigins,
			AllowMethods: []string{http.MethodGet, http.MethodPost},
			AllowHeaders: []string{echo.HeaderContentType},
			MaxAge:       600,
		}))
	}
	e.Use(middleware.BodyLimit(cfg.MaxBodyBytes))

	e.GET("/healthz", s.health)
	e.GET("/readyz", s.ready)
	e.GET("/openapi.json", s.openapi)

	api := e.Group("/api/v1")
	api.POST("/validate", s.validate, s.authenticate(cfg.RequireAPIKey), s.rateLimit)
	api.POST("/verification/send", s.sendVerification, s.authenticate(true), s.rateLimit)
	api.POST("/verification/confirm", s.confirmVerification, s.authenticate(true), s.rateLimit)

	if cfg.AdminToken != "" && d.Disposable != nil {
		admin := e.Group("/api/v1/admin", s.adminAuth)
		admin.GET("/disposable-domains", s.listDomainEntries)
		admin.PUT("/disposable-domains/:list/:domain", s.addDomainEntry)
		admin.DELETE("/disposable-domains/:list/:domain", s.removeDomainEntry)
	}
	return e
}

func ipExtractor(cfg *config.Config) echo.IPExtractor {
	if len(cfg.TrustedProxies) == 0 {
		return echo.ExtractIPDirect()
	}
	opts := []echo.TrustOption{
		echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false),
	}
	for _, p := range cfg.TrustedProxies {
		_, n, err := net.ParseCIDR(p.String())
		if err == nil {
			opts = append(opts, echo.TrustIPRange(n))
		}
	}
	return echo.ExtractIPFromXFFHeader(opts...)
}

func securityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		h := c.Response().Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		return next(c)
	}
}
