package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrUnavailable is returned while the circuit breaker is open.
var ErrUnavailable = errors.New("cache temporarily unavailable")

const (
	// redisTimeout bounds each cache operation so a slow Redis cannot slow
	// down validation.
	redisTimeout = 250 * time.Millisecond
	// redisCooldown is how long Redis is skipped after a failure.
	redisCooldown = 5 * time.Second
)

// Redis is a Cache backed by Redis. Values are stored as JSON under
// prefix+key, so V must round-trip through encoding/json.
//
// After any Redis error the cache stops calling Redis for a short cooldown
// and reports ErrUnavailable instead, so an outage costs at most one
// timeout per cooldown rather than one per request.
type Redis[V any] struct {
	client    redis.UniversalClient
	prefix    string
	downUntil atomic.Int64 // unix nanos
	now       func() time.Time
}

// NewRedis returns a Redis cache that namespaces keys with prefix.
func NewRedis[V any](client redis.UniversalClient, prefix string) *Redis[V] {
	return &Redis[V]{client: client, prefix: prefix, now: time.Now}
}

func (r *Redis[V]) Get(ctx context.Context, key string) (V, bool, error) {
	var v V
	if r.open() {
		return v, false, ErrUnavailable
	}
	opCtx, cancel := context.WithTimeout(ctx, redisTimeout)
	defer cancel()
	b, err := r.client.Get(opCtx, r.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return v, false, nil
	}
	if err != nil {
		r.failed(ctx)
		return v, false, err
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, false, err
	}
	return v, true, nil
}

func (r *Redis[V]) Set(ctx context.Context, key string, value V, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	if r.open() {
		return ErrUnavailable
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	opCtx, cancel := context.WithTimeout(ctx, redisTimeout)
	defer cancel()
	if err := r.client.Set(opCtx, r.prefix+key, b, ttl).Err(); err != nil {
		r.failed(ctx)
		return err
	}
	return nil
}

func (r *Redis[V]) open() bool { return r.now().UnixNano() < r.downUntil.Load() }

// failed opens the breaker, unless the operation failed only because the
// caller's own context ended (a disconnected client says nothing about Redis).
func (r *Redis[V]) failed(caller context.Context) {
	if caller.Err() != nil {
		return
	}
	r.downUntil.Store(r.now().Add(redisCooldown).UnixNano())
}
