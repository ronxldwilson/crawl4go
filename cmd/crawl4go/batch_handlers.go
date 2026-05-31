package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// BatchCrawlRequest is the JSON body for POST /crawl-many and POST /crawl-stream.
type BatchCrawlRequest struct {
	URLs          []string `json:"urls"`
	MaxConcurrent int      `json:"max_concurrent"`
	WaitMs        int      `json:"wait_ms"`
	Scroll        bool     `json:"scroll"`
	Output        string   `json:"output"`
	Prune         bool     `json:"prune"`
	Proxy         bool     `json:"proxy"`
}

// BatchStats summarises a completed batch crawl.
type BatchStats struct {
	Total     int   `json:"total"`
	OK        int   `json:"ok"`
	Blocked   int   `json:"blocked"`
	ElapsedMs int64 `json:"elapsed_ms"`
}

// BatchCrawlResponse is the JSON response for POST /crawl-many.
type BatchCrawlResponse struct {
	Results []CrawlResponse `json:"results"`
	Stats   BatchStats      `json:"stats"`
}

// crawlManyFn is the per-URL crawl function signature used by the batch
// orchestration helpers. It is a field so tests can inject a fake.
type batchOrchestrator struct {
	crawlFn func(ctx context.Context, req CrawlRequest) CrawlResponse
}

// run concurrently crawls all urls, capping parallelism to maxConcurrent.
// It also calls rateLimitWait before each URL and records the result
// afterwards (both are no-ops when the funcs are nil, as in tests).
func (o *batchOrchestrator) run(
	ctx context.Context,
	urls []string,
	baseReq CrawlRequest,
	maxConcurrent int,
	rateLimitWait func(ctx context.Context, url string) error,
	rateLimitRecord func(url string, status int),
) ([]CrawlResponse, BatchStats) {
	start := time.Now()

	sem := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	results := make([]CrawlResponse, 0, len(urls))
	var okCount, blockedCount atomic.Int64

	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Add(1)
		go func(pageURL string) {
			defer wg.Done()

			// Acquire concurrency slot.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				mu.Lock()
				results = append(results, CrawlResponse{URL: pageURL})
				mu.Unlock()
				return
			}

			// Per-domain rate limiting.
			if rateLimitWait != nil {
				if err := rateLimitWait(ctx, pageURL); err != nil {
					mu.Lock()
					results = append(results, CrawlResponse{URL: pageURL})
					mu.Unlock()
					return
				}
			}

			req := baseReq
			req.URL = pageURL
			resp := o.crawlFn(ctx, req)

			if rateLimitRecord != nil {
				rateLimitRecord(pageURL, resp.StatusCode)
			}

			if !resp.Blocked && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				okCount.Add(1)
			}
			if resp.Blocked {
				blockedCount.Add(1)
			}

			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
		}(u)
	}
	wg.Wait()

	stats := BatchStats{
		Total:     len(urls),
		OK:        int(okCount.Load()),
		Blocked:   int(blockedCount.Load()),
		ElapsedMs: time.Since(start).Milliseconds(),
	}
	return results, stats
}

// sseEvent formats a single SSE event line.
// Pure function — easy to unit-test.
func sseEvent(eventType, data string) string {
	if eventType == "" {
		return fmt.Sprintf("data: %s\n\n", data)
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
}

// registerBatchRoutes wires /crawl-many and /crawl-stream into mux.
// Call this from main.go after building deps:
//
//	registerBatchRoutes(mux, deps)
func registerBatchRoutes(mux *http.ServeMux, deps *Deps) {
	mux.HandleFunc("/crawl-many", crawlManyHandler(deps))
	mux.HandleFunc("/crawl-stream", crawlStreamHandler(deps))
}

// crawlManyHandler handles POST /crawl-many.
func crawlManyHandler(deps *Deps) http.HandlerFunc {
	return crawlManyHandlerWithFn(deps, nil)
}

// crawlManyHandlerWithFn is the testable variant that allows injecting a fake
// per-URL crawl function. Pass nil to use the real crawlSinglePage.
func crawlManyHandlerWithFn(deps *Deps, injectedCrawlFn func(ctx context.Context, req CrawlRequest) CrawlResponse) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req BatchCrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if len(req.URLs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "urls is required and must be non-empty"})
			return
		}

		// Apply defaults.
		if req.MaxConcurrent <= 0 {
			req.MaxConcurrent = deps.Cfg.MaxConcurrent
		}
		if req.WaitMs <= 0 {
			req.WaitMs = deps.Cfg.DefaultWaitMs
		}
		if req.Output == "" {
			req.Output = "markdown"
		}

		// Build base CrawlRequest (URL will be overridden per-URL).
		baseReq := CrawlRequest{
			WaitMs: req.WaitMs,
			Scroll: req.Scroll,
			Output: req.Output,
			Prune:  req.Prune,
			Proxy:  req.Proxy,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		crawlFn := injectedCrawlFn
		if crawlFn == nil {
			crawlFn = func(ctx context.Context, cr CrawlRequest) CrawlResponse {
				return crawlSinglePageCached(ctx, deps, cr)
			}
		}

		orch := &batchOrchestrator{crawlFn: crawlFn}
		results, stats := orch.run(
			ctx,
			req.URLs,
			baseReq,
			req.MaxConcurrent,
			deps.RateLimiter.Wait,
			deps.RateLimiter.RecordResult,
		)

		slog.Info("crawl-many completed",
			"total", stats.Total,
			"ok", stats.OK,
			"blocked", stats.Blocked,
			"elapsed_ms", stats.ElapsedMs,
		)

		writeJSON(w, http.StatusOK, BatchCrawlResponse{
			Results: results,
			Stats:   stats,
		})
	}
}

