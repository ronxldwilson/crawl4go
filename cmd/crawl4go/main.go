package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/browser"
	"github.com/ronxldwilson/crawl4go/internal/content"
	"github.com/ronxldwilson/crawl4go/internal/crawl"
)

type Config struct {
	Port             string
	ZenPandaURL      string
	TorProxyURL      string
	DefaultWaitMs    int
	MaxConcurrent    int
	RequestTimeoutMs int
}

func loadConfig() Config {
	return Config{
		Port:             getEnv("CRAWL4GO_PORT", "8082"),
		ZenPandaURL:      getEnv("ZENPANDA_URL", "http://zenpanda:9222"),
		TorProxyURL:      getEnv("TOR_PROXY_URL", "http://tor-proxy:3128"),
		DefaultWaitMs:    getEnvInt("DEFAULT_WAIT_MS", 1500),
		MaxConcurrent:    getEnvInt("MAX_CONCURRENT", 4),
		RequestTimeoutMs: getEnvInt("REQUEST_TIMEOUT_MS", 30000),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// --- Request / Response types ---

type CrawlRequest struct {
	URL            string `json:"url"`
	WaitMs         int    `json:"wait_ms"`
	Scroll         bool   `json:"scroll"`
	MaxScrollSteps int    `json:"max_scroll_steps"`
	Output         string `json:"output"`
	Prune          bool   `json:"prune"`
	Proxy          bool   `json:"proxy"`
	ExtractMeta    bool   `json:"extract_meta"`
	ExtractTables  bool   `json:"extract_tables"`
	ExtractMedia   bool   `json:"extract_media"`
}

type CrawlResponse struct {
	URL          string                   `json:"url"`
	StatusCode   int                      `json:"status_code"`
	Blocked      bool                     `json:"blocked"`
	BlockReason  string                   `json:"block_reason,omitempty"`
	Content      string                   `json:"content"`
	Links        content.LinkSet          `json:"links"`
	Metadata     *content.PageMetadata    `json:"metadata,omitempty"`
	Tables       []content.ExtractedTable `json:"tables,omitempty"`
	Media        *content.MediaSet        `json:"media,omitempty"`
	Readability  *content.ReadabilityScore `json:"readability,omitempty"`
	ContentHash  string                   `json:"content_hash,omitempty"`
	RenderTimeMs int64                    `json:"render_time_ms"`
	RenderSource string                   `json:"render_source"`
}

type DeepCrawlRequest struct {
	URL             string              `json:"url"`
	Strategy        string              `json:"strategy"`
	MaxDepth        int                 `json:"max_depth"`
	MaxPages        int                 `json:"max_pages"`
	IncludeExternal bool                `json:"include_external"`
	Filters         *crawl.FilterConfig `json:"filters,omitempty"`
	Scorer          *crawl.ScorerConfig `json:"scorer,omitempty"`
	ScoreThreshold  float64             `json:"score_threshold"`
	Output          string              `json:"output"`
	Prune           bool                `json:"prune"`
	Scroll          bool                `json:"scroll"`
	WaitMs          int                 `json:"wait_ms"`
	QueryTerms      []string            `json:"query_terms,omitempty"`
}

type DeepCrawlResponse struct {
	Results []crawl.DeepCrawlResult `json:"results"`
	Stats   crawl.CrawlStats        `json:"stats"`
}

type ExtractRequest struct {
	URL    string                  `json:"url"`
	Schema content.ExtractionSchema `json:"schema"`
	WaitMs int                     `json:"wait_ms"`
	Proxy  bool                    `json:"proxy"`
}

type LinkPreviewRequest struct {
	URLs           []string `json:"urls"`
	MaxConcurrent  int      `json:"max_concurrent"`
}

type SitemapRequest struct {
	URL     string `json:"url"`
	MaxURLs int    `json:"max_urls"`
}

type ChunkRequest struct {
	URL      string `json:"url"`
	Strategy string `json:"strategy"`
	ChunkSize int   `json:"chunk_size"`
	Overlap   int   `json:"overlap"`
	WaitMs    int   `json:"wait_ms"`
	Prune     bool  `json:"prune"`
	Proxy     bool  `json:"proxy"`
}

type ScreenshotRequest struct {
	URL      string `json:"url"`
	WaitMs   int    `json:"wait_ms"`
	FullPage bool   `json:"full_page"`
}

type XPathExtractRequest struct {
	URL    string                      `json:"url"`
	Schema content.XPathExtractionSchema `json:"schema"`
	WaitMs int                         `json:"wait_ms"`
	Proxy  bool                        `json:"proxy"`
}

type RegexExtractRequest struct {
	URL    string                      `json:"url"`
	Schema content.RegexExtractionSchema `json:"schema"`
	WaitMs int                         `json:"wait_ms"`
	Proxy  bool                        `json:"proxy"`
}

type JSExecuteRequest struct {
	URL          string `json:"url"`
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise"`
	WaitMs       int    `json:"wait_ms"`
}

type DiffRequest struct {
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

type RobotsRequest struct {
	URL       string `json:"url"`
	UserAgent string `json:"user_agent"`
}

type RobotsResponse struct {
	URL     string `json:"url"`
	Allowed bool   `json:"allowed"`
}

type BM25Request struct {
	URL       string  `json:"url"`
	Query     string  `json:"query"`
	Threshold float64 `json:"threshold"`
	WaitMs    int     `json:"wait_ms"`
	Prune     bool    `json:"prune"`
	Proxy     bool    `json:"proxy"`
}

func main() {
	cfg := loadConfig()

	cdpClient := browser.NewCDPClient(cfg.ZenPandaURL, cfg.MaxConcurrent)
	httpClient := &http.Client{Timeout: 90 * time.Second}
	pruner := content.NewPruningFilter()
	robots := crawl.NewRobotsChecker()
	rateLimiter := crawl.NewRateLimiter()

	mux := http.NewServeMux()

	mux.HandleFunc("/crawl", crawlHandler(cfg, cdpClient, httpClient, pruner))
	mux.HandleFunc("/deep-crawl", deepCrawlHandler(cfg, cdpClient, httpClient, pruner, robots, rateLimiter))
	mux.HandleFunc("/extract", extractHandler(cfg, cdpClient, httpClient))
	mux.HandleFunc("/link-preview", linkPreviewHandler(httpClient))
	mux.HandleFunc("/sitemap", sitemapHandler(httpClient))
	mux.HandleFunc("/cert/", certHandler())
	mux.HandleFunc("/screenshot", screenshotHandler(cfg, cdpClient))
	mux.HandleFunc("/chunk", chunkHandler(cfg, cdpClient, httpClient, pruner))
	mux.HandleFunc("/bm25", bm25Handler(cfg, cdpClient, httpClient, pruner))
	mux.HandleFunc("/extract-xpath", xpathExtractHandler(cfg, cdpClient, httpClient))
	mux.HandleFunc("/extract-regex", regexExtractHandler(cfg, cdpClient, httpClient))
	mux.HandleFunc("/execute", jsExecuteHandler(cfg, cdpClient))
	mux.HandleFunc("/diff", diffHandler())
	mux.HandleFunc("/cdx", cdxHandler(httpClient))
	mux.HandleFunc("/robots", robotsHandler(robots))
	mux.HandleFunc("/analyze", analyzeHandler(cfg, cdpClient, httpClient, pruner))
	mux.HandleFunc("/perf", perfHandler(cfg, cdpClient))
	mux.HandleFunc("/health", healthHandler(cdpClient))

	slog.Info("crawl4go starting",
		"port", cfg.Port,
		"zenpanda", cfg.ZenPandaURL,
		"tor_proxy", cfg.TorProxyURL,
		"max_concurrent", cfg.MaxConcurrent,
	)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      loggingMiddleware(corsMiddleware(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 15 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- Handlers ---

func crawlHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req CrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.MaxScrollSteps <= 0 {
			req.MaxScrollSteps = 10
		}
		if req.Output == "" {
			req.Output = "markdown"
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		resp := crawlSinglePage(ctx, cfg, cdpClient, httpClient, pruner, req)
		writeJSON(w, http.StatusOK, resp)
	}
}

func crawlSinglePage(ctx context.Context, cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter, req CrawlRequest) CrawlResponse {
	start := time.Now()

	proxyURL := ""
	if req.Proxy {
		proxyURL = cfg.TorProxyURL
	}

	htmlContent, statusCode, source, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, req.Scroll, req.MaxScrollSteps, proxyURL)
	if err != nil {
		return CrawlResponse{
			URL:          req.URL,
			StatusCode:   statusCode,
			Content:      "",
			RenderTimeMs: time.Since(start).Milliseconds(),
			RenderSource: source,
		}
	}

	blocked, reason := content.IsBlocked(statusCode, htmlContent)
	links := content.ExtractLinks(htmlContent, req.URL)

	var metadata *content.PageMetadata
	if req.ExtractMeta {
		metadata = content.ExtractMetadata(htmlContent)
	}

	var tables []content.ExtractedTable
	if req.ExtractTables {
		tables = content.ExtractTables(htmlContent)
	}

	var media *content.MediaSet
	if req.ExtractMedia {
		ms := content.ExtractMedia(htmlContent, req.URL)
		media = &ms
	}

	pageContent := htmlContent
	if req.Prune {
		if pruned, err := pruner.Filter(htmlContent); err == nil && len(pruned) > 0 {
			pageContent = pruned
		}
	}

	switch req.Output {
	case "text":
		pageContent = content.HTMLToText(pageContent)
	case "markdown":
		pageContent = content.HTMLToMarkdown(pageContent, req.URL)
	}

	plainText := content.HTMLToText(htmlContent)
	readability := content.ScoreReadability(plainText)
	hash := content.ContentHash(pageContent)

	return CrawlResponse{
		URL:          req.URL,
		StatusCode:   statusCode,
		Blocked:      blocked,
		BlockReason:  reason,
		Content:      pageContent,
		Links:        links,
		Metadata:     metadata,
		Tables:       tables,
		Media:        media,
		Readability:  &readability,
		ContentHash:  hash,
		RenderTimeMs: time.Since(start).Milliseconds(),
		RenderSource: source,
	}
}

func deepCrawlHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter, robots *crawl.RobotsChecker, rateLimiter *crawl.RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req DeepCrawlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.Strategy == "" {
			req.Strategy = "bfs"
		}
		if req.MaxDepth <= 0 {
			req.MaxDepth = 3
		}
		if req.MaxPages <= 0 {
			req.MaxPages = 100
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.Output == "" {
			req.Output = "markdown"
		}

		deepTimeout := 10 * time.Minute
		ctx, cancel := context.WithTimeout(r.Context(), deepTimeout)
		defer cancel()

		var filters *crawl.FilterChain
		if req.Filters != nil {
			filters = crawl.BuildFilterChain(req.Filters)
		}

		var scorer crawl.URLScorer
		if req.Scorer != nil {
			scorer = crawl.BuildScorer(req.Scorer)
		}

		crawlFn := func(ctx context.Context, pageURL string) (*crawl.DeepCrawlResult, error) {
			if err := rateLimiter.Wait(ctx, pageURL); err != nil {
				return nil, err
			}

			crawlReq := CrawlRequest{
				URL:            pageURL,
				WaitMs:         req.WaitMs,
				Scroll:         req.Scroll,
				MaxScrollSteps: 10,
				Output:         req.Output,
				Prune:          req.Prune,
				Proxy:          true,
			}
			resp := crawlSinglePage(ctx, cfg, cdpClient, httpClient, pruner, crawlReq)

			rateLimiter.RecordResult(pageURL, resp.StatusCode)

			return &crawl.DeepCrawlResult{
				URL:          resp.URL,
				StatusCode:   resp.StatusCode,
				Blocked:      resp.Blocked,
				Content:      resp.Content,
				Links:        resp.Links,
				RenderTimeMs: resp.RenderTimeMs,
			}, nil
		}

		opts := crawl.CrawlOptions{
			MaxDepth:        req.MaxDepth,
			MaxPages:        req.MaxPages,
			IncludeExternal: req.IncludeExternal,
			Filters:         filters,
			Scorer:          scorer,
			ScoreThreshold:  req.ScoreThreshold,
			Robots:          robots,
		}

		var strategy crawl.CrawlStrategy
		switch req.Strategy {
		case "dfs":
			strategy = &crawl.DFSStrategy{}
		case "best-first":
			strategy = &crawl.BestFirstStrategy{}
		case "adaptive":
			strategy = crawl.NewAdaptiveStrategy(req.QueryTerms)
		default:
			strategy = &crawl.BFSStrategy{}
		}

		results, stats := strategy.Run(ctx, req.URL, crawlFn, opts)

		slog.Info("deep-crawl completed",
			"url", req.URL,
			"strategy", req.Strategy,
			"pages_crawled", stats.PagesCrawled,
			"pages_blocked", stats.PagesBlocked,
			"max_depth_reached", stats.MaxDepthReached,
			"elapsed_ms", stats.TotalTimeMs,
		)

		writeJSON(w, http.StatusOK, DeepCrawlResponse{
			Results: results,
			Stats:   stats,
		})
	}
}

func extractHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req ExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		extractor := content.NewCSSExtractor(req.Schema)
		results, err := extractor.Extract(htmlContent)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":     req.URL,
			"results": results,
		})
	}
}

