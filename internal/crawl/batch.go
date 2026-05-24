package crawl

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// BatchConfig controls concurrent batch crawling.
type BatchConfig struct {
	MaxConcurrent int           // max parallel crawls (default 10)
	Timeout       time.Duration // per-URL timeout (default 30s)
	RetryConfig   RetryConfig   // retry settings per URL
}

// DefaultBatchConfig returns sensible defaults.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		MaxConcurrent: 10,
		Timeout:       30 * time.Second,
		RetryConfig:   DefaultRetryConfig(),
	}
}

// BatchResult holds the outcome of a batch crawl.
type BatchResult struct {
	Results []DeepCrawlResult
	Errors  map[string]error // keyed by URL
	Stats   CrawlStats
}

// CrawlMany concurrently crawls multiple URLs using a semaphore to limit
// parallelism. Results are collected as they complete.
func CrawlMany(ctx context.Context, urls []string, crawlFn CrawlFunc, cfg BatchConfig) *BatchResult {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 10
	}

	start := time.Now()
	sem := make(chan struct{}, cfg.MaxConcurrent)

	var mu sync.Mutex
	br := &BatchResult{
		Errors: make(map[string]error),
	}
	var crawled, blocked atomic.Int64

	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				br.Errors[url] = ctx.Err()
				mu.Unlock()
				return
			}

			// Per-URL timeout
			urlCtx := ctx
			if cfg.Timeout > 0 {
				var cancel context.CancelFunc
				urlCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
				defer cancel()
			}

			rr := CrawlWithRetry(urlCtx, url, crawlFn, cfg.RetryConfig, nil)

			if rr.Result != nil {
				crawled.Add(1)
				if rr.Result.Blocked {
					blocked.Add(1)
				}
				mu.Lock()
				br.Results = append(br.Results, *rr.Result)
				mu.Unlock()
			} else if rr.LastErr != nil {
				mu.Lock()
				br.Errors[url] = rr.LastErr
				mu.Unlock()
			}
		}(u)
	}

	wg.Wait()

	br.Stats = CrawlStats{
		PagesCrawled: int(crawled.Load()),
		PagesBlocked: int(blocked.Load()),
		TotalTimeMs:  time.Since(start).Milliseconds(),
	}

	return br
}
