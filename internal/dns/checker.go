package dns

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/validly/validly/internal/cache"
)

// Config tunes a Checker.
type Config struct {
	// Timeout bounds one complete domain lookup (MX plus any fallback). Give
	// each individual query (see NewResolver) a fraction of it so that a dead
	// resolver or slow MX answer leaves time for failover and the fallback.
	Timeout time.Duration
	// MaxConcurrent bounds the number of domain lookups in flight.
	MaxConcurrent int
	// MinTTL and MaxTTL clamp how long definitive results are cached.
	MinTTL, MaxTTL time.Duration
	// UnknownTTL is how long indeterminate results are cached, which damps
	// load on a failing upstream. Zero disables caching them.
	UnknownTTL time.Duration
}

// DefaultConfig returns conservative defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:       3 * time.Second,
		MaxConcurrent: 64,
		MinTTL:        30 * time.Second,
		MaxTTL:        time.Hour,
		UnknownTTL:    10 * time.Second,
	}
}

// Observer receives lookup outcomes for metrics. Methods must be cheap.
type Observer interface {
	DNSLookup(status Status, reason Reason, duration time.Duration)
	CacheResult(name string, hit bool)
}

type nopObserver struct{}

func (nopObserver) DNSLookup(Status, Reason, time.Duration) {}
func (nopObserver) CacheResult(string, bool)                {}

// Checker resolves domains to a deliverability Result. It caches results,
// coalesces concurrent lookups of the same domain and bounds concurrency.
// It is safe for concurrent use.
type Checker struct {
	q        Querier
	cache    cache.Cache[Result]
	cfg      Config
	sem      chan struct{}
	group    singleflight.Group
	observer Observer
	log      *slog.Logger
}

// NewChecker returns a Checker. c may be nil to disable caching; obs may be nil.
func NewChecker(q Querier, c cache.Cache[Result], cfg Config, obs Observer, log *slog.Logger) *Checker {
	if cfg.MaxConcurrent < 1 {
		cfg.MaxConcurrent = 1
	}
	if obs == nil {
		obs = nopObserver{}
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Checker{
		q:        q,
		cache:    c,
		cfg:      cfg,
		sem:      make(chan struct{}, cfg.MaxConcurrent),
		observer: obs,
		log:      log,
	}
}

// Check reports whether domain appears able to receive mail. domain must be a
// lowercase ASCII domain that already passed syntax validation; the checker
// never resolves anything else.
func (c *Checker) Check(ctx context.Context, domain string) Result {
	if isReserved(domain) {
		return undeliverable(ReasonReservedDomain, nil, nil)
	}

	if c.cache != nil {
		r, ok, err := c.cache.Get(ctx, domain)
		if err != nil {
			c.log.WarnContext(ctx, "dns cache read failed", "error", err)
		}
		c.observer.CacheResult("dns", ok)
		if ok {
			return r
		}
	}

	// The shared lookup is detached from the first caller's context so that
	// one caller giving up does not fail the others; it is still bounded by
	// the configured timeout.
	ch := c.group.DoChan(domain, func() (any, error) {
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.cfg.Timeout)
		defer cancel()
		return c.lookupAndCache(lctx, domain), nil
	})
	select {
	case res := <-ch:
		return res.Val.(Result)
	case <-ctx.Done():
		return unknown(ReasonTimeout)
	}
}

func (c *Checker) lookupAndCache(ctx context.Context, domain string) Result {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		r := unknown(ReasonTimeout)
		c.observer.DNSLookup(r.Status, r.Reason, 0)
		return r
	}

	start := time.Now()
	r, ttl := c.lookup(ctx, domain)
	c.observer.DNSLookup(r.Status, r.Reason, time.Since(start))

	if c.cache != nil {
		if r.Status == StatusUnknown {
			ttl = c.cfg.UnknownTTL
		} else {
			ttl = min(max(ttl, c.cfg.MinTTL), c.cfg.MaxTTL)
		}
		// The lookup context may already have expired (e.g. after a timeout);
		// the cache applies its own short deadline.
		if err := c.cache.Set(context.WithoutCancel(ctx), domain, r, ttl); err != nil {
			c.log.WarnContext(ctx, "dns cache write failed", "error", err)
		}
	}
	return r
}

