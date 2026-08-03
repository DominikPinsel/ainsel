package invocations

import (
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

// Store is an in-memory, thread-safe ring buffer of recent invocations.
//
// When the buffer reaches capacity, recording a new invocation evicts the
// oldest one. Lookups by ID remain O(1) via a map index.
type Store struct {
	mu       sync.RWMutex
	capacity int
	// order holds invocation IDs in insertion order. The oldest is at the
	// front; the newest is at the back. We use a slice rather than a ring
	// to keep the implementation straightforward — the buffer is small
	// enough (default 1000) that O(n) eviction is negligible.
	order []string
	byID  map[string]*Invocation
}

// NewStore returns a new Store with the given capacity. If capacity <= 0,
// DefaultCapacity is used.
func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Store{
		capacity: capacity,
		order:    make([]string, 0, capacity),
		byID:     make(map[string]*Invocation, capacity),
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
func (s *Store) Record(inv Invocation) Invocation {
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
func (s *Store) Complete(id, status, errMsg string, endTime time.Time) bool {
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
func (s *Store) Get(id string) (Invocation, bool) {
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
func (s *Store) List(opts ListOptions) []Invocation {
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

	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out
}

// Len returns the number of invocations currently stored.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.order)
}

// Capacity returns the maximum number of invocations the store will retain.
func (s *Store) Capacity() int {
	return s.capacity
}
