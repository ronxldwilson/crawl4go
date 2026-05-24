package crawl

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

// StreamFunc is called for each result as it arrives during a streaming crawl.
// Return false to stop the crawl early.
type StreamFunc func(result DeepCrawlResult) bool

// StreamBFS performs a breadth-first crawl, calling streamFn for each result
// as it completes rather than batching. This is useful for large crawls where
// you want to process or persist results incrementally.
func StreamBFS(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions, streamFn StreamFunc) (CrawlStats, error) {
	start := time.Now()

	visited := make(map[string]bool)
	depths := make(map[string]int)

	baseU, _ := url.Parse(startURL)
	normalizedStart := content.NormalizeURL(startURL, baseU)

	type queueItem struct {
		url       string
		parentURL string
	}

	var currentLevel []queueItem

	if opts.InitialState != nil {
		for u, v := range opts.InitialState.Visited {
			visited[u] = v
		}
		for u, d := range opts.InitialState.Depths {
			depths[u] = d
		}
		for _, u := range opts.InitialState.Pending {
			currentLevel = append(currentLevel, queueItem{url: u, parentURL: ""})
		}
	}

	if len(currentLevel) == 0 {
		visited[normalizedStart] = true
		depths[normalizedStart] = 0
		currentLevel = []queueItem{{url: startURL, parentURL: ""}}
	}

	var crawled, blocked atomic.Int64
	var maxDepthReached int
	var stopped atomic.Bool

	for depth := 0; depth <= opts.MaxDepth && len(currentLevel) > 0; depth++ {
		select {
		case <-ctx.Done():
			return CrawlStats{
				PagesCrawled:    int(crawled.Load()),
				PagesBlocked:   int(blocked.Load()),
				MaxDepthReached: maxDepthReached,
				TotalTimeMs:     time.Since(start).Milliseconds(),
			}, ctx.Err()
		default:
		}

		if stopped.Load() {
			break
		}

		type indexedResult struct {
			result *DeepCrawlResult
			item   queueItem
		}

		results := make([]indexedResult, 0, len(currentLevel))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, item := range currentLevel {
			if opts.MaxPages > 0 && int(crawled.Load()) >= opts.MaxPages {
				break
			}
			if stopped.Load() {
				break
			}

			wg.Add(1)
			crawled.Add(1)

			go func(it queueItem) {
				defer wg.Done()

				result, err := crawlFn(ctx, it.url)
				if err != nil || result == nil {
					return
				}

				result.Depth = depth
				result.ParentURL = it.parentURL

				if result.Blocked {
					blocked.Add(1)
				}

				// Stream the result immediately
				if !streamFn(*result) {
					stopped.Store(true)
					return
				}

				mu.Lock()
				results = append(results, indexedResult{result: result, item: it})
				mu.Unlock()
			}(item)
		}

		wg.Wait()

		if depth > maxDepthReached {
			maxDepthReached = depth
		}

		if stopped.Load() {
			break
		}

		// Discover links for the next level
		var nextLevel []queueItem
		for _, ir := range results {
			if ir.result.URL == "" {
				continue
			}
			if depth >= opts.MaxDepth {
				continue
			}
			newLinks := discoverLinks(ctx, *ir.result, visited, depths, depth+1, opts)
			for _, nl := range newLinks {
				nextLevel = append(nextLevel, queueItem{url: nl, parentURL: ir.result.URL})
			}
		}

		if int(crawled.Load()) >= opts.MaxPages {
			break
		}
		currentLevel = nextLevel
	}

	return CrawlStats{
		PagesCrawled:    int(crawled.Load()),
		PagesBlocked:   int(blocked.Load()),
		MaxDepthReached: maxDepthReached,
		TotalTimeMs:     time.Since(start).Milliseconds(),
	}, nil
}
