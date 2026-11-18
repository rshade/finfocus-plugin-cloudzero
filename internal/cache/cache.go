// Package cache holds query results in memory until their TTL passes.
package cache

import (
	"sync"
	"time"
)

// Entry is one cached value and the time it stops being served.
type Entry struct {
	Data      any
	ExpiresAt time.Time
}

// Cache is a thread-safe map of entries with per-entry expiry. Expired entries
// are not served.
type Cache struct {
	mu    sync.RWMutex
	store map[string]*Entry
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{
		mu:    sync.RWMutex{},
		store: make(map[string]*Entry),
	}
}

// Get returns the value stored under key, or false when it is missing or expired.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.store[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Data, true
}

// Set stores data under key until ttl has passed.
func (c *Cache) Set(key string, data any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store[key] = &Entry{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
	}
}
