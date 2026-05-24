package crawl

import (
	"sync"
	"time"
)

// SitemapCacheEntry holds parsed sitemap URLs and their lastmod timestamps,
// along with the time the entry was cached.
type SitemapCacheEntry struct {
	URLs     []SeedURL `json:"urls"`
	CachedAt time.Time `json:"cached_at"`
}

// SitemapCache is a concurrency-safe in-memory cache for parsed sitemap data,
// keyed by sitemap URL. Each entry stores the discovered seed URLs together
// with the time of caching so staleness can be evaluated.
type SitemapCache struct {
	mu      sync.RWMutex
	entries map[string]*SitemapCacheEntry
}

// NewSitemapCache creates an empty SitemapCache.
func NewSitemapCache() *SitemapCache {
	return &SitemapCache{
		entries: make(map[string]*SitemapCacheEntry),
	}
}

// Get returns the cached entry for sitemapURL if it exists and is not older
// than maxAge. Returns nil when no entry exists or when the entry is stale.
func (sc *SitemapCache) Get(sitemapURL string, maxAge time.Duration) *SitemapCacheEntry {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	entry, ok := sc.entries[sitemapURL]
	if !ok {
		return nil
	}
	if maxAge > 0 && time.Since(entry.CachedAt) > maxAge {
		return nil
	}
	return entry
}

// Put stores (or replaces) the cached sitemap entry for sitemapURL.
func (sc *SitemapCache) Put(sitemapURL string, urls []SeedURL) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.entries[sitemapURL] = &SitemapCacheEntry{
		URLs:     urls,
		CachedAt: time.Now(),
	}
}

// IsStale reports whether the cached entry for sitemapURL is older than maxAge.
// Returns true when no entry exists for the URL (missing is treated as stale).
func (sc *SitemapCache) IsStale(sitemapURL string, maxAge time.Duration) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	entry, ok := sc.entries[sitemapURL]
	if !ok {
		return true
	}
	return time.Since(entry.CachedAt) > maxAge
}

// Clear removes all cached entries.
func (sc *SitemapCache) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.entries = make(map[string]*SitemapCacheEntry)
}
