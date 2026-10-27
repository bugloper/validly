// Package config loads Validly's configuration from environment variables.
// docs/configuration.md documents every variable.
package config

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIKey is a configured client credential. Only its hash is kept.
type APIKey struct {
	Name string
	Hash [sha256.Size]byte
}

type Config struct {
	Addr            string
	MetricsAddr     string
	LogLevel        string
	MaxBodyBytes    int64
	ShutdownTimeout time.Duration

	CORSOrigins    []string
	TrustedProxies []netip.Prefix

	APIKeys       []APIKey
	RequireAPIKey bool
	AdminToken    string

	RateLimitIP     int // requests per minute per client IP
	RateLimitAPIKey int // requests per minute per API key

	DNSServers       []string
	DNSTimeout       time.Duration
	DNSMaxConcurrent int
	CacheMaxEntries  int
	RedisURL         string

	DisposableFile         string
	DisposableSyncURL      string
	DisposableSyncInterval time.Duration

	SuggestionDomains []string

	DatabaseURL string

	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	// SMTPTLS is "required" (default) or "opportunistic".
	SMTPTLS string

	VerificationURL      string
	VerificationTokenTTL time.Duration
}

// VerificationEnabled reports whether ownership verification is configured.
func (c *Config) VerificationEnabled() bool {
	return c.DatabaseURL != "" && c.SMTPHost != "" && c.VerificationURL != ""
}

