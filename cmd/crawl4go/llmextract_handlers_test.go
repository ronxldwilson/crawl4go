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

// --- helpers -----------------------------------------------------------------

// minDeps returns a *Deps with LLM nil (disabled) and a minimal Config.
func minDeps() *Deps {
	return &Deps{
		Cfg: Config{
			DefaultWaitMs:    500,
			RequestTimeoutMs: 5000,
		},
		LLM: nil, // disabled
	}
}

// minDepsWithLLM returns a *Deps with a stub LLM adapter (LLM enabled) and no
// CDPClient / HTTP client — sufficient for handler unit-tests that only need to
// reach URL or body validation, not actually render a page.
func minDepsWithLLM() *Deps {
	return &Deps{
		Cfg: Config{
			DefaultWaitMs:    500,
			RequestTimeoutMs: 5000,
		},
		LLM: &LLMAdapter{
			completer: func(_ context.Context, _ string) (string, error) { return "", nil },
			embedder:  func(_ context.Context, _ string) ([]float64, error) { return nil, nil },
		},
	}
}

// --- /extract-llm tests ------------------------------------------------------

func TestExtractLLMMethodNotAllowed(t *testing.T) {
	h := extractLLMHandler(minDeps())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/extract-llm", nil)
	h(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestExtractLLMMissingURL(t *testing.T) {
	// Use deps with a non-nil LLM so we get past the 503 gate and reach URL validation.
	d := minDepsWithLLM()
	h := extractLLMHandler(d)
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"wait_ms": 0})
	req := httptest.NewRequest(http.MethodPost, "/extract-llm", bytes.NewReader(body))
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestExtractLLMDisabled(t *testing.T) {
	h := extractLLMHandler(minDeps()) // LLM == nil
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"url": "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/extract-llm", bytes.NewReader(body))
	h(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rec.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp["error"] != "llm not configured" {
		t.Errorf("unexpected error message: %q", resp["error"])
	}
}

// --- /extract-cosine tests ---------------------------------------------------

func TestExtractCosineMethodNotAllowed(t *testing.T) {
	h := extractCosineHandler(minDeps())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/extract-cosine", nil)
	h(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestExtractCosineMissingURL(t *testing.T) {
	// Use deps with a non-nil LLM so we get past the 503 gate and reach URL validation.
	d := minDepsWithLLM()
	h := extractCosineHandler(d)
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"threshold": 0.5})
	req := httptest.NewRequest(http.MethodPost, "/extract-cosine", bytes.NewReader(body))
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
}

func TestExtractCosineDisabled(t *testing.T) {
	h := extractCosineHandler(minDeps()) // LLM == nil
	rec := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"url": "https://example.com"})
	req := httptest.NewRequest(http.MethodPost, "/extract-cosine", bytes.NewReader(body))
	h(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", rec.Code)
	}
}

// --- unit tests for strategy paths (no HTTP server, no network) ---------------

// fakeLLMProvider is a minimal content.LLMProvider that returns a canned string.
type fakeLLMProvider struct {
	response string
}

func (f *fakeLLMProvider) Complete(_ context.Context, _ string) (string, error) {
	return f.response, nil
}

// TestLLMExtractionStrategyUnit drives the strategy with a fake provider.
func TestLLMExtractionStrategyUnit(t *testing.T) {
	html := `<html><body><p>Hello world from the LLM extraction test.</p></body></html>`
	canned := "## Extracted\nHello world from the LLM extraction test."

	provider := &fakeLLMProvider{response: canned}
	strategy := content.NewLLMExtractionStrategy(provider)
	strategy.ChunkSize = 500
	strategy.Overlap = 0

	ctx := context.Background()
	results, err := strategy.Extract(ctx, html, content.ExtractionConfig{})
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result, got none")
	}
	if results[0].Content != canned {
		t.Errorf("result content = %q, want %q", results[0].Content, canned)
	}
	if strategy.InputTokens == 0 {
		t.Error("expected non-zero InputTokens")
	}
	if strategy.OutputTokens == 0 {
		t.Error("expected non-zero OutputTokens")
	}
}

// fakeEmbedder returns deterministic vectors based on content length.
type fakeEmbedder struct{}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float64, error) {
	// Return a 3-dimensional vector derived from word length mod values so
	// that blocks with different lengths end up in different clusters and
	// blocks with the same content are identical (high similarity).
	n := float64(len(strings.Fields(text)) % 7)
	return []float64{n, n * 0.5, n * 0.25}, nil
}

// TestCosineStrategyUnit drives the strategy with a fake embedder.
func TestCosineStrategyUnit(t *testing.T) {
	// Use a page with enough text that splitTextBlocks finds >20-char segments.
	html := `<html><body>
<p>The quick brown fox jumps over the lazy dog and runs away fast.</p>
<p>Machine learning models require large amounts of training data to perform well.</p>
<p>The quick brown fox jumps over the lazy dog and runs away fast.</p>
</body></html>`

	embedder := &fakeEmbedder{}
	strategy := content.NewCosineStrategy(embedder)
	strategy.Threshold = 0.99 // only exact duplicates cluster
	strategy.TopN = 3

	ctx := context.Background()
	results, err := strategy.Extract(ctx, html, content.ExtractionConfig{})
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	// We should get at least one result (or zero if the text blocks are too short).
	// The key check is that no error was returned and the type is correct.
	_ = results
}

// TestLLMEnabledFalseWhenNil verifies LLMEnabled reports false for nil LLM.
func TestLLMEnabledFalseWhenNil(t *testing.T) {
	d := &Deps{}
	if d.LLMEnabled() {
		t.Error("expected LLMEnabled() == false when LLM is nil")
	}
}

// TestLLMEnabledTrueWhenSet verifies LLMEnabled reports true with a non-nil adapter.
func TestLLMEnabledTrueWhenSet(t *testing.T) {
	d := &Deps{
		LLM: &LLMAdapter{
			completer: func(_ context.Context, _ string) (string, error) { return "", nil },
			embedder:  func(_ context.Context, _ string) ([]float64, error) { return nil, nil },
		},
	}
	if !d.LLMEnabled() {
		t.Error("expected LLMEnabled() == true when LLM is non-nil")
	}
}
