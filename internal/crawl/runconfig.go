package crawl

import "time"

// RunConfig holds all per-crawl options, bridging the gap between the HTTP API's
// CrawlRequest and the internal browser/content extraction settings.
type RunConfig struct {
	// Target
	URL string `json:"url"`

	// Browser behavior
	WaitMs          int    `json:"wait_ms"`
	Scroll          bool   `json:"scroll"`
	MaxScrollSteps  int    `json:"max_scroll_steps"`
	WaitForSelector string `json:"wait_for_selector,omitempty"`

	// Content output
	Output        string `json:"output"` // html, text, markdown
	Prune         bool   `json:"prune"`
	ExtractMeta   bool   `json:"extract_meta"`
	ExtractMedia  bool   `json:"extract_media"`
	ExtractTables bool   `json:"extract_tables"`
	ExtractLinks  bool   `json:"extract_links"`

	// Network
	Proxy     string            `json:"proxy,omitempty"`
	Timeout   time.Duration     `json:"timeout"`
	UserAgent string            `json:"user_agent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`

	// Cache control
	CacheMode CacheMode `json:"cache_mode"`

	// JS execution
	JSCode       string `json:"js_code,omitempty"`
	AwaitPromise bool   `json:"await_promise"`

	// Anti-bot
	MaxRetries  int  `json:"max_retries"`
	RotateProxy bool `json:"rotate_proxy"`

	// Hook data
	SharedData map[string]any `json:"shared_data,omitempty"` // #123 inter-hook state bag

	// Deep crawl (when used with deep-crawl strategies)
	MaxDepth        int     `json:"max_depth"`
	MaxPages        int     `json:"max_pages"`
	IncludeExternal bool    `json:"include_external"`
	ScoreThreshold  float64 `json:"score_threshold"`
}

// CacheMode controls how caching behaves for a request.
type CacheMode int

const (
	CacheModeDefault  CacheMode = iota // Use cache if available
	CacheModeBypass                    // Skip cache, fetch fresh
	CacheModeOnly                     // Only return cached, fail if miss
	CacheModeRefresh                  // Fetch fresh and update cache
	CacheModeDisabled                 // No cache read or write
)

// DefaultRunConfig returns a RunConfig with sensible defaults.
func DefaultRunConfig(url string) RunConfig {
	return RunConfig{
		URL:            url,
		WaitMs:         3000,
		Scroll:         true,
		MaxScrollSteps: 10,
		Output:         "text",
		ExtractLinks:   true,
		Timeout:        30 * time.Second,
		CacheMode:      CacheModeDefault,
		MaxRetries:     2,
		MaxDepth:       3,
		MaxPages:       100,
	}
}
