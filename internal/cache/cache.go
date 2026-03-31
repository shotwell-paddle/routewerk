// Package cache provides a lightweight in-process TTL cache backed by a
// sync.Map. No external dependencies. Designed for small, hot datasets
// like location settings and grading configs that rarely change but are
// read on nearly every page load.
//
// Cache invalidation strategy: entries expire after a configurable TTL.
// On write (UpdateLocationSettings, etc.), callers should call Invalidate()
// to immediately evict the stale entry. Worst case (caller forgets), the
// TTL ensures data becomes fresh within a few minutes.
package cache

import (
	"sync"
	"time"
)

// Entry wraps a cached value with an expiration time.
type Entry[T any] struct {
	Value     T
	ExpiresAt time.Time
}

// Cache is a generic, concurrency-safe in-process cache with TTL expiry.
type Cache[T any] struct {
	data sync.Map
	ttl  time.Duration
}

// New creates a cache with the given TTL.
func New[T any](ttl time.Duration) *Cache[T] {
	return &Cache[T]{ttl: ttl}
}

// Get retrieves a value by key. Returns the value and true if found and
// not expired, or the zero value and false otherwise.
func (c *Cache[T]) Get(key string) (T, bool) {
	raw, ok := c.data.Load(key)
	if !ok {
		var zero T
		return zero, false
	}
	entry := raw.(*Entry[T])
	if time.Now().After(entry.ExpiresAt) {
		c.data.Delete(key)
		var zero T
		return zero, false
	}
	return entry.Value, true
}

// Set stores a value with the default TTL.
func (c *Cache[T]) Set(key string, value T) {
	c.data.Store(key, &Entry[T]{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	})
}

// Invalidate removes a specific key from the cache.
func (c *Cache[T]) Invalidate(key string) {
	c.data.Delete(key)
}

// InvalidateAll removes all entries from the cache.
func (c *Cache[T]) InvalidateAll() {
	c.data.Range(func(key, _ interface{}) bool {
		c.data.Delete(key)
		return true
	})
}

// Len returns the number of entries in the cache (including expired ones
// that haven't been evicted yet). Primarily useful for testing.
func (c *Cache[T]) Len() int {
	count := 0
	c.data.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
