package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.DNSTimeout != 3*time.Second || c.RateLimitIP != 60 || c.VerificationEnabled() {
		t.Fatalf("%+v", c)
	}
}

func TestParsesValues(t *testing.T) {
	c, err := Load(env(map[string]string{
		"VALIDLY_API_KEYS":         "web:0123456789abcdef, backend:fedcba9876543210",
		"VALIDLY_TRUSTED_PROXIES":  "10.0.0.0/8, 192.0.2.7",
		"VALIDLY_CORS_ORIGINS":     "https://app.example.com",
		"VALIDLY_DNS_SERVERS":      "1.1.1.1, [2606:4700:4700::1111]:53",
		"VALIDLY_DNS_TIMEOUT":      "1500ms",
		"VALIDLY_DATABASE_URL":     "postgres://localhost/validly",
		"VALIDLY_SMTP_HOST":        "smtp.example.com",
		"VALIDLY_SMTP_FROM":        "Validly <no-reply@example.com>",
		"VALIDLY_VERIFICATION_URL": "https://app.example.com/verify?token={token}",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.APIKeys) != 2 || c.APIKeys[1].Name != "backend" {
		t.Fatalf("keys %+v", c.APIKeys)
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[1].Bits() != 32 {
		t.Fatalf("proxies %v", c.TrustedProxies)
	}
	if c.DNSTimeout != 1500*time.Millisecond || !c.VerificationEnabled() {
		t.Fatalf("%+v", c)
	}
}

func TestRejectsInvalidConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		env  map[string]string
		want string
	}{
		"short api key":        {map[string]string{"VALIDLY_API_KEYS": "web:short"}, "at least 16"},
		"malformed api key":    {map[string]string{"VALIDLY_API_KEYS": "nocolon"}, "name:secret"},
		"require without keys": {map[string]string{"VALIDLY_REQUIRE_API_KEY": "true"}, "requires at least one key"},
		"short admin token":    {map[string]string{"VALIDLY_ADMIN_TOKEN": "abc"}, "32 characters"},
		"bad duration":         {map[string]string{"VALIDLY_DNS_TIMEOUT": "soon"}, "positive duration"},
		"bad proxy":            {map[string]string{"VALIDLY_TRUSTED_PROXIES": "proxy.local"}, "not an IP"},
		"bad cors":             {map[string]string{"VALIDLY_CORS_ORIGINS": "app.example.com"}, "must be an origin"},
		"dns hostname":         {map[string]string{"VALIDLY_DNS_SERVERS": "dns.google"}, "IP address"},
		"http sync url":        {map[string]string{"VALIDLY_DISPOSABLE_SYNC_URL": "http://lists.example.com/x"}, "https"},
		"partial verification": {map[string]string{"VALIDLY_SMTP_HOST": "smtp.example.com"}, "together"},
		"bad smtp tls":         {map[string]string{"VALIDLY_SMTP_TLS": "maybe"}, "required or opportunistic"},
		"missing placeholder": {map[string]string{
			"VALIDLY_DATABASE_URL": "postgres://x", "VALIDLY_SMTP_HOST": "h", "VALIDLY_SMTP_FROM": "a@b.co",
			"VALIDLY_VERIFICATION_URL": "https://app.example.com/verify", "VALIDLY_API_KEYS": "k:0123456789abcdef",
		}, "{token}"},
		"verification without keys": {map[string]string{
			"VALIDLY_DATABASE_URL": "postgres://x", "VALIDLY_SMTP_HOST": "h", "VALIDLY_SMTP_FROM": "a@b.co",
			"VALIDLY_VERIFICATION_URL": "https://app.example.com/v?t={token}",
		}, "require at least one API key"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(env(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestDatabaseAloneIsAllowed(t *testing.T) {
	c, err := Load(env(map[string]string{"VALIDLY_DATABASE_URL": "postgres://localhost/validly"}))
	if err != nil || c.VerificationEnabled() {
		t.Fatalf("%v %v", err, c.VerificationEnabled())
	}
}
