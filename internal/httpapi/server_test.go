package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/validly/validly/internal/config"
	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/dns"
	"github.com/validly/validly/internal/suggestions"
	"github.com/validly/validly/internal/validation"
	"github.com/validly/validly/internal/verification"
	"github.com/validly/validly/openapi"
)

const (
	testKey  = "test-key-0123456789"
	adminTok = "admin-token-0123456789abcdef0123456789"
	linkTmpl = "https://app.example.com/verify?token={token}"
	clientIP = "203.0.113.9:1234"
)

type fakeDNS map[string]dns.Result

func (f fakeDNS) Check(_ context.Context, d string) dns.Result {
	if r, ok := f[d]; ok {
		return r
	}
	return dns.Result{Status: dns.StatusUnknown, Reason: dns.ReasonServFail}
}

func b(v bool) *bool { return &v }

type mailbox struct {
	mu   sync.Mutex
	msgs []verification.Message
	sent chan struct{}
}

func (m *mailbox) Send(_ context.Context, msg verification.Message) error {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
	m.sent <- struct{}{}
	return nil
}

type harness struct {
	handler  http.Handler
	mail     *mailbox
	draining *atomic.Bool
	t        *testing.T
}

func newHarness(t *testing.T, mutate func(*config.Config)) *harness {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{
			"VALIDLY_API_KEYS":    "tests:" + testKey,
			"VALIDLY_ADMIN_TOKEN": adminTok,
		}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(cfg)
	}

	detector, err := disposable.New(disposable.Config{}, disposable.NewMemoryStore(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mail := &mailbox{sent: make(chan struct{}, 10)}
	svc := verification.NewService(verification.NewMemoryStore(), mail, verification.Config{LinkTemplate: linkTmpl}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)

	draining := &atomic.Bool{}
	h := New(Deps{
		Config: cfg,
		Validator: &validation.Validator{
			DNS: fakeDNS{
				"example.com":        {Status: dns.StatusDeliverable, Reason: dns.ReasonMX, MXFound: b(true), FallbackAddressFound: b(false)},
				"gmial.com":          {Status: dns.StatusDeliverable, Reason: dns.ReasonMX, MXFound: b(true), FallbackAddressFound: b(false)},
				"nomail.example.org": {Status: dns.StatusUndeliverable, Reason: dns.ReasonNullMX, MXFound: b(true), FallbackAddressFound: b(false)},
			},
			Disposable:  detector,
			Suggestions: suggestions.New(suggestions.DefaultDomains),
		},
		Verification: svc,
		Disposable:   detector,
		Readiness: map[string]func(context.Context) error{
			"database": func(context.Context) error { return nil },
			"cache":    func(context.Context) error { return errors.New("redis down") },
		},
		Draining: draining,
	})
	return &harness{handler: h, mail: mail, draining: draining, t: t}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
	return m
}

func (h *harness) do(method, path, body string, headers ...string) response {
	h.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = clientIP
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	res := response{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
	assertContract(h.t, method, path, res)
	return res
}

// --- OpenAPI contract -------------------------------------------------------

var (
	specOnce    sync.Once
	spec        map[string]any
	compiler    *jsonschema.Compiler
	schemaMu    sync.Mutex
	schemaByRef = map[string]*jsonschema.Schema{}
)

func loadSpec(t *testing.T) {
	specOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(openapi.Spec))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(openapi.Spec, &spec); err != nil {
			t.Fatal(err)
		}
		compiler = jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		if err := compiler.AddResource("https://validly.test/openapi.json", doc); err != nil {
			t.Fatal(err)
		}
	})
}

