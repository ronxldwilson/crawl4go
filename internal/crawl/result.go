package crawl

// CrawlResult is the comprehensive per-page result that captures everything
// about a single page crawl — content in multiple formats, extraction results,
// performance data, and error details.
type CrawlResult struct {
	// Identity
	URL       string `json:"url"`
	ParentURL string `json:"parent_url,omitempty"`
	Depth     int    `json:"depth"`

	// HTTP response
	StatusCode      int               `json:"status_code"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`

	// Content in multiple formats
	HTML     string          `json:"html,omitempty"`
	Text     string          `json:"text,omitempty"`
	Markdown *MarkdownResult `json:"markdown,omitempty"`

	// Extraction results
	Links    []string       `json:"links,omitempty"`
	Media    *MediaResult   `json:"media,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`

	// Anti-bot detection
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason,omitempty"`

	// Scoring
	Score       float64 `json:"score,omitempty"`
	ContentHash string  `json:"content_hash,omitempty"`

	// Performance
	RenderTimeMs int64  `json:"render_time_ms"`
	RenderSource string `json:"render_source"` // fetch, cdp

	// Error details
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorCode    string `json:"error_code,omitempty"`

	// Retry tracking
	AttemptCount int `json:"attempt_count"`
}

// MarkdownResult holds generated markdown with optional citation references.
type MarkdownResult struct {
	Raw           string        `json:"raw"`
	WithCitations string        `json:"with_citations,omitempty"`
	References    []CitationRef `json:"references,omitempty"`
	FitMarkdown   string        `json:"fit_markdown,omitempty"` // relevance-filtered
}

// CitationRef maps a citation number to its source URL and anchor text.
type CitationRef struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
}

// MediaResult holds extracted media items from a page.
type MediaResult struct {
	Images []MediaItem `json:"images,omitempty"`
	Videos []MediaItem `json:"videos,omitempty"`
	Audio  []MediaItem `json:"audio,omitempty"`
}

// MediaItem represents a single media element found on a page.
type MediaItem struct {
	URL       string  `json:"url"`
	Alt       string  `json:"alt,omitempty"`
	Title     string  `json:"title,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	MimeType  string  `json:"mime_type,omitempty"`
	Score     float64 `json:"score,omitempty"`
	IsContent bool    `json:"is_content"` // vs decorative
}
