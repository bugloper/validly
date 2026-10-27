package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestMemoryExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1000, 0)
	m := NewMemory[string](10)
	m.now = func() time.Time { return now }

	_ = m.Set(ctx, "k", "v", time.Minute)
	if v, ok, _ := m.Get(ctx, "k"); !ok || v != "v" {
		t.Fatalf("got %q %v", v, ok)
	}
	now = now.Add(time.Minute)
	if _, ok, _ := m.Get(ctx, "k"); ok {
		t.Fatal("entry should have expired")
	}
	_ = m.Set(ctx, "zero", "v", 0)
	if _, ok, _ := m.Get(ctx, "zero"); ok {
		t.Fatal("zero TTL must not be stored")
	}
}

func TestMemoryBounded(t *testing.T) {
	ctx := context.Background()
	m := NewMemory[int](100)
	for i := range 1000 {
		_ = m.Set(ctx, fmt.Sprint(i), i, time.Hour)
	}
	if n := m.Len(); n > 100 {
		t.Fatalf("len %d exceeds bound", n)
	}
	// The most recent insert always survives.
	if v, ok, _ := m.Get(ctx, "999"); !ok || v != 999 {
		t.Fatal("latest entry evicted")
	}
}

func TestMemoryConcurrent(t *testing.T) {
	ctx := context.Background()
	m := NewMemory[int](50)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 500 {
				k := fmt.Sprint((g*500 + i) % 80)
				_ = m.Set(ctx, k, i, time.Minute)
				_, _, _ = m.Get(ctx, k)
			}
		})
	}
	wg.Wait()
}

func TestRedis(t *testing.T) {
	ctx := context.Background()
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	type value struct{ N int }
	c := NewRedis[value](client, "test:")
	if _, ok, err := c.Get(ctx, "missing"); ok || err != nil {
		t.Fatalf("miss: %v %v", ok, err)
	}
	if err := c.Set(ctx, "k", value{N: 7}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := c.Get(ctx, "k"); !ok || err != nil || v.N != 7 {
		t.Fatalf("got %+v %v %v", v, ok, err)
	}
	if !srv.Exists("test:k") {
		t.Fatal("key not namespaced")
	}
	srv.FastForward(2 * time.Minute)
	if _, ok, _ := c.Get(ctx, "k"); ok {
		t.Fatal("expected expiry")
	}

	srv.Close()
	if _, _, err := c.Get(ctx, "k"); err == nil {
		t.Fatal("expected error when Redis is down")
	}
}

func TestRedisCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	c := NewRedis[int](client, "")
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }

	srv.Close()
	if _, _, err := c.Get(ctx, "k"); err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("first failure should be the real error, got %v", err)
	}
	// While the breaker is open, calls fail fast without touching Redis.
	start := time.Now()
	for range 100 {
		if _, _, err := c.Get(ctx, "k"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("got %v", err)
		}
		if err := c.Set(ctx, "k", 1, time.Minute); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("got %v", err)
		}
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("open breaker was slow: %v", d)
	}

	// After the cooldown Redis is tried again.
	if err := srv.Restart(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(redisCooldown)
	if err := c.Set(ctx, "k", 7, time.Minute); err != nil {
		t.Fatalf("after cooldown: %v", err)
	}
	if v, ok, err := c.Get(ctx, "k"); !ok || err != nil || v != 7 {
		t.Fatalf("got %v %v %v", v, ok, err)
	}
}

func TestRedisBreakerIgnoresCallerCancellation(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	c := NewRedis[int](client, "")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := c.Get(cancelled, "k"); err == nil {
		t.Fatal("expected context error")
	}
	if err := c.Set(cancelled, "k", 1, time.Minute); err == nil {
		t.Fatal("expected context error")
	}
	// Redis itself is healthy, so the breaker must still be closed.
	if err := c.Set(context.Background(), "k", 1, time.Minute); err != nil {
		t.Fatalf("breaker opened by caller cancellation: %v", err)
	}
}