func schemaFor(t *testing.T, ref string) *jsonschema.Schema {
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if s, ok := schemaByRef[ref]; ok {
		return s
	}
	s, err := compiler.Compile("https://validly.test/openapi.json" + ref)
	if err != nil {
		t.Fatalf("compile %s: %v", ref, err)
	}
	schemaByRef[ref] = s
	return s
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// specPath finds the templated spec path that matches a concrete path.
func specPath(path string) (string, bool) {
	path, _, _ = strings.Cut(path, "?")
	for p := range spec["paths"].(map[string]any) {
		re := "^" + pathParam.ReplaceAllString(p, `[^/]+`) + "$"
		if regexp.MustCompile(re).MatchString(path) {
			return p, true
		}
	}
	return "", false
}

// assertContract checks that the status is documented for the operation and
// that the body matches the documented schema.
func assertContract(t *testing.T, method, path string, res response) {
	t.Helper()
	loadSpec(t)
	p, ok := specPath(path)
	if !ok {
		if res.status != http.StatusNotFound {
			t.Fatalf("undocumented path %s returned %d", path, res.status)
		}
		return
	}
	op, ok := spec["paths"].(map[string]any)[p].(map[string]any)[strings.ToLower(method)].(map[string]any)
	if !ok {
		if res.status != http.StatusMethodNotAllowed && res.status != http.StatusNotFound {
			t.Fatalf("undocumented method %s %s returned %d", method, p, res.status)
		}
		return
	}
	resp, ok := op["responses"].(map[string]any)[strconv.Itoa(res.status)].(map[string]any)
	if !ok {
		t.Fatalf("%s %s: status %d is not documented (body %s)", method, p, res.status, res.body)
	}
	if ref, ok := resp["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/responses/")
		resp = spec["components"].(map[string]any)["responses"].(map[string]any)[name].(map[string]any)
	}
	content, ok := resp["content"].(map[string]any)
	if !ok {
		if len(res.body) != 0 {
			t.Fatalf("%s %s %d: expected empty body, got %s", method, p, res.status, res.body)
		}
		return
	}
	schema, ok := content["application/json"].(map[string]any)["schema"].(map[string]any)
	if !ok {
		return
	}
	ref, ok := schema["$ref"].(string)
	if !ok {
		t.Fatalf("%s %s %d: inline schemas are not supported by this test", method, p, res.status)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(res.body))
	if err != nil {
		t.Fatalf("%s %s %d: body is not JSON: %s", method, p, res.status, res.body)
	}
	if err := schemaFor(t, ref).Validate(inst); err != nil {
		t.Fatalf("%s %s %d: body violates %s:\n%s\n%v", method, p, res.status, ref, res.body, err)
	}
}

func TestRoutesMatchSpec(t *testing.T) {
	loadSpec(t)
	h := newHarness(t, nil)
	var routed []string
	for _, r := range h.handler.(*echo.Echo).Router().Routes() {
		path := regexp.MustCompile(`:([a-z]+)`).ReplaceAllString(r.Path, "{$1}")
		routed = append(routed, r.Method+" "+path)
	}
	var documented []string
	for p, ops := range spec["paths"].(map[string]any) {
		for m := range ops.(map[string]any) {
			if m != "parameters" {
				documented = append(documented, strings.ToUpper(m)+" "+p)
			}
		}
	}
	sort.Strings(routed)
	sort.Strings(documented)
	if fmt.Sprint(routed) != fmt.Sprint(documented) {
		t.Fatalf("routes and spec differ:\nrouted:     %v\ndocumented: %v", routed, documented)
	}
}

// --- validation -------------------------------------------------------------

func TestValidateDeliverable(t *testing.T) {
	h := newHarness(t, nil)
	res := h.do("POST", "/api/v1/validate", `{"email":"person@example.com"}`)
	if res.status != 200 {
		t.Fatalf("%d %s", res.status, res.body)
	}
	m := res.json(t)
	if m["overall_status"] != "domain_deliverable" || m["mailbox"].(map[string]any)["status"] != "unverified" {
		t.Fatalf("%s", res.body)
	}
	if res.header.Get("Cache-Control") != "no-store" || res.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %v", res.header)
	}
}

func TestValidateOutcomes(t *testing.T) {
	h := newHarness(t, nil)
	for _, tc := range []struct {
		body, field, want string
	}{
		{`{"email":"bad..dots@example.com"}`, "overall_status", "invalid"},
		{`{"email":""}`, "overall_status", "invalid"},
		{`{"email":"a@nomail.example.org"}`, "overall_status", "undeliverable"},
		{`{"email":"a@unknown.example.org"}`, "overall_status", "unknown"},
		{`{"email":"a@example.com","checks":{"dns":false}}`, "overall_status", "syntax_valid"},
		{`{"email":"a@example.com","checks":{"syntax":true},"extra":1}`, "overall_status", "domain_deliverable"},
	} {
		res := h.do("POST", "/api/v1/validate", tc.body)
		if res.status != 200 || res.json(t)[tc.field] != tc.want {
			t.Errorf("%s: %d %s", tc.body, res.status, res.body)
		}
	}

	m := h.do("POST", "/api/v1/validate", `{"email":"support@mailinator.com"}`).json(t)
	if m["disposable"].(map[string]any)["status"] != "disposable" || m["role"].(map[string]any)["status"] != "role" {
		t.Fatalf("%v", m)
	}
	m = h.do("POST", "/api/v1/validate", `{"email":"jane@gmial.com"}`).json(t)
	if m["suggestion"].(map[string]any)["email"] != "jane@gmail.com" {
		t.Fatalf("%v", m)
	}
}

