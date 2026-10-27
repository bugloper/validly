package ratelimit

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func withClock(l *Limiter) *time.Time {
	now := time.Unix(1_000_000, 0)
	l.now = func() time.Time { return now }
	return &now
}

func TestBurstThenRefill(t *testing.T) {
	l := New(60, time.Minute, 3, 0)
	now := withClock(l)

	for i := range 3 {
		if ok, _ := l.Allow("ip:1"); !ok {
			t.Fatalf("request %d denied within burst", i)
		}
	}
	ok, wait := l.Allow("ip:1")
	if ok || wait <= 0 || wait > time.Second {
		t.Fatalf("expected denial with ~1s wait, got %v %v", ok, wait)
	}
	// Another key is independent.
	if ok, _ := l.Allow("ip:2"); !ok {
		t.Fatal("independent key denied")
	}

	*now = now.Add(time.Second)
	if ok, _ := l.Allow("ip:1"); !ok {
		t.Fatal("token not refilled after 1s")
	}
	if ok, _ := l.Allow("ip:1"); ok {
		t.Fatal("only one token should have refilled")
	}

	// Refill never exceeds the burst.
	*now = now.Add(time.Hour)
	for range 3 {
		l.Allow("ip:1")
	}
	if ok, _ := l.Allow("ip:1"); ok {
		t.Fatal("bucket exceeded burst")
	}
}

func TestDisabled(t *testing.T) {
	l := New(0, time.Minute, 0, 0)
	for range 1000 {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatal("disabled limiter denied")
		}
	}
	var nilLimiter *Limiter
	if ok, _ := nilLimiter.Allow("k"); !ok {
		t.Fatal("nil limiter denied")
	}
}

func TestMemoryBound(t *testing.T) {
	l := New(10, time.Minute, 10, 100)
	now := withClock(l)
	for i := range 1000 {
		l.Allow(fmt.Sprint(i))
		*now = now.Add(time.Millisecond)
	}
	if n := l.Len(); n > 100 {
		t.Fatalf("tracking %d keys, bound is 100", n)
	}
}

func TestSweepEvictsOldestWhenNoneRefilled(t *testing.T) {
	l := New(1, time.Minute, 1, 2)
	now := withClock(l)
	l.Allow("abuser")
	*now = now.Add(time.Second)
	l.Allow("other")
	*now = now.Add(time.Second)
	// Neither bucket has refilled, so the oldest one is dropped.
	l.Allow("third")
	if l.Len() != 2 {
		t.Fatalf("len %d", l.Len())
	}
	if _, ok := l.buckets["abuser"]; ok {
		t.Fatal("oldest bucket not evicted")
	}
}

func TestConcurrentAllow(t *testing.T) {
	l := New(100, time.Hour, 100, 0)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			for range 50 {
				if ok, _ := l.Allow("shared"); ok {
					allowed.Add(1)
				}
			}
		})
	}
	wg.Wait()
	// 1000 attempts against a burst of 100 with negligible refill.
	if n := allowed.Load(); n < 100 || n > 101 {
		t.Fatalf("allowed %d, want 100", n)
	}
}

func TestFullLimiterStaysFast(t *testing.T) {
	l := New(1, time.Hour, 1, 100_000)
	withClock(l)
	for i := range 100_000 {
		l.Allow(fmt.Sprint("ip:", i))
	}
	// None of these buckets has refilled; new keys must not each pay for a
	// full scan.
	start := time.Now()
	for i := range 20_000 {
		l.Allow(fmt.Sprint("new:", i))
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("20k inserts into a full limiter took %v", d)
	}
	if l.Len() > 100_000 {
		t.Fatalf("len %d", l.Len())
	}
}
