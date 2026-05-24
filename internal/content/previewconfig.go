package content

import "time"

// LinkPreviewConfig controls how link previews are fetched and scored.
type LinkPreviewConfig struct {
	MaxConcurrent  int           `json:"max_concurrent"`
	Timeout        time.Duration `json:"timeout"`
	MaxBodyBytes   int64         `json:"max_body_bytes"`
	ScoreByBM25    bool          `json:"score_by_bm25"`
	QueryTerms     []string      `json:"query_terms,omitempty"`
	MinScore       float64       `json:"min_score"`
	IncludeOG      bool          `json:"include_og"`
	IncludeTwitter bool          `json:"include_twitter"`
}

// DefaultLinkPreviewConfig returns sensible defaults.
func DefaultLinkPreviewConfig() LinkPreviewConfig {
	return LinkPreviewConfig{
		MaxConcurrent:  10,
		Timeout:        10 * time.Second,
		MaxBodyBytes:   32 * 1024,
		ScoreByBM25:    false,
		IncludeOG:      true,
		IncludeTwitter: true,
	}
}
