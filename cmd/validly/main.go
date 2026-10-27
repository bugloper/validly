// Command validly runs the Validly email validation service.
//
// Validly — Simple email validation for every stack.
//
// Usage:
//
//	validly [serve]          run the HTTP API (default)
//	validly check <email>    validate one address locally and print JSON
//	validly healthcheck      exit 0 if the local server's /healthz is OK
//	validly version          print the version
//
// Configuration is read from VALIDLY_* environment variables; see
// docs/configuration.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/validly/validly/internal/cache"
	"github.com/validly/validly/internal/config"
	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/dns"
	"github.com/validly/validly/internal/httpapi"
	"github.com/validly/validly/internal/observability"
	"github.com/validly/validly/internal/store"
	"github.com/validly/validly/internal/suggestions"
	"github.com/validly/validly/internal/validation"
	"github.com/validly/validly/internal/verification"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		os.Exit(serve())
	case "check":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: validly check <email>")
			os.Exit(2)
		}
		os.Exit(check(os.Args[2]))
	case "healthcheck":
		os.Exit(healthcheck())
	case "version", "--version", "-v":
		fmt.Println("validly", version)
	case "help", "--help", "-h":
		fmt.Println("Validly — Simple email validation for every stack.\n\nUsage:\n  validly [serve]\n  validly check <email>\n  validly healthcheck\n  validly version")
	default:
		fmt.Fprintf(os.Stderr, "validly: unknown command %q (try 'validly help')\n", cmd)
		os.Exit(2)
	}
}

// app holds the long-lived components built from configuration.
type app struct {
	cfg          *config.Config
	log          *slog.Logger
	telemetry    *observability.Telemetry
	validator    *validation.Validator
	detector     *disposable.Detector
	verification *verification.Service
	readiness    map[string]func(context.Context) error
	closers      []func()
}

func build(ctx context.Context, cfg *config.Config, log *slog.Logger) (*app, error) {
	a := &app{cfg: cfg, log: log, readiness: map[string]func(context.Context) error{}}

	tel, err := observability.Setup(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("telemetry: %w", err)
	}
	a.telemetry = tel

	// Each query attempt gets a third of the lookup budget, leaving time to
	// fail over to another resolver and for the A/AAAA fallback.
	resolver, err := dns.NewResolver(cfg.DNSServers, max(cfg.DNSTimeout/3, 250*time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("dns: %w", err)
	}
	var dnsCache cache.Cache[dns.Result] = cache.NewMemory[dns.Result](cfg.CacheMaxEntries)
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("VALIDLY_REDIS_URL: %w", err)
		}
		// Keep timeouts short and never retry: the cache is an optimisation
		// and a slow or absent Redis must not slow down validation.
		opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout = 250*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond
		opts.MaxRetries = -1
		opts.ContextTimeoutEnabled = true
		client := redis.NewClient(opts)
		a.closers = append(a.closers, func() { _ = client.Close() })
		dnsCache = cache.NewRedis[dns.Result](client, "validly:dns:")
		a.readiness["cache"] = func(ctx context.Context) error { return client.Ping(ctx).Err() }
	}
	dnsCfg := dns.DefaultConfig()
	dnsCfg.Timeout = cfg.DNSTimeout
	dnsCfg.MaxConcurrent = cfg.DNSMaxConcurrent
	checker := dns.NewChecker(resolver, dnsCache, dnsCfg, tel.Metrics, log)

	var adminStore disposable.AdminStore = disposable.NewMemoryStore()
	var pg *store.Postgres
	if cfg.DatabaseURL != "" {
		pg, err = store.Open(ctx, cfg.DatabaseURL, log)
		if err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		a.closers = append(a.closers, pg.Close)
		a.readiness["database"] = pg.Ping
		adminStore = pg
	}

	a.detector, err = disposable.New(disposable.Config{
		File:         cfg.DisposableFile,
		SyncURL:      cfg.DisposableSyncURL,
		SyncInterval: cfg.DisposableSyncInterval,
	}, adminStore, log)
	if err != nil {
		return nil, err
	}

	domains := cfg.SuggestionDomains
	if len(domains) == 0 {
		domains = suggestions.DefaultDomains
	}
	a.validator = &validation.Validator{
		DNS:         checker,
		Disposable:  a.detector,
		Suggestions: suggestions.New(domains),
	}

	if cfg.VerificationEnabled() {
		a.verification = verification.NewService(pg, &verification.SMTPMailer{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
			// Local development relays such as Mailpit do not offer TLS.
			Opportunistic: cfg.SMTPTLS == "opportunistic",
		}, verification.Config{
			TokenTTL:     cfg.VerificationTokenTTL,
			LinkTemplate: cfg.VerificationURL,
		}, tel.Metrics, log)
	}

	log.Info("validly configured",
		"version", version,
		"dns_servers", resolver.Servers(),
		"dns_cache", map[bool]string{true: "redis", false: "memory"}[cfg.RedisURL != ""],
		"database", cfg.DatabaseURL != "",
		"verification", a.verification != nil,
		"admin_api", cfg.AdminToken != "",
		"disposable_domains_bundled", a.detector.BundledCount(),
		"api_keys", len(cfg.APIKeys),
		"require_api_key", cfg.RequireAPIKey,
	)
	return a, nil
}

