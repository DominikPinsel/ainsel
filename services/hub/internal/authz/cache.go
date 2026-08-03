package authz

import (
	"context"
	"sync"
	"time"
)

// GroupCacheLoader fetches group roles for a user from the database.
type GroupCacheLoader func(userID string) (map[string]GroupRole, error)

// GroupCache caches per-user group membership roles with a TTL.
type GroupCache struct {
	loader GroupCacheLoader
	ttl    time.Duration
	mu     sync.RWMutex
	items  map[string]cacheEntry
}

type cacheEntry struct {
	roles   map[string]GroupRole
	expires time.Time
}

// NewGroupCache creates a cache with the given loader and TTL.
func NewGroupCache(loader GroupCacheLoader, ttl time.Duration) *GroupCache {
	return &GroupCache{
		loader: loader,
		ttl:    ttl,
		items:  make(map[string]cacheEntry),
	}
}

// UserRoles returns the full role map for the user, using the cache if fresh.
func (c *GroupCache) UserRoles(userID string) (map[string]GroupRole, error) {
	c.mu.RLock()
	if e, ok := c.items[userID]; ok && time.Now().Before(e.expires) {
		c.mu.RUnlock()
		return e.roles, nil
	}
	c.mu.RUnlock()

	roles, err := c.loader(userID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.items[userID] = cacheEntry{roles: roles, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return roles, nil
}

// UserRole returns the user's role in a specific group.
// Returns empty string if the user is not a member.
func (c *GroupCache) UserRole(_ context.Context, userID, groupID string) (GroupRole, error) {
	roles, err := c.UserRoles(userID)
	if err != nil {
		return "", err
	}
	return roles[groupID], nil
}

// GroupIDs returns all group IDs the user belongs to (convenience method).
func (c *GroupCache) GroupIDs(userID string) ([]string, error) {
	roles, err := c.UserRoles(userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(roles))
	for gid := range roles {
		ids = append(ids, gid)
	}
	return ids, nil
}

// Invalidate removes a user's cached entry.
func (c *GroupCache) Invalidate(userID string) {
	c.mu.Lock()
	delete(c.items, userID)
	c.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (c *GroupCache) InvalidateAll() {
	c.mu.Lock()
	c.items = make(map[string]cacheEntry)
	c.mu.Unlock()
}