func linkPreviewHandler(httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req LinkPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if len(req.URLs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "urls is required"})
			return
		}
		if req.MaxConcurrent <= 0 {
			req.MaxConcurrent = 5
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		previews := content.FetchLinkPreviews(ctx, req.URLs, httpClient, req.MaxConcurrent)
		writeJSON(w, http.StatusOK, map[string]any{"previews": previews})
	}
}

func sitemapHandler(httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req SitemapRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.MaxURLs <= 0 {
			req.MaxURLs = 1000
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		seeder := crawl.NewSitemapSeeder(httpClient, req.MaxURLs)
		urls, err := seeder.Discover(ctx, req.URL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sitemap discovery failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":       req.URL,
			"urls":      urls,
			"url_count": len(urls),
		})
	}
}

func certHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := strings.TrimPrefix(r.URL.Path, "/cert/")
		if host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host is required (GET /cert/{host})"})
			return
		}

		certInfo, err := content.InspectCert(host)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cert inspection failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, certInfo)
	}
}

func screenshotHandler(cfg Config, cdpClient *browser.CDPClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req ScreenshotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		data, err := cdpClient.CaptureScreenshot(ctx, req.URL, req.WaitMs, req.FullPage)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "screenshot failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":  req.URL,
			"data": data,
		})
	}
}

func chunkHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req ChunkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.ChunkSize <= 0 {
			req.ChunkSize = 4000
		}
		if req.Strategy == "" {
			req.Strategy = "fixed"
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		pageContent := htmlContent
		if req.Prune {
			if pruned, err := pruner.Filter(htmlContent); err == nil && len(pruned) > 0 {
				pageContent = pruned
			}
		}
		pageContent = content.HTMLToMarkdown(pageContent, req.URL)

		var chunker content.ChunkStrategy
		switch req.Strategy {
		case "sliding":
			chunker = content.NewSlidingWindowChunker(req.ChunkSize, req.Overlap)
		case "semantic":
			chunker = content.NewSemanticChunker(req.ChunkSize)
		case "markdown":
			chunker = content.NewMarkdownChunker(req.ChunkSize)
		default:
			chunker = content.NewFixedSizeChunker(req.ChunkSize, req.Overlap)
		}

		chunks := chunker.Chunk(pageContent)
		writeJSON(w, http.StatusOK, map[string]any{
			"url":         req.URL,
			"strategy":    req.Strategy,
			"chunk_count": len(chunks),
			"chunks":      chunks,
		})
	}
}

func bm25Handler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req BM25Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.Threshold <= 0 {
			req.Threshold = 1.0
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		if req.Prune {
			if pruned, err := pruner.Filter(htmlContent); err == nil && len(pruned) > 0 {
				htmlContent = pruned
			}
		}

		chunks := content.ExtractTextChunks(htmlContent)

		query := req.Query
		if query == "" {
			query = content.ExtractPageQuery(htmlContent)
		}

		bm25 := content.NewBM25Filter()
		bm25.Threshold = req.Threshold
		relevant := bm25.FilterByRelevance(chunks, query)

		writeJSON(w, http.StatusOK, map[string]any{
			"url":          req.URL,
			"query":        query,
			"total_chunks": len(chunks),
			"relevant":     len(relevant),
			"chunks":       relevant,
		})
	}
}

