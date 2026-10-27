package dns

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/validly/validly/internal/cache"
)

// fakeQuerier answers from a static table and counts queries.
type fakeQuerier struct {
	mu      sync.Mutex
	answers map[string]Answer
	errs    map[string]error
	delay   time.Duration
	block   chan struct{} // if non-nil, queries wait on it
	calls   atomic.Int64
	active  atomic.Int64
	peak    atomic.Int64
}

func key(name string, t QType) string { return name + "/" + t.String() }

func newFake() *fakeQuerier {
	return &fakeQuerier{answers: map[string]Answer{}, errs: map[string]error{}}
}

func (f *fakeQuerier) set(name string, t QType, a Answer)     { f.answers[key(name, t)] = a }
func (f *fakeQuerier) setErr(name string, t QType, err error) { f.errs[key(name, t)] = err }

func (f *fakeQuerier) Query(ctx context.Context, name string, t QType) (Answer, error) {
	f.calls.Add(1)
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return Answer{}, ErrTimeout
		}
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return Answer{}, fmt.Errorf("%w: fake", ErrTimeout)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errs[key(name, t)]; ok {
		return Answer{}, err
	}
	if a, ok := f.answers[key(name, t)]; ok {
		return a, nil
	}
	// Default: the name exists with no records of this type (NODATA).
	return Answer{Rcode: RcodeSuccess, TTL: 300 * time.Second}, nil
}

func addrs(s ...string) []netip.Addr {
	out := make([]netip.Addr, len(s))
	for i, a := range s {
		out[i] = netip.MustParseAddr(a)
	}
	return out
}

func newChecker(q Querier, c cache.Cache[Result]) *Checker {
	cfg := DefaultConfig()
	cfg.Timeout = 500 * time.Millisecond
	return NewChecker(q, c, cfg, nil, nil)
}

func TestCheck(t *testing.T) {
	errBoom := errors.New("connection refused")
	tests := []struct {
		name   string
		setup  func(f *fakeQuerier)
		status Status
		reason Reason
		mx, fb *bool
	}{
		{
			name: "mx records",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeMX, Answer{MX: []MX{{Host: "mx.d.com", Pref: 10}}, TTL: time.Hour})
			},
			status: StatusDeliverable, reason: ReasonMX, mx: ptr(true), fb: ptr(false),
		},
		{
			name: "null mx",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeMX, Answer{MX: []MX{{Host: "", Pref: 0}}})
			},
			status: StatusUndeliverable, reason: ReasonNullMX, mx: ptr(true), fb: ptr(false),
		},
		{
			name: "null mx alongside real exchanger",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeMX, Answer{MX: []MX{{Host: ""}, {Host: "mx.d.com", Pref: 10}}})
			},
			status: StatusDeliverable, reason: ReasonMX, mx: ptr(true), fb: ptr(false),
		},
		{
			name: "nxdomain",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeMX, Answer{Rcode: RcodeNXDomain})
			},
			status: StatusUndeliverable, reason: ReasonNXDomain, mx: ptr(false), fb: ptr(false),
		},
		{
			name: "nodata falls back to A",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeA, Answer{Addrs: addrs("93.184.215.14")})
			},
			status: StatusDeliverable, reason: ReasonAddressFallback, mx: ptr(false), fb: ptr(true),
		},
		{
			name: "nodata falls back to AAAA",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeAAAA, Answer{Addrs: addrs("2606:2800:21f:cb07:6820:80da:af6b:8b2c")})
			},
			status: StatusDeliverable, reason: ReasonAddressFallback, mx: ptr(false), fb: ptr(true),
		},
		{
			name:   "nodata everywhere",
			setup:  func(f *fakeQuerier) {},
			status: StatusUndeliverable, reason: ReasonNoMailRecords, mx: ptr(false), fb: ptr(false),
		},
		{
			name: "fallback only to private addresses",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeA, Answer{Addrs: addrs("127.0.0.1", "10.0.0.1")})
				f.set("d.com", TypeAAAA, Answer{Addrs: addrs("::1", "fd00::1")})
			},
			status: StatusUndeliverable, reason: ReasonNonPublicAddress, mx: ptr(false), fb: ptr(true),
		},
		{
			name: "mx servfail",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeMX, Answer{Rcode: RcodeServFail})
			},
			status: StatusUnknown, reason: ReasonServFail,
		},
		{
			name: "mx refused",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeMX, Answer{Rcode: RcodeRefused})
			},
			status: StatusUnknown, reason: ReasonResolverError,
		},
		{
			name: "mx timeout",
			setup: func(f *fakeQuerier) {
				f.setErr("d.com", TypeMX, fmt.Errorf("%w: 1.1.1.1:53", ErrTimeout))
			},
			status: StatusUnknown, reason: ReasonTimeout,
		},
		{
			name: "mx transport error",
			setup: func(f *fakeQuerier) {
				f.setErr("d.com", TypeMX, errBoom)
			},
			status: StatusUnknown, reason: ReasonResolverError,
		},
		{
			name: "A timeout and AAAA empty is unknown, never deliverable",
			setup: func(f *fakeQuerier) {
				f.setErr("d.com", TypeA, ErrTimeout)
			},
			status: StatusUnknown, reason: ReasonTimeout,
		},
		{
			name: "A servfail and AAAA empty is unknown",
			setup: func(f *fakeQuerier) {
				f.set("d.com", TypeA, Answer{Rcode: RcodeServFail})
			},
			status: StatusUnknown, reason: ReasonServFail,
		},
		{
			name: "A timeout but AAAA public is deliverable",
			setup: func(f *fakeQuerier) {
				f.setErr("d.com", TypeA, ErrTimeout)
				f.set("d.com", TypeAAAA, Answer{Addrs: addrs("2a00:1450:4001:80b::200e")})
			},
			status: StatusDeliverable, reason: ReasonAddressFallback, mx: ptr(false), fb: ptr(true),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake()
			tc.setup(f)
			got := newChecker(f, nil).Check(context.Background(), "d.com")
			if got.Status != tc.status || got.Reason != tc.reason {
				t.Fatalf("got %s/%s, want %s/%s", got.Status, got.Reason, tc.status, tc.reason)
			}
			if !eqPtr(got.MXFound, tc.mx) || !eqPtr(got.FallbackAddressFound, tc.fb) {
				t.Fatalf("mx=%v fb=%v, want mx=%v fb=%v", fmtPtr(got.MXFound), fmtPtr(got.FallbackAddressFound), fmtPtr(tc.mx), fmtPtr(tc.fb))
			}
		})
	}
}

func eqPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func fmtPtr(b *bool) string {
	if b == nil {
		return "nil"
	}
	return fmt.Sprint(*b)
}

func TestReservedDomainsAreNotQueried(t *testing.T) {
	f := newFake()
	c := newChecker(f, nil)
	for _, d := range []string{"printer.local", "foo.localhost", "a.test", "corp.internal", "x.onion", "router.home.arpa", "a.example"} {
		r := c.Check(context.Background(), d)
		if r.Status != StatusUndeliverable || r.Reason != ReasonReservedDomain {
			t.Errorf("%s: got %+v", d, r)
		}
	}
	if n := f.calls.Load(); n != 0 {
		t.Fatalf("made %d queries for reserved names", n)
	}
}

func TestCacheUsesTTL(t *testing.T) {
	f := newFake()
	f.set("d.com", TypeMX, Answer{MX: []MX{{Host: "mx.d.com"}}, TTL: 5 * time.Second})
	c := &spyCache{Memory: cache.NewMemory[Result](10)}
	ch := newChecker(f, c)

	ch.Check(context.Background(), "d.com")
	ch.Check(context.Background(), "d.com")
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("expected 1 query, got %d", n)
	}
	// 5s answer TTL is raised to the 30s floor.
	if c.lastTTL != 30*time.Second {
		t.Fatalf("ttl %v", c.lastTTL)
	}

	f.set("e.com", TypeMX, Answer{MX: []MX{{Host: "mx.e.com"}}, TTL: 48 * time.Hour})
	ch.Check(context.Background(), "e.com")
	if c.lastTTL != time.Hour {
		t.Fatalf("ttl %v, want clamp to 1h", c.lastTTL)
	}

	f.set("u.com", TypeMX, Answer{Rcode: RcodeServFail})
	ch.Check(context.Background(), "u.com")
	if c.lastTTL != 10*time.Second {
		t.Fatalf("unknown ttl %v", c.lastTTL)
	}
}

type spyCache struct {
	*cache.Memory[Result]
	lastTTL time.Duration
}

func (s *spyCache) Set(ctx context.Context, k string, v Result, ttl time.Duration) error {
	s.lastTTL = ttl
	return s.Memory.Set(ctx, k, v, ttl)
}