func TestResolverDetailsOnlyForAuthenticatedCallers(t *testing.T) {
	h := newHarness(t, nil)
	body := `{"email":"a@unknown.example.org"}`
	anon := h.do("POST", "/api/v1/validate", body).json(t)
	if r := anon["domain"].(map[string]any)["reason"]; r != "dns_unavailable" {
		t.Fatalf("anonymous reason %v", r)
	}
	authed := h.do("POST", "/api/v1/validate", body, "Authorization", "Bearer "+testKey).json(t)
	if r := authed["domain"].(map[string]any)["reason"]; r != "servfail" {
		t.Fatalf("authenticated reason %v", r)
	}
}

func TestValidateRequestErrors(t *testing.T) {
	h := newHarness(t, nil)
	for _, tc := range []struct {
		body, ct string
		status   int
		code     string
	}{
		{`{}`, "application/json", 400, "invalid_request"},
		{`{"email":42}`, "application/json", 400, "invalid_request"},
		{`not json`, "application/json", 400, "invalid_request"},
		{`{"email":"a@b.co"}{}`, "application/json", 400, "invalid_request"},
		{`{"email":"a@b.co"}`, "text/plain", 415, "unsupported_media_type"},
		{`{"email":"` + strings.Repeat("a", 10000) + `"}`, "application/json", 413, "payload_too_large"},
	} {
		res := h.do("POST", "/api/v1/validate", tc.body, "Content-Type", tc.ct)
		if res.status != tc.status || res.json(t)["error"].(map[string]any)["code"] != tc.code {
			t.Errorf("%.40s: %d %s", tc.body, res.status, res.body)
		}
	}
}

func TestAPIKeys(t *testing.T) {
	h := newHarness(t, nil)
	if res := h.do("POST", "/api/v1/validate", `{"email":"a@example.com"}`, "Authorization", "Bearer wrong-key-123456789"); res.status != 401 {
		t.Fatalf("wrong key accepted: %d", res.status)
	}
	if res := h.do("POST", "/api/v1/validate", `{"email":"a@example.com"}`, "X-API-Key", testKey); res.status != 200 {
		t.Fatalf("X-API-Key rejected: %d", res.status)
	}

	required := newHarness(t, func(c *config.Config) { c.RequireAPIKey = true })
	if res := required.do("POST", "/api/v1/validate", `{"email":"a@example.com"}`); res.status != 401 {
		t.Fatalf("anonymous accepted when keys required: %d", res.status)
	}
}

func TestRateLimits(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.RateLimitIP = 3; c.RateLimitAPIKey = 5 })
	body := `{"email":"a@example.com"}`
	for range 3 {
		if res := h.do("POST", "/api/v1/validate", body); res.status != 200 {
			t.Fatalf("within limit: %d", res.status)
		}
	}
	res := h.do("POST", "/api/v1/validate", body)
	if res.status != 429 || res.header.Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d %v", res.status, res.header)
	}
	// Authenticated callers use their own, separate budget.
	for i := range 5 {
		if res := h.do("POST", "/api/v1/validate", body, "X-API-Key", testKey); res.status != 200 {
			t.Fatalf("key request %d: %d", i, res.status)
		}
	}
	if res := h.do("POST", "/api/v1/validate", body, "X-API-Key", testKey); res.status != 429 {
		t.Fatalf("key limit not enforced: %d", res.status)
	}
	// Health checks are never rate limited.
	if res := h.do("GET", "/healthz", ""); res.status != 200 {
		t.Fatalf("healthz: %d", res.status)
	}
}

