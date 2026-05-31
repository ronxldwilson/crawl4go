package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeLLMCompleter is a content.LLMCompleter that returns "NOT_RELEVANT" when
// the prompt contains a marker string, and returns a markdown snippet otherwise.
type fakeLLMCompleter struct {
	// dropMarker is a substring; if found in the prompt the completer returns
	// "NOT_RELEVANT". Otherwise it returns a fixed markdown string.
	dropMarker string
}

func (f *fakeLLMCompleter) Complete(_ context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, f.dropMarker) {
		return "NOT_RELEVANT", nil
	}
	return "## Relevant\n\nThis block is relevant.", nil
}

// newDepsEnabled builds a Deps whose LLM field is non-nil (so LLMEnabled()
// returns true). The handler-level tests here only exercise method/validation
// paths that return before any page render or LLM call, so the adapter funcs
// are simple stubs and no CDP/HTTP client is needed. Filter logic is covered
// separately by the fakeLLMCompleter unit tests below.
func newDepsEnabled(_ string) *Deps {
	return &Deps{
		Cfg:    Config{DefaultWaitMs: 1500, TorProxyURL: "http://tor:3128"},
		Pruner: content.NewPruningFilter(),
		LLM: &LLMAdapter{
			completer: func(_ context.Context, _ string) (string, error) { return "", nil },
			embedder:  func(_ context.Context, _ string) ([]float64, error) { return nil, nil },
		},
	}
}

func newDepsDisabled() *Deps {
	return &Deps{
		Cfg:    Config{DefaultWaitMs: 1500},
		Pruner: content.NewPruningFilter(),
		LLM:    nil, // disabled
	}
}

