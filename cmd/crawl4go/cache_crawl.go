package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/storage"
)

// crawlSinglePageCached wraps crawlSinglePage with the persistent response
// cache (deps.Cache). It is a transparent layer: when caching is disabled
// (deps.Cache == nil) or the request opts out (req.NoCache), it falls straight
// through to crawlSinglePage.
//
// The cache key includes the URL and every option that changes the produced
// output (format, prune, extraction flags), so a cached entry is only served
// to an identical request shape. Only successful, non-blocked responses are
// stored. TTL/expiry is enforced by the FileCache.
func crawlSinglePageCached(ctx context.Context, deps *Deps, req CrawlRequest) CrawlResponse {
	if deps.Cache == nil || req.NoCache {
		return crawlSinglePage(ctx, deps.Cfg, deps.CDP, deps.HTTP, deps.Pruner, req)
	}

	key := cacheKey(req)

	if cached, err := deps.Cache.Get(key); err == nil && cached != nil {
		var resp CrawlResponse
		if jsonErr := json.Unmarshal([]byte(cached.Markdown), &resp); jsonErr == nil {
			resp.RenderSource = "cache"
			return resp
		}
		// Corrupt entry — drop it and fall through to a fresh crawl.
		_ = deps.Cache.Delete(key)
	}

	resp := crawlSinglePage(ctx, deps.Cfg, deps.CDP, deps.HTTP, deps.Pruner, req)

	// Only cache responses worth replaying.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && !resp.Blocked {
		if data, err := json.Marshal(resp); err == nil {
			_ = deps.Cache.Put(key, &storage.CachedResult{
				URL:        req.URL,
				Markdown:   string(data),
				StatusCode: resp.StatusCode,
				FetchedAt:  time.Now(),
			})
		}
	}

	return resp
}

// cacheKey derives a cache key from the URL plus all options that affect the
// produced output. Two requests that would render and process identically map
// to the same key.
func cacheKey(req CrawlRequest) string {
	return fmt.Sprintf("%s\x00out=%s\x00scroll=%t\x00prune=%t\x00meta=%t\x00tables=%t\x00media=%t\x00proxy=%t",
		req.URL, req.Output, req.Scroll, req.Prune, req.ExtractMeta, req.ExtractTables, req.ExtractMedia, req.Proxy)
}
