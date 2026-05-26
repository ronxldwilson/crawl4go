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
	"github.com/ronxldwilson/crawl4go/internal/llm"
	"github.com/ronxldwilson/crawl4go/internal/proxy"
)

type Config struct {
	Port             string
	ZenPandaURL      string
	TorProxyURL      string
	ProxyURLs        []string // comma-separated list parsed from PROXY_URLS
	DefaultWaitMs    int
	MaxConcurrent    int
	RequestTimeoutMs int
	LLMProvider      string
	LLMModel         string
	LLMAPIKey        string
}

func loadConfig() Config {
	cfg := Config{
		Port:             getEnv("CRAWL4GO_PORT", "8082"),
		ZenPandaURL:      getEnv("ZENPANDA_URL", "http://zenpanda:9222"),
		TorProxyURL:      getEnv("TOR_PROXY_URL", "http://tor-proxy:3128"),
		DefaultWaitMs:    getEnvInt("DEFAULT_WAIT_MS", 1500),
		MaxConcurrent:    getEnvInt("MAX_CONCURRENT", 4),
		RequestTimeoutMs: getEnvInt("REQUEST_TIMEOUT_MS", 30000),
		LLMProvider:      getEnv("LLM_PROVIDER", "openai"),
		LLMModel:         getEnv("LLM_MODEL", "gpt-4o-mini"),
		LLMAPIKey:        getEnv("LLM_API_KEY", ""),
	}
	if v := os.Getenv("PROXY_URLS"); v != "" {
		for _, u := range strings.Split(v, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				cfg.ProxyURLs = append(cfg.ProxyURLs, u)
			}
		}
	}
	return cfg
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

type LLMExtractRequest struct {
	URL       string `json:"url"`
	Provider  string `json:"provider"`   // openai, anthropic, ollama
	Model     string `json:"model"`
	APIKey    string `json:"api_key"`
	Schema    string `json:"schema,omitempty"` // JSON schema for structured output
	ChunkSize int    `json:"chunk_size"`
	WaitMs    int    `json:"wait_ms"`
	Proxy     bool   `json:"proxy"`
}

type CosineExtractRequest struct {
	URL       string  `json:"url"`
	Provider  string  `json:"provider"` // openai for embeddings
	APIKey    string  `json:"api_key"`
	Model     string  `json:"model"` // embedding model
	Threshold float64 `json:"threshold"`
	TopN      int     `json:"top_n"`
	WaitMs    int     `json:"wait_ms"`
	Proxy     bool    `json:"proxy"`
}

type PDFExtractRequest struct {
	Path     string `json:"path"`      // local file path
	MaxPages int    `json:"max_pages"`
}

type BatchCrawlRequest struct {
	URLs          []string `json:"urls"`
	MaxConcurrent int      `json:"max_concurrent"`
	TimeoutMs     int      `json:"timeout_ms"`
	MaxRetries    int      `json:"max_retries"`
	Output        string   `json:"output"`
	Prune         bool     `json:"prune"`
	WaitMs        int      `json:"wait_ms"`
}

type FilterRequest struct {
	URL      string   `json:"url"`
	Query    string   `json:"query"`
	Filters  []string `json:"filters"` // "bm25", "pruning", "llm"
	Provider string   `json:"provider,omitempty"`
	APIKey   string   `json:"api_key,omitempty"`
	Model    string   `json:"model,omitempty"`
	WaitMs   int      `json:"wait_ms"`
	Proxy    bool     `json:"proxy"`
}

// --- LLM adapters ---

// llmAdapter adapts llm.Client to satisfy content.LLMProvider / content.LLMCompleter.
type llmAdapter struct {
	client *llm.Client
}