// Load reads configuration using getenv (normally os.Getenv).
func Load(getenv func(string) string) (*Config, error) {
	p := parser{getenv: getenv}
	c := &Config{
		Addr:            p.str("VALIDLY_ADDR", ":8080"),
		MetricsAddr:     p.str("VALIDLY_METRICS_ADDR", ":9090"),
		LogLevel:        p.str("VALIDLY_LOG_LEVEL", "info"),
		MaxBodyBytes:    int64(p.int("VALIDLY_MAX_BODY_BYTES", 8192)),
		ShutdownTimeout: p.duration("VALIDLY_SHUTDOWN_TIMEOUT", 15*time.Second),

		CORSOrigins:   p.list("VALIDLY_CORS_ORIGINS"),
		RequireAPIKey: p.bool("VALIDLY_REQUIRE_API_KEY", false),
		AdminToken:    p.str("VALIDLY_ADMIN_TOKEN", ""),

		RateLimitIP:     p.int("VALIDLY_RATE_LIMIT_IP", 60),
		RateLimitAPIKey: p.int("VALIDLY_RATE_LIMIT_API_KEY", 600),

		DNSServers:       p.list("VALIDLY_DNS_SERVERS"),
		DNSTimeout:       p.duration("VALIDLY_DNS_TIMEOUT", 3*time.Second),
		DNSMaxConcurrent: p.int("VALIDLY_DNS_MAX_CONCURRENT", 64),
		CacheMaxEntries:  p.int("VALIDLY_CACHE_MAX_ENTRIES", 100_000),
		RedisURL:         p.str("VALIDLY_REDIS_URL", ""),

		DisposableFile:         p.str("VALIDLY_DISPOSABLE_FILE", ""),
		DisposableSyncURL:      p.str("VALIDLY_DISPOSABLE_SYNC_URL", ""),
		DisposableSyncInterval: p.duration("VALIDLY_DISPOSABLE_SYNC_INTERVAL", 24*time.Hour),

		SuggestionDomains: p.list("VALIDLY_SUGGESTION_DOMAINS"),

		DatabaseURL: p.str("VALIDLY_DATABASE_URL", ""),

		SMTPHost:     p.str("VALIDLY_SMTP_HOST", ""),
		SMTPPort:     p.str("VALIDLY_SMTP_PORT", "587"),
		SMTPUsername: p.str("VALIDLY_SMTP_USERNAME", ""),
		SMTPPassword: p.str("VALIDLY_SMTP_PASSWORD", ""),
		SMTPFrom:     p.str("VALIDLY_SMTP_FROM", ""),
		SMTPTLS:      p.str("VALIDLY_SMTP_TLS", "required"),

		VerificationURL:      p.str("VALIDLY_VERIFICATION_URL", ""),
		VerificationTokenTTL: p.duration("VALIDLY_VERIFICATION_TOKEN_TTL", 30*time.Minute),
	}

	for _, s := range p.list("VALIDLY_TRUSTED_PROXIES") {
		pfx, err := netip.ParsePrefix(s)
		if err != nil {
			addr, aerr := netip.ParseAddr(s)
			if aerr != nil {
				p.fail("VALIDLY_TRUSTED_PROXIES", "%q is not an IP or CIDR", s)
				continue
			}
			pfx = netip.PrefixFrom(addr, addr.BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, pfx)
	}

	seen := map[string]bool{}
	for _, entry := range p.list("VALIDLY_API_KEYS") {
		name, secret, ok := strings.Cut(entry, ":")
		switch {
		case !ok || name == "" || secret == "":
			p.fail("VALIDLY_API_KEYS", "entries must be name:secret")
		case len(secret) < 16:
			p.fail("VALIDLY_API_KEYS", "secret for %q must be at least 16 characters", name)
		case seen[name]:
			p.fail("VALIDLY_API_KEYS", "duplicate key name %q", name)
		default:
			seen[name] = true
			c.APIKeys = append(c.APIKeys, APIKey{Name: name, Hash: sha256.Sum256([]byte(secret))})
		}
	}

	c.validate(&p)
	return c, errors.Join(p.errs...)
}

func (c *Config) validate(p *parser) {
	if c.RequireAPIKey && len(c.APIKeys) == 0 {
		p.fail("VALIDLY_REQUIRE_API_KEY", "requires at least one key in VALIDLY_API_KEYS")
	}
	if c.AdminToken != "" && len(c.AdminToken) < 32 {
		p.fail("VALIDLY_ADMIN_TOKEN", "must be at least 32 characters")
	}
	if c.MaxBodyBytes < 512 {
		p.fail("VALIDLY_MAX_BODY_BYTES", "must be at least 512")
	}
	if c.DNSTimeout <= 0 || c.DNSMaxConcurrent < 1 || c.CacheMaxEntries < 1 {
		p.fail("VALIDLY_DNS_*", "timeout, concurrency and cache size must be positive")
	}
	for _, o := range c.CORSOrigins {
		if o == "*" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" {
			p.fail("VALIDLY_CORS_ORIGINS", "%q must be an origin such as https://app.example.com", o)
		}
	}
	for _, s := range c.DNSServers {
		host := s
		if h, _, err := net.SplitHostPort(s); err == nil {
			host = h
		}
		if _, err := netip.ParseAddr(host); err != nil {
			p.fail("VALIDLY_DNS_SERVERS", "%q must be an IP address with optional port", s)
		}
	}
	if c.DisposableSyncURL != "" {
		if u, err := url.Parse(c.DisposableSyncURL); err != nil || u.Scheme != "https" {
			p.fail("VALIDLY_DISPOSABLE_SYNC_URL", "must be an https URL")
		}
	}

	// Ownership verification needs a database, SMTP and a link template. A
	// database on its own is fine (it persists the admin lists), but SMTP or
	// a link template without the rest is a partial configuration: fail fast
	// instead of silently disabling the feature.
	if (c.SMTPHost != "" || c.VerificationURL != "") && !c.VerificationEnabled() {
		p.fail("VALIDLY_VERIFICATION_URL", "ownership verification needs VALIDLY_DATABASE_URL, VALIDLY_SMTP_HOST and VALIDLY_VERIFICATION_URL together")
	}
	if c.SMTPTLS != "required" && c.SMTPTLS != "opportunistic" {
		p.fail("VALIDLY_SMTP_TLS", "must be required or opportunistic")
	}
	if c.VerificationEnabled() {
		if !strings.Contains(c.VerificationURL, "{token}") {
			p.fail("VALIDLY_VERIFICATION_URL", "must contain the {token} placeholder")
		} else if u, err := url.Parse(strings.ReplaceAll(c.VerificationURL, "{token}", "x")); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			p.fail("VALIDLY_VERIFICATION_URL", "must be an http(s) URL")
		}
		if c.SMTPFrom == "" {
			p.fail("VALIDLY_SMTP_FROM", "is required when ownership verification is enabled")
		}
		if len(c.APIKeys) == 0 {
			p.fail("VALIDLY_API_KEYS", "ownership verification endpoints require at least one API key")
		}
	}
}

type parser struct {
	getenv func(string) string
	errs   []error
}

func (p *parser) fail(key, format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf("%s: %s", key, fmt.Sprintf(format, args...)))
}

func (p *parser) str(key, def string) string {
	if v := strings.TrimSpace(p.getenv(key)); v != "" {
		return v
	}
	return def
}

func (p *parser) list(key string) []string {
	var out []string
	for _, s := range strings.Split(p.getenv(key), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (p *parser) int(key string, def int) int {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		p.fail(key, "must be a non-negative integer")
		return def
	}
	return n
}

func (p *parser) bool(key string, def bool) bool {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		p.fail(key, "must be true or false")
		return def
	}
	return b
}

func (p *parser) duration(key string, def time.Duration) time.Duration {
	v := p.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		p.fail(key, "must be a positive duration such as 3s or 30m")
		return def
	}
	return d
}