// crawlStreamHandler handles POST /crawl-stream — SSE variant.
func crawlStreamHandler(deps *Deps) http.HandlerFunc {
	return crawlStreamHandlerWithFn(deps, nil)
}

// crawlStreamHandlerWithFn is the testable variant that allows injecting a fake
// per-URL crawl function. Pass nil to use the real crawlSinglePage.
func crawlStreamHandlerWithFn(deps *Deps, injectedCrawlFn func(ctx context.Context, req CrawlRequest) CrawlResponse) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req BatchCrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if len(req.URLs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "urls is required and must be non-empty"})
			return
		}

		// Apply defaults.
		if req.MaxConcurrent <= 0 {
			req.MaxConcurrent = deps.Cfg.MaxConcurrent
		}
		if req.WaitMs <= 0 {
			req.WaitMs = deps.Cfg.DefaultWaitMs
		}
		if req.Output == "" {
			req.Output = "markdown"
		}

		// Ensure the client supports SSE flushing.
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		baseReq := CrawlRequest{
			WaitMs: req.WaitMs,
			Scroll: req.Scroll,
			Output: req.Output,
			Prune:  req.Prune,
			Proxy:  req.Proxy,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		crawlFn := injectedCrawlFn
		if crawlFn == nil {
			crawlFn = func(ctx context.Context, cr CrawlRequest) CrawlResponse {
				return crawlSinglePageCached(ctx, deps, cr)
			}
		}

		start := time.Now()
		sem := make(chan struct{}, req.MaxConcurrent)
		resultCh := make(chan CrawlResponse, len(req.URLs))

		var wg sync.WaitGroup
		for _, u := range req.URLs {
			wg.Add(1)
			go func(pageURL string) {
				defer wg.Done()

				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					resultCh <- CrawlResponse{URL: pageURL}
					return
				}

				if err := deps.RateLimiter.Wait(ctx, pageURL); err != nil {
					resultCh <- CrawlResponse{URL: pageURL}
					return
				}

				cr := baseReq
				cr.URL = pageURL
				resp := crawlFn(ctx, cr)

				deps.RateLimiter.RecordResult(pageURL, resp.StatusCode)
				resultCh <- resp
			}(u)
		}

		// Close the result channel once all goroutines finish.
		go func() {
			wg.Wait()
			close(resultCh)
		}()

		var okCount, blockedCount int
		for {
			select {
			case <-ctx.Done():
				// Client disconnected or timeout — send done event and exit.
				stats := BatchStats{
					Total:     len(req.URLs),
					OK:        okCount,
					Blocked:   blockedCount,
					ElapsedMs: time.Since(start).Milliseconds(),
				}
				statsJSON, _ := json.Marshal(stats)
				fmt.Fprint(w, sseEvent("done", string(statsJSON)))
				flusher.Flush()
				return

			case resp, open := <-resultCh:
				if !open {
					// All done — send final stats.
					stats := BatchStats{
						Total:     len(req.URLs),
						OK:        okCount,
						Blocked:   blockedCount,
						ElapsedMs: time.Since(start).Milliseconds(),
					}
					statsJSON, _ := json.Marshal(stats)
					fmt.Fprint(w, sseEvent("done", string(statsJSON)))
					flusher.Flush()

					slog.Info("crawl-stream completed",
						"total", stats.Total,
						"ok", stats.OK,
						"blocked", stats.Blocked,
						"elapsed_ms", stats.ElapsedMs,
					)
					return
				}

				if !resp.Blocked && resp.StatusCode >= 200 && resp.StatusCode < 300 {
					okCount++
				}
				if resp.Blocked {
					blockedCount++
				}

				respJSON, _ := json.Marshal(resp)
				fmt.Fprint(w, sseEvent("", string(respJSON)))
				flusher.Flush()
			}
		}
	}
}