// lookup performs the RFC 5321 resolution and returns the result with the
// TTL it may be cached for.
func (c *Checker) lookup(ctx context.Context, domain string) (Result, time.Duration) {
	mx, err := c.q.Query(ctx, domain, TypeMX)
	if err != nil {
		return unknownFromErr(err), 0
	}
	switch mx.Rcode {
	case RcodeSuccess:
	case RcodeNXDomain:
		return undeliverable(ReasonNXDomain, ptr(false), ptr(false)), mx.TTL
	case RcodeServFail:
		return unknown(ReasonServFail), 0
	default:
		return unknown(ReasonResolverError), 0
	}

	if len(mx.MX) > 0 {
		hasNull, hasHost := false, false
		for _, r := range mx.MX {
			if r.Host == "" {
				hasNull = true
			} else {
				hasHost = true
			}
		}
		// RFC 7505: a null MX must be the only MX record. If a domain
		// publishes real exchangers alongside it, the real ones win.
		if hasHost {
			return deliverable(ReasonMX, ptr(true), ptr(false)), mx.TTL
		}
		if hasNull {
			return undeliverable(ReasonNullMX, ptr(true), ptr(false)), mx.TTL
		}
	}

	// NODATA for MX: fall back to the domain's own addresses.
	type res struct {
		a   Answer
		err error
	}
	aCh, aaaaCh := make(chan res, 1), make(chan res, 1)
	go func() { a, err := c.q.Query(ctx, domain, TypeA); aCh <- res{a, err} }()
	go func() { a, err := c.q.Query(ctx, domain, TypeAAAA); aaaaCh <- res{a, err} }()
	answers := []res{<-aCh, <-aaaaCh}

	var (
		public, nonPublic bool
		failure           *res
		ttl               = mx.TTL
	)
	for i := range answers {
		r := answers[i]
		if r.err != nil || (r.a.Rcode != RcodeSuccess && r.a.Rcode != RcodeNXDomain) {
			failure = &answers[i]
			continue
		}
		if r.a.TTL > 0 && (ttl == 0 || r.a.TTL < ttl) {
			ttl = r.a.TTL
		}
		for _, addr := range r.a.Addrs {
			if isPublic(addr) {
				public = true
			} else {
				nonPublic = true
			}
		}
	}

	switch {
	case public:
		return deliverable(ReasonAddressFallback, ptr(false), ptr(true)), ttl
	case failure != nil:
		// One family failed and the other found nothing usable: we cannot
		// rule out a working fallback.
		if failure.err != nil {
			return unknownFromErr(failure.err), 0
		}
		if failure.a.Rcode == RcodeServFail {
			return unknown(ReasonServFail), 0
		}
		return unknown(ReasonResolverError), 0
	case nonPublic:
		return undeliverable(ReasonNonPublicAddress, ptr(false), ptr(true)), ttl
	default:
		return undeliverable(ReasonNoMailRecords, ptr(false), ptr(false)), ttl
	}
}

func unknownFromErr(err error) Result {
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return unknown(ReasonTimeout)
	}
	return unknown(ReasonResolverError)
}

func deliverable(r Reason, mx, fb *bool) Result {
	return Result{Status: StatusDeliverable, Reason: r, MXFound: mx, FallbackAddressFound: fb}
}

func undeliverable(r Reason, mx, fb *bool) Result {
	return Result{Status: StatusUndeliverable, Reason: r, MXFound: mx, FallbackAddressFound: fb}
}

func unknown(r Reason) Result { return Result{Status: StatusUnknown, Reason: r} }

func ptr(b bool) *bool { return &b }

// reservedSuffixes are special-use names (RFC 2606, 6761, 6762, 7686, 8375,
// 9476 and the ICANN-reserved .internal) that never route public email.
// Rejecting them up front also keeps the service from querying names that
// only resolve inside a private network.
var reservedSuffixes = []string{
	"localhost", "test", "invalid", "example", "local", "onion", "alt", "internal", "arpa",
}

func isReserved(domain string) bool {
	tld := domain[strings.LastIndexByte(domain, '.')+1:]
	for _, s := range reservedSuffixes {
		if tld == s {
			return true
		}
	}
	return false
}

// isPublic reports whether addr is a globally routable unicast address.
func isPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}
	for _, p := range nonPublicPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64
}
