package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/crawl"
)

// ---- helpers ----

func testDeps() *Deps {
	return &Deps{
		Cfg: Config{
			MaxConcurrent:    4,
			DefaultWaitMs:    100,
			RequestTimeoutMs: 5000,
		},
		RateLimiter: crawl.NewRateLimiter(),
	}
}

// fakeCrawlFn returns a crawl function that immediately returns a canned
// response for each URL. The optional sleepMs parameter inserts a brief delay
// so we can observe concurrency behaviour.
func fakeCrawlFn(sleepMs int) func(ctx context.Context, req CrawlRequest) CrawlResponse {
	return func(ctx context.Context, req CrawlRequest) CrawlResponse {
		if sleepMs > 0 {
			time.Sleep(time.Duration(sleepMs) * time.Millisecond)
		}
		return CrawlResponse{
			URL:        req.URL,
			StatusCode: 200,
		}
	}
}

func postJSON(t *testing.T, handler http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// ---- /crawl-many tests ----

func TestCrawlManyHandler_NonPost(t *testing.T) {
	deps := testDeps()
	h := crawlManyHandlerWithFn(deps, fakeCrawlFn(0))

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/crawl-many", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: want 405, got %d", method, rr.Code)
		}
	}
}

func TestCrawlManyHandler_EmptyURLs(t *testing.T) {
	deps := testDeps()
	h := crawlManyHandlerWithFn(deps, fakeCrawlFn(0))

	rr := postJSON(t, h, map[string]any{"urls": []string{}})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCrawlManyHandler_MissingURLs(t *testing.T) {
	deps := testDeps()
	h := crawlManyHandlerWithFn(deps, fakeCrawlFn(0))

	rr := postJSON(t, h, map[string]any{})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCrawlManyHandler_AllURLsProcessed(t *testing.T) {
	deps := testDeps()
	urls := []string{"http://a.test/1", "http://b.test/2", "http://c.test/3"}

	h := crawlManyHandlerWithFn(deps, fakeCrawlFn(0))
	rr := postJSON(t, h, map[string]any{"urls": urls})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var resp BatchCrawlResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Results) != len(urls) {
		t.Errorf("want %d results, got %d", len(urls), len(resp.Results))
	}
	if resp.Stats.Total != len(urls) {
		t.Errorf("stats.total: want %d, got %d", len(urls), resp.Stats.Total)
	}
}

func TestCrawlManyHandler_ConcurrencyCapRespected(t *testing.T) {
	const maxConcurrent = 2
	const numURLs = 6

	deps := testDeps()
	deps.Cfg.MaxConcurrent = maxConcurrent

	// Track peak concurrency.
	var active atomic.Int64
	var peak atomic.Int64

	crawlFn := func(ctx context.Context, req CrawlRequest) CrawlResponse {
		cur := active.Add(1)
		defer active.Add(-1)

		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond) // hold the slot briefly
		return CrawlResponse{URL: req.URL, StatusCode: 200}
	}

	urls := make([]string, numURLs)
	for i := range urls {
		urls[i] = "http://example.test/" + string(rune('a'+i))
	}

	h := crawlManyHandlerWithFn(deps, crawlFn)
	rr := postJSON(t, h, map[string]any{
		"urls":           urls,
		"max_concurrent": maxConcurrent,
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if got := peak.Load(); got > maxConcurrent {
		t.Errorf("concurrency cap breached: peak=%d, max=%d", got, maxConcurrent)
	}
}

// ---- /crawl-stream tests ----

func TestCrawlStreamHandler_NonPost(t *testing.T) {
	deps := testDeps()
	h := crawlStreamHandlerWithFn(deps, fakeCrawlFn(0))

	req := httptest.NewRequest(http.MethodGet, "/crawl-stream", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

func TestCrawlStreamHandler_EmptyURLs(t *testing.T) {
	deps := testDeps()
	h := crawlStreamHandlerWithFn(deps, fakeCrawlFn(0))

	rr := postJSON(t, h, map[string]any{"urls": []string{}})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCrawlStreamHandler_SSEFormat(t *testing.T) {
	deps := testDeps()
	urls := []string{"http://x.test/1", "http://x.test/2"}

	h := crawlStreamHandlerWithFn(deps, fakeCrawlFn(0))
	rr := postJSON(t, h, map[string]any{"urls": urls})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type want text/event-stream, got %q", ct)
	}

	// Parse SSE lines.
	var dataLines []string
	var doneEvents []string
	scanner := bufio.NewScanner(rr.Body)
	var lastEvent string
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			lastEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if lastEvent == "done" {
				doneEvents = append(doneEvents, data)
			} else {
				dataLines = append(dataLines, data)
			}
			lastEvent = ""
		}
	}

	// Should have one data event per URL.
	if len(dataLines) != len(urls) {
		t.Errorf("want %d data events, got %d", len(urls), len(dataLines))
	}
	// Should have exactly one done event.
	if len(doneEvents) != 1 {
		t.Errorf("want 1 done event, got %d", len(doneEvents))
	}

	// Done event must be valid JSON with a total field.
	var stats BatchStats
	if err := json.Unmarshal([]byte(doneEvents[0]), &stats); err != nil {
		t.Fatalf("done stats JSON: %v", err)
	}
	if stats.Total != len(urls) {
		t.Errorf("done stats.total: want %d, got %d", len(urls), stats.Total)
	}
}

// ---- sseEvent unit tests ----

func TestSSEEvent_DataOnly(t *testing.T) {
	got := sseEvent("", "hello")
	want := "data: hello\n\n"
	if got != want {
		t.Errorf("sseEvent: want %q, got %q", want, got)
	}
}

func TestSSEEvent_WithType(t *testing.T) {
	got := sseEvent("done", `{"total":3}`)
	want := "event: done\ndata: {\"total\":3}\n\n"
	if got != want {
		t.Errorf("sseEvent: want %q, got %q", want, got)
	}
}

// ---- batchOrchestrator unit tests ----

func TestBatchOrchestrator_AllURLsReturned(t *testing.T) {
	urls := []string{"http://a.test/", "http://b.test/", "http://c.test/"}

	orch := &batchOrchestrator{
		crawlFn: fakeCrawlFn(0),
	}

	results, stats := orch.run(context.Background(), urls, CrawlRequest{Output: "markdown"}, 3, nil, nil)

	if len(results) != len(urls) {
		t.Errorf("want %d results, got %d", len(urls), len(results))
	}
	if stats.Total != len(urls) {
		t.Errorf("stats.total: want %d, got %d", len(urls), stats.Total)
	}
	if stats.OK != len(urls) {
		t.Errorf("stats.ok: want %d, got %d", len(urls), stats.OK)
	}
}

func TestBatchOrchestrator_ConcurrencyCap(t *testing.T) {
	const maxConcurrent = 2
	const numURLs = 8

	var active atomic.Int64
	var peak atomic.Int64

	crawlFn := func(ctx context.Context, req CrawlRequest) CrawlResponse {
		cur := active.Add(1)
		defer active.Add(-1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		return CrawlResponse{URL: req.URL, StatusCode: 200}
	}

	urls := make([]string, numURLs)
	for i := range urls {
		urls[i] = "http://ex.test/" + string(rune('a'+i))
	}

	orch := &batchOrchestrator{crawlFn: crawlFn}
	results, _ := orch.run(context.Background(), urls, CrawlRequest{}, maxConcurrent, nil, nil)

	if len(results) != numURLs {
		t.Errorf("want %d results, got %d", numURLs, len(results))
	}
	if got := peak.Load(); got > maxConcurrent {
		t.Errorf("concurrency cap breached: peak=%d, max=%d", got, maxConcurrent)
	}
}

func TestBatchOrchestrator_BlockedCounted(t *testing.T) {
	urls := []string{"http://a.test/", "http://b.test/"}

	crawlFn := func(ctx context.Context, req CrawlRequest) CrawlResponse {
		return CrawlResponse{URL: req.URL, StatusCode: 403, Blocked: true}
	}

	orch := &batchOrchestrator{crawlFn: crawlFn}
	_, stats := orch.run(context.Background(), urls, CrawlRequest{}, 4, nil, nil)

	if stats.Blocked != len(urls) {
		t.Errorf("want %d blocked, got %d", len(urls), stats.Blocked)
	}
	if stats.OK != 0 {
		t.Errorf("want 0 ok, got %d", stats.OK)
	}
}
