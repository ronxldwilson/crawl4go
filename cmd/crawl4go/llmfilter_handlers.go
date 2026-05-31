package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ronxldwilson/crawl4go/internal/browser"
	"github.com/ronxldwilson/crawl4go/internal/content"
)

// Deps, LLMAdapter, and LLMEnabled are defined in deps.go (shared bundle).

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

// LLMFilterRequest is the payload for POST /llm-filter.
type LLMFilterRequest struct {
	URL         string  `json:"url"`
	Query       string  `json:"query"`
	WaitMs      int     `json:"wait_ms"`
	Proxy       bool    `json:"proxy"`
	Prune       bool    `json:"prune"`
	Instruction string  `json:"instruction"`
	Threshold   float64 `json:"threshold"`
}

// LLMFilterResponse is the response body for POST /llm-filter.
type LLMFilterResponse struct {
	URL         string                  `json:"url"`
	Query       string                  `json:"query"`
	TotalBlocks int                     `json:"total_blocks"`
	Kept        int                     `json:"kept"`
	Blocks      []content.FilteredBlock `json:"blocks"`
}

// CrawlFitRequest is the payload for POST /crawl-fit.
type CrawlFitRequest struct {
	URL    string `json:"url"`
	Query  string `json:"query"`
	WaitMs int    `json:"wait_ms"`
	Proxy  bool   `json:"proxy"`
}

// CrawlFitResponse is the response body for POST /crawl-fit.
type CrawlFitResponse struct {
	URL         string `json:"url"`
	FitMarkdown string `json:"fit_markdown"`
	KeptBlocks  int    `json:"kept_blocks"`
	TotalBlocks int    `json:"total_blocks"`
}

// ---------------------------------------------------------------------------
// joinKeptBlocks — testable helper that joins kept FilteredBlocks
// ---------------------------------------------------------------------------

// joinKeptBlocks concatenates the Content of every kept FilteredBlock,
// separated by double newlines. It is a pure function with no side effects so
// it can be unit-tested without HTTP or LLM infrastructure.
func joinKeptBlocks(blocks []content.FilteredBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Kept {
			parts = append(parts, b.Content)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ---------------------------------------------------------------------------
// textChunksToStrings — converts []TextChunk to []string
// ---------------------------------------------------------------------------

func textChunksToStrings(chunks []content.TextChunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Text
	}
	return out
}

// ---------------------------------------------------------------------------
// llmFilterHandler — POST /llm-filter
// ---------------------------------------------------------------------------

// llmFilterHandler handles POST /llm-filter.
func llmFilterHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		if !deps.LLMEnabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM not configured"})
			return
		}

		var req LLMFilterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.Query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = deps.Cfg.DefaultWaitMs
		}

		ctx := r.Context()
		proxyURL := ""
		if req.Proxy {
			proxyURL = deps.Cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, deps.CDP, deps.HTTP, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		if req.Prune && deps.Pruner != nil {
			if pruned, pErr := deps.Pruner.Filter(htmlContent); pErr == nil && len(pruned) > 0 {
				htmlContent = pruned
			}
		}

		textChunks := content.ExtractTextChunks(htmlContent)
		blocks := textChunksToStrings(textChunks)

		filter := content.NewLLMContentFilter(deps.LLM)
		if req.Instruction != "" {
			filter.Instruction = req.Instruction
		}
		if req.Threshold > 0 {
			filter.Threshold = req.Threshold
		}

		results, err := filter.Filter(ctx, blocks, req.Query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "filter failed: " + err.Error()})
			return
		}

		kept := 0
		var keptBlocks []content.FilteredBlock
		for _, b := range results {
			if b.Kept {
				kept++
				keptBlocks = append(keptBlocks, b)
			}
		}

		writeJSON(w, http.StatusOK, LLMFilterResponse{
			URL:         req.URL,
			Query:       req.Query,
			TotalBlocks: len(blocks),
			Kept:        kept,
			Blocks:      keptBlocks,
		})
	}
}

// ---------------------------------------------------------------------------
// crawlFitHandler — POST /crawl-fit
// ---------------------------------------------------------------------------

// crawlFitHandler handles POST /crawl-fit. It renders the page, derives text
// blocks, runs the LLM filter, and joins kept blocks into "fit markdown".
func crawlFitHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		if !deps.LLMEnabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LLM not configured"})
			return
		}

		var req CrawlFitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.Query == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = deps.Cfg.DefaultWaitMs
		}

		ctx := r.Context()
		proxyURL := ""
		if req.Proxy {
			proxyURL = deps.Cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, deps.CDP, deps.HTTP, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		textChunks := content.ExtractTextChunks(htmlContent)
		blocks := textChunksToStrings(textChunks)

		filter := content.NewLLMContentFilter(deps.LLM)
		results, err := filter.Filter(ctx, blocks, req.Query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "filter failed: " + err.Error()})
			return
		}

		fitMarkdown := joinKeptBlocks(results)

		kept := 0
		for _, b := range results {
			if b.Kept {
				kept++
			}
		}

		writeJSON(w, http.StatusOK, CrawlFitResponse{
			URL:         req.URL,
			FitMarkdown: fitMarkdown,
			KeptBlocks:  kept,
			TotalBlocks: len(blocks),
		})
	}
}

// ---------------------------------------------------------------------------
// registerLLMFilterRoutes — wire endpoints onto mux
// ---------------------------------------------------------------------------

// registerLLMFilterRoutes registers the /llm-filter and /crawl-fit endpoints
// on mux using the supplied Deps. The integrator should call this from main
// after constructing a Deps with a valid LLMAdapter and PageRenderer:
//
//	registerLLMFilterRoutes(mux, deps)
func registerLLMFilterRoutes(mux *http.ServeMux, deps *Deps) {
	mux.HandleFunc("/llm-filter", llmFilterHandler(deps))
	mux.HandleFunc("/crawl-fit", crawlFitHandler(deps))
}