// postFilterJSON sends a POST request with JSON body to h and returns the recorder.
func postFilterJSON(t *testing.T, h http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func getFilterRequest(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// /llm-filter — method / validation / disabled checks
// ---------------------------------------------------------------------------

func TestLLMFilterHandler_NonPost(t *testing.T) {
	h := llmFilterHandler(newDepsEnabled("<html><body>hello world</body></html>"))
	rr := getFilterRequest(t, h, "/llm-filter")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

func TestLLMFilterHandler_LLMDisabled(t *testing.T) {
	h := llmFilterHandler(newDepsDisabled())
	rr := postFilterJSON(t, h, map[string]string{"url": "http://example.com", "query": "test"})
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rr.Code)
	}
}

func TestLLMFilterHandler_MissingURL(t *testing.T) {
	h := llmFilterHandler(newDepsEnabled("<html><body>hello world</body></html>"))
	rr := postFilterJSON(t, h, map[string]string{"query": "test"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestLLMFilterHandler_MissingQuery(t *testing.T) {
	h := llmFilterHandler(newDepsEnabled("<html><body>hello world</body></html>"))
	rr := postFilterJSON(t, h, map[string]string{"url": "http://example.com"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// /crawl-fit — method / validation / disabled checks
// ---------------------------------------------------------------------------

func TestCrawlFitHandler_NonPost(t *testing.T) {
	h := crawlFitHandler(newDepsEnabled("<html><body>hello world</body></html>"))
	rr := getFilterRequest(t, h, "/crawl-fit")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rr.Code)
	}
}

func TestCrawlFitHandler_LLMDisabled(t *testing.T) {
	h := crawlFitHandler(newDepsDisabled())
	rr := postFilterJSON(t, h, map[string]string{"url": "http://example.com", "query": "test"})
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rr.Code)
	}
}

func TestCrawlFitHandler_MissingURL(t *testing.T) {
	h := crawlFitHandler(newDepsEnabled("<html><body>hello world</body></html>"))
	rr := postFilterJSON(t, h, map[string]string{"query": "test"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCrawlFitHandler_MissingQuery(t *testing.T) {
	h := crawlFitHandler(newDepsEnabled("<html><body>hello world</body></html>"))
	rr := postFilterJSON(t, h, map[string]string{"url": "http://example.com"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for LLMContentFilter with fakeLLMCompleter
// ---------------------------------------------------------------------------

func TestLLMFilter_KeepsDrop(t *testing.T) {
	// The completer drops blocks containing "NOISE" and keeps others.
	completer := &fakeLLMCompleter{dropMarker: "NOISE"}

	blocks := []string{
		"This is a NOISE advertisement block",
		"This is useful content about the topic",
		"More NOISE navigation links",
		"Another relevant paragraph about the subject",
	}

	filter := content.NewLLMContentFilter(completer)
	results, err := filter.Filter(context.Background(), blocks, "useful topic")
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}

	if len(results) != len(blocks) {
		t.Fatalf("want %d results, got %d", len(blocks), len(results))
	}

	// Blocks 0 and 2 contain "NOISE" → dropped.
	for _, idx := range []int{0, 2} {
		if results[idx].Kept {
			t.Errorf("block %d should be dropped (contains NOISE)", idx)
		}
	}
	// Blocks 1 and 3 do not contain "NOISE" → kept.
	for _, idx := range []int{1, 3} {
		if !results[idx].Kept {
			t.Errorf("block %d should be kept", idx)
		}
	}
}

func TestLLMFilter_AllKept(t *testing.T) {
	completer := &fakeLLMCompleter{dropMarker: "XXXXXXNOMATCH"} // never matches
	blocks := []string{"alpha", "beta", "gamma"}
	filter := content.NewLLMContentFilter(completer)
	results, err := filter.Filter(context.Background(), blocks, "anything")
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}
	for i, r := range results {
		if !r.Kept {
			t.Errorf("block %d should be kept", i)
		}
	}
}

func TestLLMFilter_AllDropped(t *testing.T) {
	// drop everything by matching on the query itself (always in the prompt).
	completer := &fakeLLMCompleter{dropMarker: "anything"} // query appears in every prompt
	blocks := []string{"alpha", "beta"}
	filter := content.NewLLMContentFilter(completer)
	// Use a query that contains "anything" so every prompt matches.
	results, err := filter.Filter(context.Background(), blocks, "anything")
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}
	for i, r := range results {
		if r.Kept {
			t.Errorf("block %d should be dropped", i)
		}
	}
}

// ---------------------------------------------------------------------------
// Unit tests for joinKeptBlocks helper
// ---------------------------------------------------------------------------

func TestJoinKeptBlocks_Basic(t *testing.T) {
	blocks := []content.FilteredBlock{
		{Content: "First kept block", Kept: true},
		{Content: "This is dropped", Kept: false},
		{Content: "Second kept block", Kept: true},
	}
	got := joinKeptBlocks(blocks)
	want := "First kept block\n\nSecond kept block"
	if got != want {
		t.Errorf("joinKeptBlocks =\n%q\nwant\n%q", got, want)
	}
}

func TestJoinKeptBlocks_Empty(t *testing.T) {
	if got := joinKeptBlocks(nil); got != "" {
		t.Errorf("joinKeptBlocks(nil) = %q, want empty", got)
	}
}

func TestJoinKeptBlocks_AllDropped(t *testing.T) {
	blocks := []content.FilteredBlock{
		{Content: "dropped", Kept: false},
	}
	if got := joinKeptBlocks(blocks); got != "" {
		t.Errorf("joinKeptBlocks all-dropped = %q, want empty", got)
	}
}

func TestJoinKeptBlocks_Single(t *testing.T) {
	blocks := []content.FilteredBlock{
		{Content: "only one", Kept: true},
	}
	if got := joinKeptBlocks(blocks); got != "only one" {
		t.Errorf("joinKeptBlocks single = %q, want 'only one'", got)
	}
}

// ---------------------------------------------------------------------------
// registerLLMFilterRoutes — smoke test
// ---------------------------------------------------------------------------

func TestRegisterLLMFilterRoutes_Wires(t *testing.T) {
	mux := http.NewServeMux()
	deps := newDepsDisabled() // LLM disabled → 503 on both endpoints
	registerLLMFilterRoutes(mux, deps)

	for _, path := range []string{"/llm-filter", "/crawl-fit"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"url":"http://x.com","query":"q"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: want 503, got %d", path, rr.Code)
		}
	}
}
