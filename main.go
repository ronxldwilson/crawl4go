package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
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
	Output         string `json:"output"` // "markdown", "text", "html"
	Prune          bool   `json:"prune"`
	Proxy          bool   `json:"proxy"`
}

type CrawlResponse struct {
	URL          string  `json:"url"`
	StatusCode   int     `json:"status_code"`
	Blocked      bool    `json:"blocked"`
	BlockReason  string  `json:"block_reason,omitempty"`
	Content      string  `json:"content"`
	Links        LinkSet `json:"links"`
	RenderTimeMs int64   `json:"render_time_ms"`
	RenderSource string  `json:"render_source"`
}

type DeepCrawlRequest struct {
	URL             string             `json:"url"`
	Strategy        string             `json:"strategy"` // "bfs", "dfs", "best-first"
	MaxDepth        int                `json:"max_depth"`
	MaxPages        int                `json:"max_pages"`
	IncludeExternal bool               `json:"include_external"`
	Filters         *FilterConfig      `json:"filters,omitempty"`
	Scorer          *ScorerConfig      `json:"scorer,omitempty"`
	ScoreThreshold  float64            `json:"score_threshold"`
	Output          string             `json:"output"`
	Prune           bool               `json:"prune"`
	Scroll          bool               `json:"scroll"`
	WaitMs          int                `json:"wait_ms"`
}

type FilterConfig struct {
	URLPatterns      []string `json:"url_patterns,omitempty"`
	BlockedDomains   []string `json:"blocked_domains,omitempty"`
	AllowedDomains   []string `json:"allowed_domains,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
}

type ScorerConfig struct {
	Keywords       []string `json:"keywords,omitempty"`
	KeywordWeight  float64  `json:"keyword_weight"`
	FreshnessWeight float64 `json:"freshness_weight"`
	DepthWeight    float64  `json:"depth_weight"`
}

type DeepCrawlResponse struct {
	Results []DeepCrawlResult `json:"results"`
	Stats   CrawlStats        `json:"stats"`
}

func main() {
	cfg := loadConfig()

	cdpClient := NewCDPClient(cfg.ZenPandaURL, cfg.MaxConcurrent)
	httpClient := &http.Client{Timeout: 90 * time.Second}
	pruner := NewPruningFilter()

	mux := http.NewServeMux()
	mux.HandleFunc("/crawl", crawlHandler(cfg, cdpClient, httpClient, pruner))
	mux.HandleFunc("/deep-crawl", deepCrawlHandler(cfg, cdpClient, httpClient, pruner))
	mux.HandleFunc("/health", healthHandler)

	slog.Info("crawl4go starting",
		"port", cfg.Port,
		"zenpanda", cfg.ZenPandaURL,
		"tor_proxy", cfg.TorProxyURL,
		"max_concurrent", cfg.MaxConcurrent,
	)

	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func crawlHandler(cfg Config, cdpClient *CDPClient, httpClient *http.Client, pruner *PruningFilter) http.HandlerFunc {
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

func crawlSinglePage(ctx context.Context, cfg Config, cdpClient *CDPClient, httpClient *http.Client, pruner *PruningFilter, req CrawlRequest) CrawlResponse {
	start := time.Now()

	proxyURL := ""
	if req.Proxy {
		proxyURL = cfg.TorProxyURL
	}

	htmlContent, statusCode, source, err := RenderPage(ctx, cdpClient, httpClient, req.URL, req.WaitMs, req.Scroll, req.MaxScrollSteps, proxyURL)
	if err != nil {
		return CrawlResponse{
			URL:          req.URL,
			StatusCode:   statusCode,
			Content:      "",
			RenderTimeMs: time.Since(start).Milliseconds(),
			RenderSource: source,
		}
	}

	blocked, reason := IsBlocked(statusCode, htmlContent)

	links := ExtractLinks(htmlContent, req.URL)

	content := htmlContent
	if req.Prune {
		if pruned, err := pruner.Filter(htmlContent); err == nil && len(pruned) > 0 {
			content = pruned
		}
	}

	switch req.Output {
	case "text":
		content = HTMLToText(content)
	case "markdown":
		content = HTMLToMarkdown(content, req.URL)
	}

	return CrawlResponse{
		URL:          req.URL,
		StatusCode:   statusCode,
		Blocked:      blocked,
		BlockReason:  reason,
		Content:      content,
		Links:        links,
		RenderTimeMs: time.Since(start).Milliseconds(),
		RenderSource: source,
	}
}

func deepCrawlHandler(cfg Config, cdpClient *CDPClient, httpClient *http.Client, pruner *PruningFilter) http.HandlerFunc {
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

		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.RequestTimeoutMs)*time.Millisecond)
		defer cancel()

		var filters *FilterChain
		if req.Filters != nil {
			filters = BuildFilterChain(req.Filters)
		}

		var scorer URLScorer
		if req.Scorer != nil {
			scorer = BuildScorer(req.Scorer)
		}

		crawlFn := func(ctx context.Context, pageURL string) (*DeepCrawlResult, error) {
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
			return &DeepCrawlResult{
				URL:          resp.URL,
				StatusCode:   resp.StatusCode,
				Blocked:      resp.Blocked,
				Content:      resp.Content,
				Links:        resp.Links,
				RenderTimeMs: resp.RenderTimeMs,
			}, nil
		}

		opts := CrawlOptions{
			MaxDepth:        req.MaxDepth,
			MaxPages:        req.MaxPages,
			IncludeExternal: req.IncludeExternal,
			Filters:         filters,
			Scorer:          scorer,
			ScoreThreshold:  req.ScoreThreshold,
		}

		var strategy CrawlStrategy
		switch req.Strategy {
		case "dfs":
			strategy = &DFSStrategy{}
		case "best-first":
			strategy = &BestFirstStrategy{}
		default:
			strategy = &BFSStrategy{}
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

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
