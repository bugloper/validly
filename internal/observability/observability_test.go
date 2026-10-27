package observability

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/validly/validly/internal/dns"
)

func TestMetricsAreExported(t *testing.T) {
	tel, err := Setup(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })

	m := tel.Metrics
	m.HTTPRequest("/api/v1/validate", "POST", 200, 5*time.Millisecond)
	m.Validation("domain_deliverable", 3*time.Millisecond)
	m.APIError("rate_limited")
	m.DNSLookup(dns.StatusDeliverable, dns.ReasonMX, time.Millisecond)
	m.CacheResult("dns", true)
	m.VerificationEvent("sent")

	rec := httptest.NewRecorder()
	tel.MetricsHandler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []struct {
		name   string
		labels []string
	}{
		{"validly_http_requests_total", []string{`http_route="/api/v1/validate"`, `http_request_method="POST"`, `http_response_status_code="200"`}},
		{"validly_http_request_duration_seconds_bucket", nil},
		{"validly_validation_duration_seconds_count", []string{`overall_status="domain_deliverable"`}},
		{"validly_api_errors_total", []string{`code="rate_limited"`}},
		{"validly_dns_lookups_total", []string{`status="deliverable"`, `reason="mx_records"`}},
		{"validly_dns_lookup_duration_seconds_bucket", nil},
		{"validly_cache_requests_total", []string{`cache="dns"`, `result="hit"`}},
		{"validly_verification_events_total", []string{`event="sent"`}},
		{"target_info", []string{`service_name="validly"`}},
	} {
		if !hasSeries(body, want.name, want.labels...) {
			t.Errorf("metrics output has no %s series with %v", want.name, want.labels)
		}
	}
	if strings.Contains(body, "@") {
		t.Error("metrics must not contain email addresses")
	}
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, "warn")
	log.Info("hidden")
	log.Warn("shown", "k", "v")
	out, _ := io.ReadAll(&buf)
	if strings.Contains(string(out), "hidden") || !strings.Contains(string(out), `"msg":"shown"`) {
		t.Fatalf("unexpected log output: %s", out)
	}
	// Unknown levels fall back to info.
	buf.Reset()
	NewLogger(&buf, "loud").Info("x")
	if buf.Len() == 0 {
		t.Fatal("expected info output")
	}
}

// hasSeries reports whether body has a sample of metric name carrying all labels.
func hasSeries(body, name string, labels ...string) bool {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, name+"{") {
			continue
		}
		ok := true
		for _, l := range labels {
			if !strings.Contains(line, l) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
