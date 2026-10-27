// Package validation composes the individual checks into one result.
//
// The result deliberately separates four questions that are often conflated:
// is the address well formed (syntax), does its domain appear to accept mail
// (domain), does this particular mailbox exist (never established here, so
// mailbox is always "unverified"), and does someone control it (only the
// ownership-verification workflow can answer that).
package validation

import (
	"context"

	"github.com/validly/validly/internal/disposable"
	"github.com/validly/validly/internal/dns"
	"github.com/validly/validly/internal/role"
	"github.com/validly/validly/internal/suggestions"
	"github.com/validly/validly/internal/syntax"
)

type SyntaxStatus string

const (
	SyntaxValid   SyntaxStatus = "valid"
	SyntaxInvalid SyntaxStatus = "invalid"
)

type DomainStatus string

const (
	DomainDeliverable   DomainStatus = "deliverable"
	DomainUndeliverable DomainStatus = "undeliverable"
	DomainUnknown       DomainStatus = "unknown"
	DomainSkipped       DomainStatus = "skipped"
)

// Domain reasons that are not DNS outcomes.
const (
	DomainReasonNotRequested   = "not_requested"
	DomainReasonSyntaxInvalid  = "syntax_invalid"
	DomainReasonDNSUnavailable = "dns_unavailable"
)

type DisposableStatus string

const (
	Disposable        DisposableStatus = "disposable"
	NotDisposable     DisposableStatus = "not_disposable"
	DisposableSkipped DisposableStatus = "skipped"
)

type RoleStatus string

const (
	Role        RoleStatus = "role"
	NotRole     RoleStatus = "not_role"
	RoleSkipped RoleStatus = "skipped"
)

type MailboxStatus string

const (
	MailboxUnverified MailboxStatus = "unverified"
	MailboxVerified   MailboxStatus = "verified"
)

type OverallStatus string

const (
	OverallInvalid           OverallStatus = "invalid"
	OverallUndeliverable     OverallStatus = "undeliverable"
	OverallUnknown           OverallStatus = "unknown"
	OverallDomainDeliverable OverallStatus = "domain_deliverable"
	// OverallSyntaxValid is returned when the DNS check was not requested.
	OverallSyntaxValid OverallStatus = "syntax_valid"
)

// Checks selects the optional stages. Syntax is always evaluated because
// every other stage depends on it.
type Checks struct {
	DNS        bool
	Disposable bool
	Role       bool
	Typos      bool
}

// AllChecks enables every stage.
func AllChecks() Checks { return Checks{DNS: true, Disposable: true, Role: true, Typos: true} }

type Result struct {
	Email         string           `json:"email"`
	Syntax        SyntaxResult     `json:"syntax"`
	Domain        DomainResult     `json:"domain"`
	Disposable    DisposableResult `json:"disposable"`
	Role          RoleResult       `json:"role"`
	Suggestion    *Suggestion      `json:"suggestion"`
	Mailbox       MailboxResult    `json:"mailbox"`
	OverallStatus OverallStatus    `json:"overall_status"`
}

type SyntaxResult struct {
	Status SyntaxStatus `json:"status"`
	Reason *string      `json:"reason"`
}

type DomainResult struct {
	Normalized           *string      `json:"normalized"`
	Status               DomainStatus `json:"status"`
	Reason               *string      `json:"reason"`
	MXFound              *bool        `json:"mx_found"`
	FallbackAddressFound *bool        `json:"fallback_address_found"`
}

type DisposableResult struct {
	Status DisposableStatus `json:"status"`
}

type RoleResult struct {
	Status RoleStatus `json:"status"`
}

type Suggestion struct {
	Domain string `json:"domain"`
	Email  string `json:"email"`
}

type MailboxResult struct {
	Status MailboxStatus `json:"status"`
}

// DomainChecker is satisfied by *dns.Checker.
type DomainChecker interface {
	Check(ctx context.Context, domain string) dns.Result
}

// DisposableChecker is satisfied by *disposable.Detector.
type DisposableChecker interface {
	Check(domain string) disposable.Status
}

// Validator runs the validation stages. Any dependency may be nil, in which
// case the corresponding stage reports "skipped".
type Validator struct {
	DNS         DomainChecker
	Disposable  DisposableChecker
	Suggestions *suggestions.Suggester
}

// Validate evaluates email. It never modifies the submitted address.
func (v *Validator) Validate(ctx context.Context, email string, checks Checks) Result {
	r := Result{
		Email:      email,
		Disposable: DisposableResult{Status: DisposableSkipped},
		Role:       RoleResult{Status: RoleSkipped},
		Mailbox:    MailboxResult{Status: MailboxUnverified},
	}

	parsed := syntax.Parse(email)
	if parsed.Domain != "" {
		r.Domain.Normalized = &parsed.Domain
	}
	if !parsed.Valid {
		r.Syntax = SyntaxResult{Status: SyntaxInvalid, Reason: str(string(parsed.Reason))}
		r.Domain.Status, r.Domain.Reason = DomainSkipped, str(DomainReasonSyntaxInvalid)
		r.OverallStatus = OverallInvalid
		return r
	}
	r.Syntax = SyntaxResult{Status: SyntaxValid}

	if checks.Disposable && v.Disposable != nil {
		r.Disposable.Status = DisposableStatus(v.Disposable.Check(parsed.Domain))
	}
	if checks.Role {
		r.Role.Status = NotRole
		if role.IsRole(parsed.Local) {
			r.Role.Status = Role
		}
	}
	if checks.Typos && v.Suggestions != nil {
		if d := v.Suggestions.Suggest(parsed.Domain); d != "" {
			r.Suggestion = &Suggestion{Domain: d, Email: parsed.Local + "@" + d}
		}
	}

	if !checks.DNS || v.DNS == nil {
		r.Domain.Status, r.Domain.Reason = DomainSkipped, str(DomainReasonNotRequested)
		r.OverallStatus = OverallSyntaxValid
		return r
	}

	d := v.DNS.Check(ctx, parsed.Domain)
	r.Domain.Status = DomainStatus(d.Status)
	r.Domain.Reason = str(string(d.Reason))
	r.Domain.MXFound = d.MXFound
	r.Domain.FallbackAddressFound = d.FallbackAddressFound
	switch d.Status {
	case dns.StatusDeliverable:
		r.OverallStatus = OverallDomainDeliverable
	case dns.StatusUndeliverable:
		r.OverallStatus = OverallUndeliverable
	default:
		r.OverallStatus = OverallUnknown
	}
	return r
}

// Redact hides resolver failure details, which unauthenticated callers do
// not need and could use to probe the resolver.
func (r *Result) Redact() {
	if r.Domain.Status == DomainUnknown {
		r.Domain.Reason = str(DomainReasonDNSUnavailable)
	}
}

func str(s string) *string { return &s }
