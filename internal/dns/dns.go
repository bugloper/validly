// Package dns decides whether a domain appears able to receive email.
//
// The check follows RFC 5321 section 5.1: use MX records if present; if the
// domain exists but has no MX records, fall back to its A/AAAA records (the
// "implicit MX"). A null MX (RFC 7505) means the domain explicitly accepts no
// mail. A deliverable result says only that the domain routes mail somewhere;
// it never proves that a particular mailbox exists.
//
// Transport failures, timeouts and SERVFAIL responses produce StatusUnknown.
// A lookup that could not finish is never reported as deliverable.
package dns

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

// Status is the outcome of a domain check.
type Status string

const (
	StatusDeliverable   Status = "deliverable"
	StatusUndeliverable Status = "undeliverable"
	StatusUnknown       Status = "unknown"
)

// Reason gives the detail behind a Status.
type Reason string

const (
	// Deliverable.
	ReasonMX              Reason = "mx_records"
	ReasonAddressFallback Reason = "address_fallback"

	// Undeliverable.
	ReasonNullMX           Reason = "null_mx"
	ReasonNXDomain         Reason = "nxdomain"
	ReasonNoMailRecords    Reason = "no_mail_records"
	ReasonNonPublicAddress Reason = "non_public_address"
	ReasonReservedDomain   Reason = "reserved_domain"

	// Unknown.
	ReasonTimeout       Reason = "timeout"
	ReasonServFail      Reason = "servfail"
	ReasonResolverError Reason = "resolver_error"
)

// Result is the outcome of checking one domain.
type Result struct {
	Status Status `json:"status"`
	Reason Reason `json:"reason"`
	// MXFound and FallbackAddressFound are nil when the lookup could not
	// establish an answer.
	MXFound              *bool `json:"mx_found"`
	FallbackAddressFound *bool `json:"fallback_address_found"`
}

// QType is a DNS record type that the checker queries.
type QType uint16

const (
	TypeA    QType = 1
	TypeMX   QType = 15
	TypeAAAA QType = 28
)

func (t QType) String() string {
	switch t {
	case TypeA:
		return "A"
	case TypeMX:
		return "MX"
	case TypeAAAA:
		return "AAAA"
	}
	return "UNKNOWN"
}

// Rcode is a simplified DNS response code.
type Rcode int

const (
	RcodeSuccess Rcode = iota
	RcodeNXDomain
	RcodeServFail
	RcodeRefused
	RcodeOther
)

// MX is a single mail exchanger record.
type MX struct {
	// Host is lowercase without a trailing dot; "" denotes the root (".").
	Host string
	Pref uint16
}

// Answer is the parsed response to one query.
type Answer struct {
	Rcode Rcode
	MX    []MX
	Addrs []netip.Addr
	// TTL is the minimum TTL of the answer records or, for negative answers,
	// the negative-caching TTL from the SOA record. Zero means unknown.
	TTL time.Duration
}

// ErrTimeout is returned (possibly wrapped) by a Querier whose query timed out.
var ErrTimeout = errors.New("dns query timed out")

// Querier performs a single DNS query. A non-nil error means no usable
// response was received at all.
type Querier interface {
	Query(ctx context.Context, name string, qtype QType) (Answer, error)
}
