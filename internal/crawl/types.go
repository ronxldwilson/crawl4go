package crawl

import (
	"context"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

type CrawlFunc func(ctx context.Context, pageURL string) (*DeepCrawlResult, error)

type CrawlStrategy interface {
	Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats)
}

type CrawlOptions struct {
	MaxDepth        int
	MaxPages        int
	IncludeExternal bool
	Filters         *FilterChain
	Scorer          URLScorer
	ScoreThreshold  float64
	Robots          *RobotsChecker
	InitialState    *CrawlState
}

type DeepCrawlResult struct {
	URL          string          `json:"url"`
	Depth        int             `json:"depth"`
	ParentURL    string          `json:"parent_url,omitempty"`
	StatusCode   int             `json:"status_code"`
	Blocked      bool            `json:"blocked"`
	Content      string          `json:"content"`
	Links        content.LinkSet `json:"links"`
	Score        float64         `json:"score,omitempty"`
	RenderTimeMs int64           `json:"render_time_ms"`
}

type CrawlStats struct {
	PagesCrawled    int   `json:"pages_crawled"`
	PagesBlocked    int   `json:"pages_blocked"`
	MaxDepthReached int   `json:"max_depth_reached"`
	TotalTimeMs     int64 `json:"total_time_ms"`
}

type FilterConfig struct {
	URLPatterns       []string `json:"url_patterns,omitempty"`
	BlockedDomains    []string `json:"blocked_domains,omitempty"`
	AllowedDomains    []string `json:"allowed_domains,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
}

type ScorerConfig struct {
	Keywords        []string `json:"keywords,omitempty"`
	KeywordWeight   float64  `json:"keyword_weight"`
	FreshnessWeight float64  `json:"freshness_weight"`
	DepthWeight     float64  `json:"depth_weight"`
}
