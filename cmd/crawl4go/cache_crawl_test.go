package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/storage"
)

func TestCacheKeyDistinguishesOptions(t *testing.T) {
	base := CrawlRequest{URL: "https://example.com", Output: "markdown"}

	same := cacheKey(base)
	if cacheKey(base) != same {
		t.Error("cacheKey not stable for identical requests")
	}

	variants := []CrawlRequest{
		{URL: "https://example.com", Output: "text"},
		{URL: "https://example.com", Output: "markdown", Prune: true},
		{URL: "https://example.com", Output: "markdown", ExtractMeta: true},
		{URL: "https://example.com", Output: "markdown", ExtractTables: true},
		{URL: "https://other.com", Output: "markdown"},
	}
	for i, v := range variants {
		if cacheKey(v) == same {
			t.Errorf("variant %d should produce a distinct cache key", i)
		}
	}
}

func TestCrawlSinglePageCachedServesFromCache(t *testing.T) {
	fc, err := storage.NewFileCache(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	deps := &Deps{Cache: fc}

	req := CrawlRequest{URL: "https://example.com", Output: "markdown"}

	// Pre-populate the cache with a known response under the request's key.
	want := CrawlResponse{
		URL:        req.URL,
		StatusCode: 200,
		Content:    "# Cached content",
	}
	data, _ := json.Marshal(want)
	if err := fc.Put(cacheKey(req), &storage.CachedResult{
		URL:        req.URL,
		Markdown:   string(data),
		StatusCode: 200,
		FetchedAt:  time.Now(),
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	// On a hit, crawlSinglePageCached returns before ever calling the renderer,
	// so nil CDP/HTTP deps are safe here.
	got := crawlSinglePageCached(context.Background(), deps, req)

	if got.Content != want.Content {
		t.Errorf("content = %q, want %q", got.Content, want.Content)
	}
	if got.RenderSource != "cache" {
		t.Errorf("RenderSource = %q, want %q", got.RenderSource, "cache")
	}
}
