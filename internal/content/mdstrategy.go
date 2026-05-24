package content

import (
	"strings"
)

// MarkdownStrategy defines the interface for converting HTML to markdown.
type MarkdownStrategy interface {
	// Convert takes HTML content and a base URL, and returns markdown.
	Convert(html string, baseURL string) (string, error)
	// Name returns the strategy identifier.
	Name() string
}

// DefaultMarkdownStrategy uses the built-in HTML-to-markdown converter.
type DefaultMarkdownStrategy struct {
	// IncludeLinks preserves hyperlinks in the output.
	IncludeLinks bool
	// IncludeImages preserves image references in the output.
	IncludeImages bool
	// StripStyles removes inline style attributes before conversion.
	StripStyles bool
	// CodeBlocks preserves fenced code blocks.
	CodeBlocks bool
}

// NewDefaultMarkdownStrategy creates a strategy with sensible defaults.
func NewDefaultMarkdownStrategy() *DefaultMarkdownStrategy {
	return &DefaultMarkdownStrategy{
		IncludeLinks:  true,
		IncludeImages: true,
		StripStyles:   true,
		CodeBlocks:    true,
	}
}

// Name returns the strategy identifier.
func (s *DefaultMarkdownStrategy) Name() string {
	return "default"
}

// Convert uses the existing HTMLToMarkdown function from markdown.go.
func (s *DefaultMarkdownStrategy) Convert(html string, baseURL string) (string, error) {
	return HTMLToMarkdown(html, baseURL), nil
}

// FitMarkdownStrategy generates relevance-filtered markdown by keeping
// only content blocks that match the provided query terms.
type FitMarkdownStrategy struct {
	// QueryTerms are the words used to filter paragraphs.
	QueryTerms []string
	// MinScore is the minimum fraction of query terms that must appear
	// in a paragraph for it to be kept (0.0 to 1.0).
	MinScore float64
}

// Name returns the strategy identifier.
func (s *FitMarkdownStrategy) Name() string {
	return "fit"
}

// Convert first produces full markdown, then filters paragraphs by relevance
// to the configured query terms.
func (s *FitMarkdownStrategy) Convert(html string, baseURL string) (string, error) {
	md := HTMLToMarkdown(html, baseURL)
	if len(s.QueryTerms) == 0 {
		return md, nil
	}
	return filterByRelevance(md, s.QueryTerms, s.MinScore), nil
}

// filterByRelevance splits markdown into paragraphs (separated by blank lines)
// and keeps only those where the fraction of matching query terms meets or
// exceeds minScore. Headings (lines starting with #) are always kept so the
// document structure is preserved.
func filterByRelevance(md string, queryTerms []string, minScore float64) string {
	// Normalise query terms to lower-case for case-insensitive matching.
	lowerTerms := make([]string, len(queryTerms))
	for i, t := range queryTerms {
		lowerTerms[i] = strings.ToLower(t)
	}

	paragraphs := strings.Split(md, "\n\n")
	var kept []string

	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}

		// Always keep headings and reference sections.
		if strings.HasPrefix(trimmed, "#") {
			kept = append(kept, p)
			continue
		}

		lower := strings.ToLower(trimmed)
		matches := 0
		for _, term := range lowerTerms {
			if strings.Contains(lower, term) {
				matches++
			}
		}

		score := float64(matches) / float64(len(lowerTerms))
		if score >= minScore {
			kept = append(kept, p)
		}
	}

	return strings.Join(kept, "\n\n")
}
