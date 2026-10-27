// Package cache provides a small TTL cache abstraction with an in-memory
// implementation and an optional Redis implementation for sharing results
// between replicas.
//
// Caches are an optimisation only. Callers must treat any error as a miss and
// carry on, so a failing cache never breaks validation.
package cache

import (
	"context"
	"sync"
	"time"
)

// Cache stores values of type V under string keys with a per-entry TTL.
type Cache[V any] interface {
	Get(ctx context.Context, key string) (V, bool, error)
	Set(ctx context.Context, key string, value V, ttl time.Duration) error
}

// Memory is a bounded in-process Cache. When full, expired entries are swept
// first and then arbitrary entries are evicted.
type Memory[V any] struct {
	mu         sync.Mutex
	entries    map[string]memoryEntry[V]
	maxEntries int
	now        func() time.Time
}

type memoryEntry[V any] struct {
	value   V
	expires time.Time
}

// NewMemory returns a Memory cache holding at most maxEntries values.
func NewMemory[V any](maxEntries int) *Memory[V] {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &Memory[V]{
		entries:    make(map[string]memoryEntry[V]),
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

func (m *Memory[V]) Get(_ context.Context, key string) (V, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	if !ok {
		var zero V
		return zero, false, nil
	}
	if !m.now().Before(e.expires) {
		delete(m.entries, key)
		var zero V
		return zero, false, nil
	}
	return e.value, true, nil
}

func (m *Memory[V]) Set(_ context.Context, key string, value V, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.entries[key]; !exists && len(m.entries) >= m.maxEntries {
		m.evictLocked()
	}
	m.entries[key] = memoryEntry[V]{value: value, expires: m.now().Add(ttl)}
	return nil
}

// Len reports the number of stored entries, including expired ones not yet swept.
func (m *Memory[V]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func (m *Memory[V]) evictLocked() {
	now := m.now()
	for k, e := range m.entries {
		if !now.Before(e.expires) {
			delete(m.entries, k)
		}
	}
	// Evict a tenth of the capacity at random (map order) so a full cache does
	// not pay the sweep cost on every insert.
	target := m.maxEntries - max(1, m.maxEntries/10)
	for k := range m.entries {
		if len(m.entries) <= target {
			break
		}
		delete(m.entries, k)
	}
}