func robotsHandler(robots *crawl.RobotsChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req RobotsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.UserAgent == "" {
			req.UserAgent = "*"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		allowed := robots.CanFetch(ctx, req.UserAgent, req.URL)
		writeJSON(w, http.StatusOK, RobotsResponse{
			URL:     req.URL,
			Allowed: allowed,
		})
	}
}

func healthHandler(cdpClient *browser.CDPClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		status := map[string]any{
			"status":   "ok",
			"zenpanda": cdpClient.Healthy(ctx),
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func xpathExtractHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req XPathExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		extractor := content.NewXPathExtractor(req.Schema)
		results, err := extractor.Extract(htmlContent)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "xpath extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":     req.URL,
			"results": results,
		})
	}
}

func regexExtractHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req RegexExtractRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		extractor := content.NewRegexExtractor(req.Schema)
		results, err := extractor.Extract(htmlContent)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "regex extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":     req.URL,
			"results": results,
		})
	}
}

func jsExecuteHandler(cfg Config, cdpClient *browser.CDPClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req JSExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" || req.Expression == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url and expression are required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		result, err := cdpClient.ExecuteJS(ctx, req.URL, req.WaitMs, req.Expression, req.AwaitPromise)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "js execution failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":    req.URL,
			"result": result,
		})
	}
}

func diffHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req DiffRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		result := content.DiffText(req.OldText, req.NewText)
		writeJSON(w, http.StatusOK, map[string]any{
			"diff":     result,
			"old_hash": content.ContentHash(req.OldText),
			"new_hash": content.ContentHash(req.NewText),
		})
	}
}

func cdxHandler(httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			Domain  string `json:"domain"`
			MaxURLs int    `json:"max_urls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Domain == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
			return
		}
		if req.MaxURLs <= 0 {
			req.MaxURLs = 500
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		seeder := crawl.NewCDXSeeder(httpClient, req.MaxURLs)
		records, err := seeder.Discover(ctx, req.Domain)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cdx discovery failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"domain":    req.Domain,
			"records":   records,
			"url_count": len(records),
		})
	}
}

// --- Logging Middleware ---

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"req_bytes", r.ContentLength,
			"resp_bytes", rw.bytes,
			"remote", r.RemoteAddr,
		)
	})
}

func analyzeHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			URL    string `json:"url"`
			WaitMs int    `json:"wait_ms"`
			Proxy  bool   `json:"proxy"`
			TopN   int    `json:"top_n"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.TopN <= 0 {
			req.TopN = 20
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := ""
		if req.Proxy {
			proxyURL = cfg.TorProxyURL
		}

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		plainText := content.HTMLToText(htmlContent)
		structure := content.AnalyzeStructure(htmlContent)
		stats := content.AnalyzeContent(plainText, req.TopN)
		readability := content.ScoreReadability(plainText)
		lang := content.DetectLanguage(plainText)

		writeJSON(w, http.StatusOK, map[string]any{
			"url":         req.URL,
			"structure":   structure,
			"stats":       stats,
			"readability": readability,
			"language":    lang,
		})
	}
}

func perfHandler(cfg Config, cdpClient *browser.CDPClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req struct {
			URL    string `json:"url"`
			WaitMs int    `json:"wait_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
			return
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		metrics, err := cdpClient.CollectMetrics(ctx, req.URL, req.WaitMs)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "metrics collection failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":     req.URL,
			"metrics": metrics,
		})
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func init() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	fmt.Println(`
                         _  _
  ___ _ __ __ ___      _| || |   __ _  ___
 / __| '__/ _' \ \ /\ / / || |_ / _' |/ _ \
| (__| | | (_| |\ V  V /|__   _| (_| | (_) |
 \___|_|  \__,_| \_/\_/    |_|  \__, |\___/
                                 |___/
	`)
}