func (a *app) close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

func serve() int {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validly: invalid configuration:\n"+err.Error())
		return 2
	}
	log := observability.NewLogger(os.Stdout, cfg.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := build(ctx, cfg, log)
	if err != nil {
		log.Error("startup failed", "error", err)
		return 1
	}
	defer a.close()

	bg, cancelBG := context.WithCancel(context.Background())
	defer cancelBG()
	go a.detector.Run(bg)
	if a.verification != nil {
		go a.verification.Run(bg)
	}

	draining := &atomic.Bool{}
	handler := httpapi.New(httpapi.Deps{
		Config:       cfg,
		Validator:    a.validator,
		Verification: a.verification,
		Disposable:   a.detector,
		Readiness:    a.readiness,
		Draining:     draining,
		Recorder:     a.telemetry.Metrics,
		Tracer:       a.telemetry.Tracer,
		Logger:       log,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	servers := []*http.Server{srv}
	if cfg.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", a.telemetry.MetricsHandler)
		servers = append(servers, &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second})
	}

	errc := make(chan error, len(servers))
	for _, s := range servers {
		ln, err := net.Listen("tcp", s.Addr)
		if err != nil {
			log.Error("listen failed", "addr", s.Addr, "error", err)
			return 1
		}
		log.Info("listening", "addr", ln.Addr().String())
		go func() {
			if err := s.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errc:
		log.Error("server failed", "error", err)
		return 1
	}

	// Fail readiness first so load balancers stop routing new traffic, then
	// let in-flight requests finish.
	draining.Store(true)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	for _, s := range servers {
		if err := s.Shutdown(shutdownCtx); err != nil {
			log.Warn("graceful shutdown incomplete", "addr", s.Addr, "error", err)
		}
	}
	cancelBG()
	if err := a.telemetry.Shutdown(shutdownCtx); err != nil {
		log.Warn("telemetry shutdown failed", "error", err)
	}
	log.Info("stopped")
	return 0
}

func check(email string) int {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validly: invalid configuration:\n"+err.Error())
		return 2
	}
	// Keep the CLI quiet and free of side effects: no database, no Redis.
	cfg.DatabaseURL, cfg.RedisURL, cfg.SMTPHost, cfg.VerificationURL = "", "", "", ""
	log := observability.NewLogger(os.Stderr, "error")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DNSTimeout+time.Second)
	defer cancel()
	a, err := build(ctx, cfg, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validly:", err)
		return 1
	}
	defer a.close()
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(a.validator.Validate(ctx, email, validation.AllChecks()))
	return 0
}

func healthcheck() int {
	addr := os.Getenv("VALIDLY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validly: bad VALIDLY_ADDR:", err)
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "validly: healthcheck failed:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "validly: healthcheck status", resp.StatusCode)
		return 1
	}
	return 0
}
