package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/browser"
	"github.com/ronxldwilson/crawl4go/internal/content"
)

// registerLLMExtractRoutes registers the /extract-llm and /extract-cosine
// endpoints on mux. Call this from main after building the deps bundle.
func registerLLMExtractRoutes(mux *http.ServeMux, deps *Deps) {
	mux.HandleFunc("/extract-llm", extractLLMHandler(deps))
	mux.HandleFunc("/extract-cosine", extractCosineHandler(deps))
}

// --- Request types ---

// LLMExtractRequest is the JSON body for POST /extract-llm.
type LLMExtractRequest struct {
	URL       string `json:"url"`
	WaitMs    int    `json:"wait_ms"`
	Proxy     bool   `json:"proxy"`
	Prune     bool   `json:"prune"`
	Schema    string `json:"schema"`     // optional JSON schema string
	ChunkSize int    `json:"chunk_size"` // defaults to strategy default (4000)
	Overlap   int    `json:"overlap"`    // defaults to strategy default (200)
}

// LLMExtractResponse is the JSON body returned by POST /extract-llm.
type LLMExtractResponse struct {
	URL          string                     `json:"url"`
	Results      []content.ExtractionResult `json:"results"`
	InputTokens  int                        `json:"input_tokens"`
	OutputTokens int                        `json:"output_tokens"`
}

// CosineExtractRequest is the JSON body for POST /extract-cosine.
type CosineExtractRequest struct {
	URL       string  `json:"url"`
	WaitMs    int     `json:"wait_ms"`
	Proxy     bool    `json:"proxy"`
	Threshold float64 `json:"threshold"` // cosine similarity threshold; 0 → use default (0.6)
	TopN      int     `json:"top_n"`     // number of clusters to return; 0 → use default (5)
}

// CosineExtractResponse is the JSON body returned by POST /extract-cosine.
type CosineExtractResponse struct {
	URL     string                     `json:"url"`
	Results []content.ExtractionResult `json:"results"`
}

// --- Handlers ---

// extractLLMHandler returns an http.HandlerFunc for POST /extract-llm.
func extractLLMHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		if !deps.LLMEnabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llm not configured"})
			return
		}

		var req LLMExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = deps.Cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(deps.Cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = deps.Cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, deps.CDP, deps.HTTP, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		if req.Prune {
			if pruned, pruneErr := deps.Pruner.Filter(htmlContent); pruneErr == nil && len(pruned) > 0 {
				htmlContent = pruned
			}
		}

		strategy := content.NewLLMExtractionStrategy(deps.LLM)
		if req.ChunkSize > 0 {
			strategy.ChunkSize = req.ChunkSize
		}
		if req.Overlap > 0 {
			strategy.Overlap = req.Overlap
		}
		if req.Schema != "" {
			strategy.Schema = req.Schema
		}

		results, err := strategy.Extract(ctx, htmlContent, content.ExtractionConfig{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "llm extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, LLMExtractResponse{
			URL:          req.URL,
			Results:      results,
			InputTokens:  strategy.InputTokens,
			OutputTokens: strategy.OutputTokens,
		})
	}
}

// extractCosineHandler returns an http.HandlerFunc for POST /extract-cosine.
func extractCosineHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		if !deps.LLMEnabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llm not configured"})
			return
		}

		var req CosineExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = deps.Cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(deps.Cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = deps.Cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, deps.CDP, deps.HTTP, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		strategy := content.NewCosineStrategy(deps.LLM)
		if req.Threshold > 0 {
			strategy.Threshold = req.Threshold
		}
		if req.TopN > 0 {
			strategy.TopN = req.TopN
		}

		results, err := strategy.Extract(ctx, htmlContent, content.ExtractionConfig{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cosine extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, CosineExtractResponse{
			URL:     req.URL,
			Results: results,
		})
	}
}
