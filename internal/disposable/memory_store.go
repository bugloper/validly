package disposable

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-process AdminStore. Entries are lost on restart and
// are not shared between replicas; use the PostgreSQL store for that.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[ListName]map[string]time.Time
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: map[ListName]map[string]time.Time{}}
}

func (m *MemoryStore) ListEntries(context.Context) ([]Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Entry
	for list, domains := range m.entries {
		for d, at := range domains {
			out = append(out, Entry{List: list, Domain: d, CreatedAt: at})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].List != out[j].List {
			return out[i].List < out[j].List
		}
		return out[i].Domain < out[j].Domain
	})
	return out, nil
}

func (m *MemoryStore) AddEntry(_ context.Context, list ListName, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[list] == nil {
		m.entries[list] = map[string]time.Time{}
	}
	if _, ok := m.entries[list][domain]; !ok {
		m.entries[list][domain] = time.Now().UTC()
	}
	return nil
}

func (m *MemoryStore) RemoveEntry(_ context.Context, list ListName, domain string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.entries[list][domain]
	delete(m.entries[list], domain)
	return ok, nil
}