func TestForwardedForOnlyFromTrustedProxies(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.RateLimitIP = 1 })
	body := `{"email":"a@example.com"}`
	h.do("POST", "/api/v1/validate", body, "X-Forwarded-For", "198.51.100.1")
	// Without trusted proxies the header is ignored, so this is the same client.
	if res := h.do("POST", "/api/v1/validate", body, "X-Forwarded-For", "198.51.100.2"); res.status != 429 {
		t.Fatalf("spoofed X-Forwarded-For bypassed the limit: %d", res.status)
	}
}

func TestClientKey(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.9":          "203.0.113.9",
		"::ffff:203.0.113.9":   "203.0.113.9",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::/64",
		"not-an-ip":            "not-an-ip",
	} {
		if got := clientKey(in); got != want {
			t.Errorf("clientKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCORS(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.CORSOrigins = []string{"https://app.example.com"} })
	req := httptest.NewRequest("OPTIONS", "/api/v1/validate", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}

	req.Header.Set("Origin", "https://evil.example.net")
	rec = httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unlisted origin allowed")
	}
}

// --- verification -----------------------------------------------------------

func TestVerificationFlow(t *testing.T) {
	h := newHarness(t, nil)
	auth := []string{"Authorization", "Bearer " + testKey}

	if res := h.do("POST", "/api/v1/verification/send", `{"email":"a@example.com"}`); res.status != 401 {
		t.Fatalf("send without key: %d", res.status)
	}
	if res := h.do("POST", "/api/v1/verification/send", `{"email":"nope"}`, auth...); res.status != 400 {
		t.Fatalf("invalid email: %d", res.status)
	}
	res := h.do("POST", "/api/v1/verification/send", `{"email":"Person@example.com"}`, auth...)
	if res.status != 202 {
		t.Fatalf("send: %d %s", res.status, res.body)
	}
	select {
	case <-h.mail.sent:
	case <-time.After(2 * time.Second):
		t.Fatal("no email sent")
	}
	h.mail.mu.Lock()
	token := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(h.mail.msgs[0].Text)[1]
	h.mail.mu.Unlock()

	res = h.do("POST", "/api/v1/verification/confirm", `{"token":"`+token+`"}`, auth...)
	if res.status != 200 || res.json(t)["email"] != "Person@example.com" || res.json(t)["status"] != "verified" {
		t.Fatalf("confirm: %d %s", res.status, res.body)
	}
	res = h.do("POST", "/api/v1/verification/confirm", `{"token":"`+token+`"}`, auth...)
	if res.status != 400 || res.json(t)["error"].(map[string]any)["code"] != "invalid_token" {
		t.Fatalf("replay: %d %s", res.status, res.body)
	}
}

func TestVerificationResponsesDoNotRevealAddressState(t *testing.T) {
	h := newHarness(t, nil)
	auth := []string{"X-API-Key", testKey}
	var bodies []string
	// The fourth request for the same address is suppressed by the per-address
	// limit, but the response is identical.
	for range 4 {
		res := h.do("POST", "/api/v1/verification/send", `{"email":"x@example.com"}`, auth...)
		bodies = append(bodies, fmt.Sprintf("%d %s", res.status, res.body))
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatalf("responses differ: %q vs %q", bodies[0], b)
		}
	}
}

func TestVerificationDisabled(t *testing.T) {
	cfg, _ := config.Load(func(k string) string {
		if k == "VALIDLY_API_KEYS" {
			return "tests:" + testKey
		}
		return ""
	})
	h := &harness{t: t, handler: New(Deps{Config: cfg, Validator: &validation.Validator{}})}
	res := h.do("POST", "/api/v1/verification/send", `{"email":"a@example.com"}`, "X-API-Key", testKey)
	if res.status != 503 || res.json(t)["error"].(map[string]any)["code"] != "verification_disabled" {
		t.Fatalf("%d %s", res.status, res.body)
	}
	// Admin routes are absent without an admin token.
	if res := h.do("GET", "/api/v1/admin/disposable-domains", ""); res.status != 404 {
		t.Fatalf("admin route registered without token: %d", res.status)
	}
}

// --- admin ------------------------------------------------------------------

