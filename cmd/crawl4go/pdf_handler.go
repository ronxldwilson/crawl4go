package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

// maxPDFBytes caps how much of a fetched PDF we read into memory.
const maxPDFBytes = 50 << 20 // 50 MiB

// PDFExtractRequest is the JSON body for POST /extract-pdf.
type PDFExtractRequest struct {
	URL      string `json:"url"`
	Proxy    bool   `json:"proxy"`
	MaxPages int    `json:"max_pages"`
}

// PDFExtractResponse is the JSON body returned by POST /extract-pdf.
type PDFExtractResponse struct {
	URL   string `json:"url"`
	Text  string `json:"text"`
	Chars int    `json:"chars"`
}

// registerPDFRoutes wires the /extract-pdf endpoint onto mux.
func registerPDFRoutes(mux *http.ServeMux, deps *Deps) {
	mux.HandleFunc("/extract-pdf", extractPDFHandler(deps))
}

// extractPDFHandler fetches a PDF by URL and extracts its text content.
func extractPDFHandler(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req PDFExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(deps.Cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		client, err := pdfFetchClient(deps, req.Proxy)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proxy config: " + err.Error()})
			return
		}

		data, err := fetchPDFBytes(ctx, client, req.URL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "fetch failed: " + err.Error()})
			return
		}

		processor := content.NewPDFProcessor(content.PDFProcessorConfig{MaxPages: req.MaxPages})
		text, err := processor.ExtractFromBytes(data)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "pdf extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, PDFExtractResponse{
			URL:   req.URL,
			Text:  text,
			Chars: len(text),
		})
	}
}

// pdfFetchClient returns the HTTP client to fetch the PDF with. When proxy is
// requested it returns a client routed through the configured Tor proxy;
// otherwise it reuses deps.HTTP.
func pdfFetchClient(deps *Deps, proxy bool) (*http.Client, error) {
	if !proxy || deps.Cfg.TorProxyURL == "" {
		return deps.HTTP, nil
	}
	pu, err := url.Parse(deps.Cfg.TorProxyURL)
	if err != nil {
		return nil, err
	}
	timeout := 90 * time.Second
	if deps.HTTP != nil && deps.HTTP.Timeout > 0 {
		timeout = deps.HTTP.Timeout
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(pu)},
	}, nil
}

// fetchPDFBytes GETs url and returns up to maxPDFBytes of the body.
func fetchPDFBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxPDFBytes))
}
