package validly

// SyntaxStatus is the result of syntax validation.
type SyntaxStatus string

const (
	SyntaxValid   SyntaxStatus = "valid"
	SyntaxInvalid SyntaxStatus = "invalid"
)

// DomainStatus describes whether the domain appears able to receive mail.
// DomainDeliverable never means that a particular mailbox exists.
type DomainStatus string

const (
	DomainDeliverable   DomainStatus = "deliverable"
	DomainUndeliverable DomainStatus = "undeliverable"
	DomainUnknown       DomainStatus = "unknown"
	DomainSkipped       DomainStatus = "skipped"
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

// OverallStatus summarises syntax and domain results. Disposable, role and
// suggestion results do not affect it; apply your own policy to them.
type OverallStatus string

const (
	OverallInvalid           OverallStatus = "invalid"
	OverallUndeliverable     OverallStatus = "undeliverable"
	OverallUnknown           OverallStatus = "unknown"
	OverallDomainDeliverable OverallStatus = "domain_deliverable"
	OverallSyntaxValid       OverallStatus = "syntax_valid"
)

// Result is the response of the validate endpoint.
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
	// Reason is set when Status is SyntaxInvalid, e.g. "local_part_too_long".
	Reason *string `json:"reason"`
}

type DomainResult struct {
	// Normalized is the lowercase ASCII domain, or nil if it did not parse.
	Normalized *string      `json:"normalized"`
	Status     DomainStatus `json:"status"`
	Reason     *string      `json:"reason"`
	// MXFound and FallbackAddressFound are nil when DNS was skipped or could
	// not produce a reliable answer.
	MXFound              *bool `json:"mx_found"`
	FallbackAddressFound *bool `json:"fallback_address_found"`
}

type DisposableResult struct {
	Status DisposableStatus `json:"status"`
}

type RoleResult struct {
	Status RoleStatus `json:"status"`
}

// Suggestion proposes a likely intended address. It is never applied
// automatically; ask the user to confirm it.
type Suggestion struct {
	Domain string `json:"domain"`
	Email  string `json:"email"`
}

type MailboxResult struct {
	Status MailboxStatus `json:"status"`
}

// Checks disables optional stages. The zero value runs every check.
type Checks struct {
	SkipDNS        bool
	SkipDisposable bool
	SkipRole       bool
	SkipTypos      bool
}

type checksBody struct {
	DNS        bool `json:"dns"`
	Disposable bool `json:"disposable"`
	Role       bool `json:"role"`
	Typos      bool `json:"typos"`
}

func (c Checks) body() checksBody {
	return checksBody{DNS: !c.SkipDNS, Disposable: !c.SkipDisposable, Role: !c.SkipRole, Typos: !c.SkipTypos}
}
