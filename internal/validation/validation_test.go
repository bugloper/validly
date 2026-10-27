package validation

import (
	"context"
	"testing"

	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/dns"
	"github.com/validly/validly/internal/suggestions"
)

type fixedDNS map[string]dns.Result

func (f fixedDNS) Check(_ context.Context, domain string) dns.Result {
	if r, ok := f[domain]; ok {
		return r
	}
	return dns.Result{Status: dns.StatusUnknown, Reason: dns.ReasonTimeout}
}

type fixedDisposable map[string]bool

func (f fixedDisposable) Check(domain string) disposable.Status {
	if f[domain] {
		return disposable.StatusDisposable
	}
	return disposable.StatusNotDisposable
}

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

func validator() *Validator {
	return &Validator{
		DNS: fixedDNS{
			"example.com":    {Status: dns.StatusDeliverable, Reason: dns.ReasonMX, MXFound: yes(), FallbackAddressFound: no()},
			"gmial.com":      {Status: dns.StatusDeliverable, Reason: dns.ReasonMX, MXFound: yes(), FallbackAddressFound: no()},
			"null.example":   {Status: dns.StatusUndeliverable, Reason: dns.ReasonNullMX, MXFound: yes(), FallbackAddressFound: no()},
			"mailinator.com": {Status: dns.StatusDeliverable, Reason: dns.ReasonMX, MXFound: yes(), FallbackAddressFound: no()},
		},
		Disposable:  fixedDisposable{"mailinator.com": true},
		Suggestions: suggestions.New(suggestions.DefaultDomains),
	}
}

func TestValidate(t *testing.T) {
	ctx := context.Background()
	v := validator()

	r := v.Validate(ctx, "person@Example.com", AllChecks())
	if r.Email != "person@Example.com" {
		t.Fatalf("input modified: %q", r.Email)
	}
	if r.Syntax.Status != SyntaxValid || r.Syntax.Reason != nil {
		t.Fatalf("syntax %+v", r.Syntax)
	}
	if *r.Domain.Normalized != "example.com" || r.Domain.Status != DomainDeliverable || !*r.Domain.MXFound {
		t.Fatalf("domain %+v", r.Domain)
	}
	if r.OverallStatus != OverallDomainDeliverable || r.Mailbox.Status != MailboxUnverified {
		t.Fatalf("overall %s mailbox %s", r.OverallStatus, r.Mailbox.Status)
	}
	if r.Disposable.Status != NotDisposable || r.Role.Status != NotRole || r.Suggestion != nil {
		t.Fatalf("%+v", r)
	}
}

func TestValidateInvalidSyntaxSkipsEverything(t *testing.T) {
	r := validator().Validate(context.Background(), "person..x@example.com", AllChecks())
	if r.OverallStatus != OverallInvalid || r.Syntax.Status != SyntaxInvalid || *r.Syntax.Reason != "local_part_dot_placement" {
		t.Fatalf("%+v", r)
	}
	if r.Domain.Status != DomainSkipped || *r.Domain.Reason != DomainReasonSyntaxInvalid || *r.Domain.Normalized != "example.com" {
		t.Fatalf("domain %+v", r.Domain)
	}
	if r.Disposable.Status != DisposableSkipped || r.Role.Status != RoleSkipped {
		t.Fatalf("%+v", r)
	}

	r = validator().Validate(context.Background(), "no-at-sign", AllChecks())
	if r.Domain.Normalized != nil {
		t.Fatal("normalized domain set for unparseable input")
	}
}

func TestValidateStatuses(t *testing.T) {
	ctx := context.Background()
	v := validator()

	if r := v.Validate(ctx, "a@null.example", AllChecks()); r.OverallStatus != OverallUndeliverable {
		t.Fatalf("null mx: %s", r.OverallStatus)
	}
	r := v.Validate(ctx, "a@unknown.example.org", AllChecks())
	if r.OverallStatus != OverallUnknown || *r.Domain.Reason != "timeout" || r.Domain.MXFound != nil {
		t.Fatalf("unknown: %+v", r.Domain)
	}
	r.Redact()
	if *r.Domain.Reason != DomainReasonDNSUnavailable {
		t.Fatalf("redacted reason %q", *r.Domain.Reason)
	}

	if r := v.Validate(ctx, "a@mailinator.com", AllChecks()); r.Disposable.Status != Disposable {
		t.Fatalf("disposable: %s", r.Disposable.Status)
	}
	if r := v.Validate(ctx, "Support+x@example.com", AllChecks()); r.Role.Status != Role {
		t.Fatalf("role: %s", r.Role.Status)
	}
	r = v.Validate(ctx, "Jane.Doe@gmial.com", AllChecks())
	if r.Suggestion == nil || r.Suggestion.Email != "Jane.Doe@gmail.com" || r.Suggestion.Domain != "gmail.com" {
		t.Fatalf("suggestion: %+v", r.Suggestion)
	}
}

func TestValidateRespectsChecks(t *testing.T) {
	r := validator().Validate(context.Background(), "support@gmial.com", Checks{})
	if r.OverallStatus != OverallSyntaxValid || r.Domain.Status != DomainSkipped || *r.Domain.Reason != DomainReasonNotRequested {
		t.Fatalf("%+v", r)
	}
	if r.Role.Status != RoleSkipped || r.Disposable.Status != DisposableSkipped || r.Suggestion != nil {
		t.Fatalf("%+v", r)
	}
}

func TestValidateWithoutDependencies(t *testing.T) {
	r := (&Validator{}).Validate(context.Background(), "a@example.com", AllChecks())
	if r.OverallStatus != OverallSyntaxValid || r.Disposable.Status != DisposableSkipped {
		t.Fatalf("%+v", r)
	}
}