func (a *llmAdapter) Complete(ctx context.Context, prompt string) (string, error) {
	resp, err := a.client.Complete(ctx, llm.CompletionRequest{
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// embedAdapter adapts llm.Client to satisfy content.Embedder.
type embedAdapter struct {
	client *llm.Client
	model  string
}

func (a *embedAdapter) Embed(ctx context.Context, text string) ([]float64, error) {
	resp, err := a.client.Embed(ctx, llm.EmbeddingRequest{
		Input: []string{text},
		Model: a.model,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return resp.Embeddings[0], nil
}

func main() {
	cfg := loadConfig()

	cdpClient := browser.NewCDPClient(cfg.ZenPandaURL, cfg.MaxConcurrent)
	httpClient := &http.Client{Timeout: 90 * time.Second}
	pruner := content.NewPruningFilter()
	robots := crawl.NewRobotsChecker()
	rateLimiter := crawl.NewRateLimiter()

	// Build proxy pool from PROXY_URLS, or fall back to single TorProxyURL.
	var proxyPool *proxy.Pool
	if len(cfg.ProxyURLs) > 0 {
		proxyCfgs := make([]proxy.Config, len(cfg.ProxyURLs))
		for i, u := range cfg.ProxyURLs {
			proxyCfgs[i] = proxy.Config{URL: u}
		}
		proxyPool = proxy.NewPool(proxyCfgs)
	} else if cfg.TorProxyURL != "" {
		proxyPool = proxy.NewSinglePool(cfg.TorProxyURL)
	} else {
		proxyPool = proxy.NewPool(nil)
	}

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
	mux.HandleFunc("/extract-llm", llmExtractHandler(cfg, cdpClient, httpClient, proxyPool))
	mux.HandleFunc("/extract-cosine", cosineExtractHandler(cfg, cdpClient, httpClient, proxyPool))
	mux.HandleFunc("/pdf", pdfExtractHandler())
	mux.HandleFunc("/batch", batchCrawlHandler(cfg, cdpClient, httpClient, pruner, rateLimiter))
	mux.HandleFunc("/deep-crawl-stream", deepCrawlStreamHandler(cfg, cdpClient, httpClient, pruner, robots, rateLimiter))
	mux.HandleFunc("/filter", filterHandler(cfg, cdpClient, httpClient, pruner, proxyPool))

	slog.Info("crawl4go starting",
		"port", cfg.Port,
		"zenpanda", cfg.ZenPandaURL,
		"tor_proxy", cfg.TorProxyURL,
		"proxy_pool_size", proxyPool.Size(),
		"max_concurrent", cfg.MaxConcurrent,
		"llm_provider", cfg.LLMProvider,
		"llm_model", cfg.LLMModel,
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

// proxyURLFromPool returns the next proxy URL from the pool, or empty string if pool is empty.
func proxyURLFromPool(pool *proxy.Pool, useProxy bool) string {
	if !useProxy {
		return ""
	}
	cfg := pool.Next()
	return cfg.URL
}

func llmExtractHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, proxyPool *proxy.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
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
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.Provider == "" {
			req.Provider = cfg.LLMProvider
		}
		if req.Model == "" {
			req.Model = cfg.LLMModel
		}
		if req.APIKey == "" {
			req.APIKey = cfg.LLMAPIKey
		}
		if req.ChunkSize <= 0 {
			req.ChunkSize = 4000
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := proxyURLFromPool(proxyPool, req.Proxy)

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		llmClient := llm.NewClient(llm.Config{
			Provider:   req.Provider,
			Model:      req.Model,
			APIKey:     req.APIKey,
			MaxTokens:  4096,
			MaxRetries: 3,
			Timeout:    60 * time.Second,
		})

		adapter := &llmAdapter{client: llmClient}
		strategy := content.NewLLMExtractionStrategy(adapter)
		strategy.ChunkSize = req.ChunkSize
		strategy.Schema = req.Schema

		results, err := strategy.Extract(ctx, htmlContent, content.ExtractionConfig{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "llm extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":            req.URL,
			"results":        results,
			"input_tokens":   strategy.InputTokens,
			"output_tokens":  strategy.OutputTokens,
			"total_tokens":   strategy.InputTokens + strategy.OutputTokens,
		})
	}
}

func cosineExtractHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, proxyPool *proxy.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
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
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.Provider == "" {
			req.Provider = cfg.LLMProvider
		}
		if req.APIKey == "" {
			req.APIKey = cfg.LLMAPIKey
		}
		if req.Threshold <= 0 {
			req.Threshold = 0.6
		}
		if req.TopN <= 0 {
			req.TopN = 5
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := proxyURLFromPool(proxyPool, req.Proxy)

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		llmClient := llm.NewClient(llm.Config{
			Provider:   req.Provider,
			APIKey:     req.APIKey,
			MaxRetries: 3,
			Timeout:    60 * time.Second,
		})

		embedder := &embedAdapter{client: llmClient, model: req.Model}
		strategy := content.NewCosineStrategy(embedder)
		strategy.Threshold = req.Threshold
		strategy.TopN = req.TopN

		results, err := strategy.Extract(ctx, htmlContent, content.ExtractionConfig{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cosine extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":     req.URL,
			"results": results,
		})
	}
}

func pdfExtractHandler() http.HandlerFunc {
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
		if req.Path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}

		processor := content.NewPDFProcessor(content.PDFProcessorConfig{
			MaxPages: req.MaxPages,
		})

		text, err := processor.ExtractFromPath(req.Path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "pdf extraction failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"path":    req.Path,
			"content": text,
			"length":  len(text),
		})
	}
}

func batchCrawlHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter, rateLimiter *crawl.RateLimiter) http.HandlerFunc {
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
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "urls is required"})
			return
		}
		if req.MaxConcurrent <= 0 {
			req.MaxConcurrent = cfg.MaxConcurrent
		}
		if req.TimeoutMs <= 0 {
			req.TimeoutMs = cfg.RequestTimeoutMs
		}
		if req.WaitMs <= 0 {
			req.WaitMs = cfg.DefaultWaitMs
		}
		if req.Output == "" {
			req.Output = "markdown"
		}

		batchTimeout := 10 * time.Minute
		ctx, cancel := context.WithTimeout(r.Context(), batchTimeout)
		defer cancel()

		crawlFn := func(ctx context.Context, pageURL string) (*crawl.DeepCrawlResult, error) {
			if err := rateLimiter.Wait(ctx, pageURL); err != nil {
				return nil, err
			}

			crawlReq := CrawlRequest{
				URL:            pageURL,
				WaitMs:         req.WaitMs,
				MaxScrollSteps: 10,
				Output:         req.Output,
				Prune:          req.Prune,
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

		retryCfg := crawl.DefaultRetryConfig()
		if req.MaxRetries > 0 {
			retryCfg.MaxRetries = req.MaxRetries
		}

		batchCfg := crawl.BatchConfig{
			MaxConcurrent: req.MaxConcurrent,
			Timeout:       time.Duration(req.TimeoutMs) * time.Millisecond,
			RetryConfig:   retryCfg,
		}

		result := crawl.CrawlMany(ctx, req.URLs, crawlFn, batchCfg)

		// Convert error map to string map for JSON serialisation.
		errMap := make(map[string]string, len(result.Errors))
		for u, e := range result.Errors {
			errMap[u] = e.Error()
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"results": result.Results,
			"errors":  errMap,
			"stats":   result.Stats,
		})
	}
}

func deepCrawlStreamHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter, robots *crawl.RobotsChecker, rateLimiter *crawl.RateLimiter) http.HandlerFunc {
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

		// Set SSE headers before any writing.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
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

		streamFn := func(result crawl.DeepCrawlResult) bool {
			data, err := json.Marshal(result)
			if err != nil {
				return true // skip bad result, keep going
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return true
		}

		stats, err := crawl.StreamBFS(ctx, req.URL, crawlFn, opts, streamFn)

		// Send a final stats event.
		statsData, _ := json.Marshal(map[string]any{"stats": stats, "error": func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}()})
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", statsData)
		flusher.Flush()
	}
}

func filterHandler(cfg Config, cdpClient *browser.CDPClient, httpClient *http.Client, pruner *content.PruningFilter, proxyPool *proxy.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}

		var req FilterRequest
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
		if req.Provider == "" {
			req.Provider = cfg.LLMProvider
		}
		if req.Model == "" {
			req.Model = cfg.LLMModel
		}
		if req.APIKey == "" {
			req.APIKey = cfg.LLMAPIKey
		}
		if len(req.Filters) == 0 {
			req.Filters = []string{"bm25"}
		}

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		proxyURL := proxyURLFromPool(proxyPool, req.Proxy)

		htmlContent, _, _, err := browser.RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, false, 0, proxyURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "render failed: " + err.Error()})
			return
		}

		if req.Proxy {
			if pruned, err := pruner.Filter(htmlContent); err == nil && len(pruned) > 0 {
				htmlContent = pruned
			}
		}

		blocks := content.ExtractTextChunks(htmlContent)
		blockTexts := make([]string, len(blocks))
		for i, b := range blocks {
			blockTexts[i] = b.Text
		}

		var contentFilters []content.ContentFilter
		for _, name := range req.Filters {
			switch name {
			case "bm25":
				contentFilters = append(contentFilters, content.NewBM25ContentFilter())
			case "pruning":
				contentFilters = append(contentFilters, content.NewPruningContentFilter())
			case "llm":
				llmClient := llm.NewClient(llm.Config{
					Provider:   req.Provider,
					Model:      req.Model,
					APIKey:     req.APIKey,
					MaxTokens:  4096,
					MaxRetries: 3,
					Timeout:    60 * time.Second,
				})
				adapter := &llmAdapter{client: llmClient}
				contentFilters = append(contentFilters, content.NewLLMContentFilter(adapter))
			}
		}

		pipeline := content.NewFilterPipeline(contentFilters...)
		filtered, err := pipeline.Run(ctx, blockTexts, req.Query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "filter failed: " + err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"url":           req.URL,
			"query":         req.Query,
			"total_blocks":  len(blockTexts),
			"filters_used":  req.Filters,
			"results":       filtered,
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
