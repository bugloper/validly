// Package syntax validates the structure of an email address.
//
// Validly accepts the dot-atom form of RFC 5321/5322 addresses with an ASCII
// local part and an ASCII or internationalized (IDNA) domain. Quoted local
// parts, domain literals and SMTPUTF8 (non-ASCII) local parts are rejected with
// a dedicated reason so callers can tell "unsupported" apart from "malformed".
//
// The submitted address is never modified: Parse reports the normalized ASCII
// domain separately and leaves the input untouched.
package syntax

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

// Reason explains why an address failed syntax validation.
type Reason string

const (
	ReasonEmpty                    Reason = "empty"
	ReasonTooLong                  Reason = "too_long"
	ReasonWhitespace               Reason = "contains_whitespace"
	ReasonMissingAt                Reason = "missing_at_sign"
	ReasonMultipleAt               Reason = "multiple_at_signs"
	ReasonLocalEmpty               Reason = "local_part_empty"
	ReasonLocalTooLong             Reason = "local_part_too_long"
	ReasonLocalInvalidCharacter    Reason = "local_part_invalid_character"
	ReasonLocalDotPlacement        Reason = "local_part_dot_placement"
	ReasonQuotedLocalUnsupported   Reason = "quoted_local_part_unsupported"
	ReasonNonASCIILocalUnsupported Reason = "non_ascii_local_part_unsupported"
	ReasonDomainEmpty              Reason = "domain_empty"
	ReasonDomainLiteralUnsupported Reason = "domain_literal_unsupported"
	ReasonDomainInvalid            Reason = "domain_invalid"
	ReasonDomainTooLong            Reason = "domain_too_long"
	ReasonDomainLabelTooLong       Reason = "domain_label_too_long"
	ReasonDomainNotQualified       Reason = "domain_not_fully_qualified"
)

// Length limits, in octets, from RFC 5321 section 4.5.3.1. MaxAddressLength is
// the 256-octet path limit minus the angle brackets.
const (
	MaxAddressLength = 254
	MaxLocalLength   = 64
	MaxDomainLength  = 253
	MaxLabelLength   = 63

	// maxInputLength bounds the work done on raw input before IDNA mapping.
	// Anything longer cannot produce a valid address.
	maxInputLength = 512
)

// Result is the outcome of Parse.
type Result struct {
	Valid  bool
	Reason Reason // empty when Valid

	Local string
	// Domain is the lowercase ASCII (A-label) form of the domain. It is set
	// whenever the domain itself parsed, even if the local part is invalid.
	Domain string
}

var idnaProfile = idna.New(
	idna.MapForLookup(),
	idna.BidiRule(),
	idna.ValidateLabels(true),
	idna.StrictDomainName(true),
	idna.Transitional(false),
)

// Parse validates email and returns its parts. It performs no network access.
func Parse(email string) Result {
	if email == "" {
		return invalid(ReasonEmpty)
	}
	if len(email) > maxInputLength {
		return invalid(ReasonTooLong)
	}
	if !utf8.ValidString(email) {
		return invalid(ReasonLocalInvalidCharacter)
	}
	if strings.IndexFunc(email, unicode.IsSpace) >= 0 {
		return invalid(ReasonWhitespace)
	}
	if email[0] == '"' {
		return invalid(ReasonQuotedLocalUnsupported)
	}

	at := strings.IndexByte(email, '@')
	if at < 0 {
		return invalid(ReasonMissingAt)
	}
	if strings.IndexByte(email[at+1:], '@') >= 0 {
		return invalid(ReasonMultipleAt)
	}
	local, rawDomain := email[:at], email[at+1:]

	domain, reason := parseDomain(rawDomain)
	if reason != "" {
		return invalid(reason)
	}
	if reason := checkLocal(local); reason != "" {
		return Result{Reason: reason, Domain: domain}
	}
	if len(local)+1+len(domain) > MaxAddressLength {
		return Result{Reason: ReasonTooLong, Domain: domain}
	}
	return Result{Valid: true, Local: local, Domain: domain}
}

// NormalizeDomain validates a bare domain and returns its lowercase ASCII form.
// It applies the same rules as the domain part of Parse.
func NormalizeDomain(domain string) (string, Reason) {
	if len(domain) > maxInputLength {
		return "", ReasonDomainTooLong
	}
	if strings.IndexFunc(domain, unicode.IsSpace) >= 0 {
		return "", ReasonWhitespace
	}
	return parseDomain(domain)
}

func invalid(r Reason) Result { return Result{Reason: r} }

func checkLocal(local string) Reason {
	if local == "" {
		return ReasonLocalEmpty
	}
	for i := 0; i < len(local); i++ {
		if local[i] >= utf8.RuneSelf {
			return ReasonNonASCIILocalUnsupported
		}
	}
	if len(local) > MaxLocalLength {
		return ReasonLocalTooLong
	}
	for i := 0; i < len(local); i++ {
		if local[i] != '.' && !isAtext(local[i]) {
			return ReasonLocalInvalidCharacter
		}
	}
	if local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return ReasonLocalDotPlacement
	}
	return ""
}

// isAtext reports whether c is an RFC 5322 atext character.
func isAtext(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-/=?^_`{|}~", c) >= 0
}

func parseDomain(raw string) (string, Reason) {
	if raw == "" {
		return "", ReasonDomainEmpty
	}
	if raw[0] == '[' {
		return "", ReasonDomainLiteralUnsupported
	}
	// A trailing dot (fully-qualified root) is not accepted in addresses.
	if strings.HasSuffix(raw, ".") {
		return "", ReasonDomainInvalid
	}
	ascii, err := idnaProfile.ToASCII(raw)
	if err != nil || ascii == "" {
		return "", ReasonDomainInvalid
	}
	ascii = strings.ToLower(ascii)
	if len(ascii) > MaxDomainLength {
		return "", ReasonDomainTooLong
	}
	labels := strings.Split(ascii, ".")
	for _, l := range labels {
		if l == "" {
			return "", ReasonDomainInvalid
		}
		if len(l) > MaxLabelLength {
			return "", ReasonDomainLabelTooLong
		}
		if !isLDHLabel(l) {
			return "", ReasonDomainInvalid
		}
	}
	if len(labels) < 2 {
		return "", ReasonDomainNotQualified
	}
	// An all-numeric top-level label would make the domain indistinguishable
	// from an IPv4 address.
	if isNumeric(labels[len(labels)-1]) {
		return "", ReasonDomainInvalid
	}
	return ascii, ""
}

func isLDHLabel(l string) bool {
	if l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func isNumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