func TestAdminDomainLists(t *testing.T) {
	h := newHarness(t, nil)
	admin := []string{"Authorization", "Bearer " + adminTok}

	if res := h.do("GET", "/api/v1/admin/disposable-domains", ""); res.status != 401 {
		t.Fatalf("no token: %d", res.status)
	}
	if res := h.do("GET", "/api/v1/admin/disposable-domains", "", "Authorization", "Bearer "+testKey); res.status != 401 {
		t.Fatalf("API key accepted as admin token: %d", res.status)
	}

	res := h.do("PUT", "/api/v1/admin/disposable-domains/blocklist/Burner.Example.ORG", "", admin...)
	if res.status != 200 || res.json(t)["domain"] != "burner.example.org" {
		t.Fatalf("add: %d %s", res.status, res.body)
	}
	m := h.do("POST", "/api/v1/validate", `{"email":"a@burner.example.org","checks":{"dns":false}}`).json(t)
	if m["disposable"].(map[string]any)["status"] != "disposable" {
		t.Fatalf("blocklist not applied: %v", m)
	}

	res = h.do("GET", "/api/v1/admin/disposable-domains", "", admin...)
	if entries := res.json(t)["entries"].([]any); len(entries) != 1 {
		t.Fatalf("list: %s", res.body)
	}

	if res := h.do("PUT", "/api/v1/admin/disposable-domains/blocklist/not_a_domain", "", admin...); res.status != 400 {
		t.Fatalf("invalid domain: %d", res.status)
	}
	if res := h.do("PUT", "/api/v1/admin/disposable-domains/greylist/x.example.org", "", admin...); res.status != 404 {
		t.Fatalf("unknown list: %d", res.status)
	}
	if res := h.do("DELETE", "/api/v1/admin/disposable-domains/blocklist/burner.example.org", "", admin...); res.status != 204 {
		t.Fatalf("delete: %d", res.status)
	}
	if res := h.do("DELETE", "/api/v1/admin/disposable-domains/blocklist/burner.example.org", "", admin...); res.status != 404 {
		t.Fatalf("delete missing: %d", res.status)
	}
}

// --- operations -------------------------------------------------------------

func TestOperationalEndpoints(t *testing.T) {
	h := newHarness(t, nil)
	if res := h.do("GET", "/healthz", ""); res.status != 200 {
		t.Fatalf("healthz %d", res.status)
	}
	res := h.do("GET", "/readyz", "")
	checks := res.json(t)["checks"].(map[string]any)
	if res.status != 200 || checks["database"] != "ok" || checks["cache"] != "error" {
		t.Fatalf("readyz %d %s", res.status, res.body)
	}
	h.draining.Store(true)
	if res := h.do("GET", "/readyz", ""); res.status != 503 {
		t.Fatalf("readyz while draining %d", res.status)
	}

	res = h.do("GET", "/openapi.json", "")
	if res.status != 200 || !bytes.Equal(res.body, openapi.Spec) {
		t.Fatalf("openapi %d", res.status)
	}

	if res := h.do("GET", "/nope", ""); res.status != 404 || res.json(t)["error"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("404: %d %s", res.status, res.body)
	}
	if res := h.do("GET", "/api/v1/validate", ""); res.status != 405 {
		t.Fatalf("405: %d %s", res.status, res.body)
	}
}

func TestConcurrentRequests(t *testing.T) {
	h := newHarness(t, func(c *config.Config) { c.RateLimitIP = 0 })
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			req := httptest.NewRequest("POST", "/api/v1/validate", strings.NewReader(fmt.Sprintf(`{"email":"user%d@example.com"}`, i)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.handler.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Errorf("status %d", rec.Code)
			}
		})
	}
	wg.Wait()
}

func TestContractValidatorRejectsBadBodies(t *testing.T) {
	loadSpec(t)
	for ref, body := range map[string]string{
		"#/components/schemas/Health":           `{"status":"bad"}`,
		"#/components/schemas/ValidationResult": `{"email":"a@b.co"}`,
		"#/components/schemas/DomainResult":     `{"normalized":null,"status":"maybe","reason":null,"mx_found":null,"fallback_address_found":null}`,
		"#/components/schemas/Error":            `{"error":{"code":"teapot","message":""}}`,
	} {
		inst, _ := jsonschema.UnmarshalJSON(strings.NewReader(body))
		if err := schemaFor(t, ref).Validate(inst); err == nil {
			t.Errorf("%s accepted %s", ref, body)
		}
	}
}