type failingCache struct{}

func (failingCache) Get(context.Context, string) (Result, bool, error) {
	return Result{}, false, errors.New("cache down")
}
func (failingCache) Set(context.Context, string, Result, time.Duration) error {
	return errors.New("cache down")
}

func TestCacheFailureIsIgnored(t *testing.T) {
	f := newFake()
	f.set("d.com", TypeMX, Answer{MX: []MX{{Host: "mx.d.com"}}})
	r := newChecker(f, failingCache{}).Check(context.Background(), "d.com")
	if r.Status != StatusDeliverable {
		t.Fatalf("got %+v", r)
	}
}

func TestConcurrentLookupsAreCoalesced(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{})
	f.set("d.com", TypeMX, Answer{MX: []MX{{Host: "mx.d.com"}}})
	c := newChecker(f, nil)

	var wg sync.WaitGroup
	results := make([]Result, 50)
	for i := range results {
		wg.Go(func() { results[i] = c.Check(context.Background(), "d.com") })
	}
	// Give the goroutines time to join the in-flight lookup.
	time.Sleep(50 * time.Millisecond)
	close(f.block)
	wg.Wait()

	if n := f.calls.Load(); n != 1 {
		t.Fatalf("expected 1 query, got %d", n)
	}
	for _, r := range results {
		if r.Status != StatusDeliverable {
			t.Fatalf("got %+v", r)
		}
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	f := newFake()
	f.delay = 20 * time.Millisecond
	cfg := DefaultConfig()
	cfg.MaxConcurrent = 3
	cfg.Timeout = 5 * time.Second
	c := NewChecker(f, nil, cfg, nil, nil)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() { c.Check(context.Background(), fmt.Sprintf("d%d.com", i)) })
	}
	wg.Wait()
	// Each lookup holds one slot and issues at most two queries at once
	// (A and AAAA fallback).
	if p := f.peak.Load(); p > 6 {
		t.Fatalf("peak concurrent queries %d exceeds bound", p)
	}
}

func TestLookupTimeout(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{}) // never released
	cfg := DefaultConfig()
	cfg.Timeout = 50 * time.Millisecond
	c := NewChecker(f, nil, cfg, nil, nil)

	start := time.Now()
	r := c.Check(context.Background(), "slow.com")
	if r.Status != StatusUnknown || r.Reason != ReasonTimeout {
		t.Fatalf("got %+v", r)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("timeout not enforced: %v", d)
	}
}

func TestCallerCancellation(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{})
	defer close(f.block)
	c := newChecker(f, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if r := c.Check(ctx, "d.com"); r.Status != StatusUnknown {
		t.Fatalf("got %+v", r)
	}
}

func TestIsPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8":         true,
		"2001:4860::8888": true,
		"127.0.0.1":       false,
		"10.1.2.3":        false,
		"172.16.0.1":      false,
		"192.168.1.1":     false,
		"169.254.1.1":     false,
		"100.64.0.1":      false,
		"0.0.0.0":         false,
		"192.0.2.1":       false,
		"::1":             false,
		"fe80::1":         false,
		"fd12::1":         false,
		"2001:db8::1":     false,
		"::ffff:10.0.0.1": false,
		"224.0.0.1":       false,
	} {
		if got := isPublic(netip.MustParseAddr(addr)); got != want {
			t.Errorf("isPublic(%s) = %v", addr, got)
		}
	}
}

// After a lookup times out, the result is still written to the cache: the
// write must not reuse the expired lookup context.
func TestCacheWriteAfterTimeout(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{})
	defer close(f.block)
	c := &ctxCheckingCache{Memory: cache.NewMemory[Result](10)}
	cfg := DefaultConfig()
	cfg.Timeout = 30 * time.Millisecond
	NewChecker(f, c, cfg, nil, nil).Check(context.Background(), "slow.com")
	if c.writeErr != nil {
		t.Fatalf("cache written with a dead context: %v", c.writeErr)
	}
	if _, ok, _ := c.Get(context.Background(), "slow.com"); !ok {
		t.Fatal("unknown result not cached")
	}
}

type ctxCheckingCache struct {
	*cache.Memory[Result]
	writeErr error
}

func (c *ctxCheckingCache) Set(ctx context.Context, k string, v Result, ttl time.Duration) error {
	c.writeErr = ctx.Err()
	return c.Memory.Set(ctx, k, v, ttl)
}
