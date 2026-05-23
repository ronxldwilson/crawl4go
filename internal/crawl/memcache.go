package crawl

import (
	"sync"
	"time"
)

// CachedResponse holds a cached HTTP response with its metadata.
type CachedResponse struct {
	HTML       string    `json:"html"`
	StatusCode int       `json:"status_code"`
	CachedAt   time.Time `json:"cached_at"`
}

// CacheStats reports hit/miss counters for a MemCache.
type CacheStats struct {
	Hits       int64 `json:"hits"`
	Misses     int64 `json:"misses"`
	Entries    int   `json:"entries"`
	MaxEntries int   `json:"max_entries"`
}

// MemCache is a thread-safe in-memory response cache with TTL and max-entry eviction.
type MemCache struct {
	mu         sync.RWMutex
	entries    map[string]CachedResponse
	maxEntries int
	ttl        time.Duration
	hits       int64
	misses     int64
}

// NewMemCache creates a new in-memory cache. Entries older than ttl are considered
// expired and will be evicted on access or via Evict(). When the cache reaches
// maxEntries, the oldest entry is evicted to make room.
func NewMemCache(maxEntries int, ttl time.Duration) *MemCache {
	return &MemCache{
		entries:    make(map[string]CachedResponse),
		maxEntries: maxEntries,
		ttl:        ttl,
	}
}

// Get retrieves a cached response for the given URL. It returns false if the URL
// is not cached or if the entry has expired (expired entries are removed on access).
func (c *MemCache) Get(url string) (CachedResponse, bool) {
	c.mu.RLock()
	entry, ok := c.entries[url]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return CachedResponse{}, false
	}

	if time.Since(entry.CachedAt) > c.ttl {
		c.mu.Lock()
		// Re-check under write lock in case another goroutine already removed it.
		if e, still := c.entries[url]; still && time.Since(e.CachedAt) > c.ttl {
			delete(c.entries, url)
		}
		c.misses++
		c.mu.Unlock()
		return CachedResponse{}, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return entry, true
}

// Set stores a response in the cache. If the cache is at capacity, the oldest
// entry is evicted first.
func (c *MemCache) Set(url string, resp CachedResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If the URL is already cached, just overwrite.
	if _, exists := c.entries[url]; exists {
		c.entries[url] = resp
		return
	}

	// Evict the oldest entry if at capacity.
	if len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}

	c.entries[url] = resp
}

// Evict removes all expired entries from the cache.
func (c *MemCache) Evict() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for url, entry := range c.entries {
		if now.Sub(entry.CachedAt) > c.ttl {
			delete(c.entries, url)
		}
	}
}

// Stats returns current cache statistics.
func (c *MemCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Hits:       c.hits,
		Misses:     c.misses,
		Entries:    len(c.entries),
		MaxEntries: c.maxEntries,
	}
}

// evictOldest removes the entry with the earliest CachedAt timestamp.
// Must be called with c.mu held for writing.
func (c *MemCache) evictOldest() {
	var oldestURL string
	var oldestTime time.Time

	for url, entry := range c.entries {
		if oldestURL == "" || entry.CachedAt.Before(oldestTime) {
			oldestURL = url
			oldestTime = entry.CachedAt
		}
	}

	if oldestURL != "" {
		delete(c.entries, oldestURL)
	}
}
