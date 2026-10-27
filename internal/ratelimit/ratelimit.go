// Package ratelimit implements an in-memory token-bucket limiter keyed by an
// arbitrary string (client IP, API key name, hashed email, ...).
//
// Limits are per process. Behind a load balancer with N replicas the
// effective limit is up to N times the configured one.
package ratelimit

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Limiter allows Rate events per Period per key, with bursts of up to Burst.
// It is safe for concurrent use and bounded in memory: once MaxKeys keys are
// tracked, buckets that have refilled completely are discarded (they behave
// identically to a new bucket), and if that is not enough the oldest
// buckets are dropped.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	perSec  float64
	burst   float64
	maxKeys int
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a Limiter allowing rate events per period with the given burst.
// A non-positive rate disables limiting.
func New(rate int, period time.Duration, burst int, maxKeys int) *Limiter {
	if burst < 1 {
		burst = max(1, rate)
	}
	if maxKeys < 1 {
		maxKeys = 100_000
	}
	perSec := 0.0
	if rate > 0 && period > 0 {
		perSec = float64(rate) / period.Seconds()
	}
	return &Limiter{
		buckets: make(map[string]*bucket),
		perSec:  perSec,
		burst:   float64(burst),
		maxKeys: maxKeys,
		now:     time.Now,
	}
}

// Allow consumes one token for key. When the bucket is empty it returns false
// and how long until a token becomes available.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil || l.perSec == 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.sweepLocked(now)
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	} else {
		b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.perSec)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.perSec * float64(time.Second))
	return false, wait
}

// Len reports the number of tracked keys.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// sweepLocked makes room for new keys. Buckets that have refilled are
// dropped (they behave exactly like new ones). If that is not enough, the
// least recently used tenth is evicted at once, so a full limiter pays for
// a sweep only once per maxKeys/10 new keys rather than on every request.
func (l *Limiter) sweepLocked(now time.Time) {
	fullAfter := time.Duration(l.burst / l.perSec * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= fullAfter {
			delete(l.buckets, k)
		}
	}
	if len(l.buckets) < l.maxKeys {
		return
	}
	type entry struct {
		key  string
		last time.Time
	}
	entries := make([]entry, 0, len(l.buckets))
	for k, b := range l.buckets {
		entries = append(entries, entry{k, b.last})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].last.Before(entries[j].last) })
	for _, e := range entries[:max(1, l.maxKeys/10)] {
		delete(l.buckets, e.key)
	}
}
