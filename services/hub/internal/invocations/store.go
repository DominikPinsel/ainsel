package invocations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// DefaultCapacity is the default in-memory ring buffer size used when no
// explicit capacity is provided.
const DefaultCapacity = 1000

// Retention is how long invocation records are kept before pruning. It is
// deliberately aligned with tasklogs.ConversationRetention so an invocation
// and its conversation transcript expire together: the UI reaches a
// transcript only through its invocation record, so a conversation that
// outlives its invocation would be unreachable.
const Retention = 48 * time.Hour

// Store is the persistence contract for invocation history. The hub always
// wires the Postgres-backed implementation (NewPgStore) in production;
// NewMemoryStore exists for unit tests and as a fallback when no database
// is configured.
type Store interface {
	// Record creates a new invocation record in StatusRunning and stores it.
	// The returned invocation is a copy with ID and StartTime populated.
	Record(inv Invocation) Invocation
	// Complete updates an existing invocation with its terminal status and
	// returns false when the invocation is unknown. errMsg is recorded when
	// status is failure/timeout.
	Complete(id, status, errMsg string, endTime time.Time) bool
	// Get returns the invocation with the given ID, or false if absent.
	Get(id string) (Invocation, bool)
	// List returns invocations sorted newest-first, applying the given filters.
	List(opts ListOptions) []Invocation
	// ListWithTotal behaves like List but also returns the number of
	// invocations matching the filters before opts.Limit is applied.
	ListWithTotal(opts ListOptions) ([]Invocation, int)
	// Len returns the number of invocations currently stored.
	Len() int
	// Capacity reports the configured retention bound for API compatibility.
	// The in-memory store evicts by count; the Postgres store prunes by age.
	Capacity() int
	// Prune removes records older than the given retention. Implementations
	// that evict by capacity (memory) have nothing to prune and return 0.
	Prune(ctx context.Context, retention time.Duration) (int64, error)
}

// MemoryStore is an in-memory, thread-safe ring buffer of recent
// invocations.
//
// When the buffer reaches capacity, recording a new invocation evicts the
// oldest one. Lookups by ID remain O(1) via a map index. Records do not
// survive hub restarts — use NewPgStore for durable history.
type MemoryStore struct {
	mu       sync.RWMutex
	capacity int
	// order holds invocation IDs in insertion order. The oldest is at the
	// front; the newest is at the back. We use a slice rather than a ring
	// to keep the implementation straightforward — the buffer is small
	// enough (default 1000) that O(n) eviction is negligible.
	order []string
	byID  map[string]*Invocation
}

// NewMemoryStore returns a new MemoryStore with the given capacity. If
// capacity <= 0, DefaultCapacity is used.
func NewMemoryStore(capacity int) *MemoryStore {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &MemoryStore{
		capacity: capacity,
		order:     make([]string, 0, capacity),
		byID:      make(map[string]*Invocation, capacity),
	}
}

// generateID returns a new opaque invocation ID.
func generateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("inv-%s", hex.EncodeToString(b))
}

// Record creates a new invocation record in StatusRunning and stores it.
// The returned invocation is a copy; the canonical record lives in the store.
func (s *MemoryStore) Record(inv Invocation) Invocation {
	if inv.ID == "" {
		inv.ID = generateID()
	}
	if inv.StartTime.IsZero() {
		inv.StartTime = time.Now().UTC()
	}
	if inv.Status == "" {
		inv.Status = StatusRunning
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Evict oldest if at capacity. Avoid evicting the same ID we're inserting.
	for len(s.order) >= s.capacity {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.byID, oldest)
	}

	stored := inv
	s.byID[inv.ID] = &stored
	s.order = append(s.order, inv.ID)

	// Return a copy so callers can't mutate the stored record.
	return stored
}

// Complete updates an existing invocation with its terminal status.
//
// If the invocation is not found (e.g. evicted from the ring buffer),
// Complete returns false. errMsg is recorded when status is failure/timeout.
func (s *MemoryStore) Complete(id, status, errMsg string, endTime time.Time) bool {
	if endTime.IsZero() {
		endTime = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[id]
	if !ok {
		return false
	}
	rec.Status = status
	rec.EndTime = &endTime
	d := endTime.Sub(rec.StartTime).Milliseconds()
	rec.DurationMs = &d
	rec.Error = errMsg
	return true
}

// Get returns a copy of the invocation with the given ID, or false if absent.
func (s *MemoryStore) Get(id string) (Invocation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.byID[id]
	if !ok {
		return Invocation{}, false
	}
	return *rec, true
}

// ListOptions filters and paginates List results. The zero value lists all
// invocations newest-first with no filters and no limit.
type ListOptions struct {
	// AgentName, if non-empty, restricts results to invocations for this agent.
	AgentName string
	// Status, if non-empty, restricts results to invocations with this status.
	Status string
	// TriggerName, if non-empty, restricts results to invocations dispatched by this trigger.
	TriggerName string
	// EventID, if non-empty, restricts results to invocations for this event.
	EventID string
	// Since, if non-zero, restricts results to invocations started at or after this time.
	Since time.Time
	// Until, if non-zero, restricts results to invocations started before this time.
	Until time.Time
	// Limit, if > 0, caps the number of results returned.
	Limit int
}

// List returns invocations sorted newest-first, applying the given filters.
//
// The returned slice contains copies of the stored records; callers may
// safely mutate them without affecting the store.
func (s *MemoryStore) List(opts ListOptions) []Invocation {
	items, _ := s.ListWithTotal(opts)
	return items
}

// ListWithTotal behaves like List but additionally returns the number of
// invocations matching the filters before opts.Limit is applied. This lets
// callers report a truthful total even when the result set is capped.
func (s *MemoryStore) ListWithTotal(opts ListOptions) ([]Invocation, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Invocation, 0, len(s.order))
	for _, id := range s.order {
		rec := s.byID[id]
		if rec == nil {
			continue
		}
		if opts.AgentName != "" && rec.AgentName != opts.AgentName {
			continue
		}
		if opts.Status != "" && rec.Status != opts.Status {
			continue
		}
		if opts.TriggerName != "" && rec.TriggerName != opts.TriggerName {
			continue
		}
		if opts.EventID != "" && rec.EventID != opts.EventID {
			continue
		}
		if !opts.Since.IsZero() && rec.StartTime.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && !rec.StartTime.Before(opts.Until) {
			continue
		}
		out = append(out, *rec)
	}

	// Sort newest first.
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartTime.After(out[j].StartTime)
	})

	total := len(out)
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, total
}

// Len returns the number of invocations currently stored.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.order)
}

// Capacity returns the maximum number of invocations the store will retain.
func (s *MemoryStore) Capacity() int {
	return s.capacity
}

// Prune is a no-op: the ring buffer bounds itself by capacity, so there is
// nothing to age out. Satisfies the Store interface.
func (s *MemoryStore) Prune(ctx context.Context, retention time.Duration) (int64, error) {
	return 0, nil
}
