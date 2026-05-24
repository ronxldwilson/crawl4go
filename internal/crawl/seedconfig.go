package crawl

// SeedingConfigV2 is a comprehensive configuration struct for URL discovery,
// ported from Python Crawl4AI's SeedingConfig. It controls how seed URLs are
// discovered across sitemaps, CDX archives, and robots.txt directives.
type SeedingConfigV2 struct {
	// Sitemap discovery
	SitemapEnabled bool     `json:"sitemap_enabled"`
	SitemapURLs    []string `json:"sitemap_urls,omitempty"` // explicit sitemap URLs to fetch

	// CDX (Common Crawl / Wayback Machine) discovery
	CDXEnabled bool     `json:"cdx_enabled"`
	CDXURLs    []string `json:"cdx_urls,omitempty"` // explicit CDX API endpoints

	// Robots.txt
	RobotsTxtEnabled bool `json:"robots_txt_enabled"`
	RespectRobotsTxt bool `json:"respect_robots_txt"` // honour Disallow rules during crawl

	// Crawl bounds
	MaxDepth int `json:"max_depth"`
	MaxPages int `json:"max_pages"`

	// Scope filtering
	AllowedDomains   []string `json:"allowed_domains,omitempty"`
	ExcludedPatterns []string `json:"excluded_patterns,omitempty"` // regex or glob patterns to reject

	// HTTP behavior
	FollowRedirects bool `json:"follow_redirects"`
}

// DefaultSeedingConfigV2 returns a SeedingConfigV2 with sensible defaults.
func DefaultSeedingConfigV2() SeedingConfigV2 {
	return SeedingConfigV2{
		SitemapEnabled:   true,
		CDXEnabled:       false,
		RobotsTxtEnabled: true,
		RespectRobotsTxt: true,
		MaxDepth:         3,
		MaxPages:         10000,
		FollowRedirects:  true,
	}
}
