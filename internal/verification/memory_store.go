package verification

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-process Store used in tests.
type MemoryStore struct {
	mu     sync.Mutex
	tokens map[Hash]*memToken
}

type memToken struct {
	Token
	used bool
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore { return &MemoryStore{tokens: map[Hash]*memToken{}} }

func (m *MemoryStore) CreateToken(_ context.Context, t Token, maxRecent int, since time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	recent := 0
	for _, x := range m.tokens {
		if x.EmailHash == t.EmailHash && !x.CreatedAt.Before(since) {
			recent++
		}
	}
	if recent >= maxRecent {
		return false, nil
	}
	m.tokens[t.Hash] = &memToken{Token: t}
	return true, nil
}

func (m *MemoryStore) ConsumeToken(_ context.Context, hash Hash, now time.Time) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tokens[hash]
	if !ok || t.used || !now.Before(t.ExpiresAt) {
		return "", ErrInvalidToken
	}
	t.used = true
	return t.Email, nil
}

func (m *MemoryStore) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for h, t := range m.tokens {
		if t.ExpiresAt.Before(before) {
			delete(m.tokens, h)
			n++
		}
	}
	return n, nil
}
