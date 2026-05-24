package content

import (
	"context"
	"math"
	"strings"
)

// RelevanceConfig holds configuration for the ContentRelevanceFilter.
type RelevanceConfig struct {
	Query         string
	MinScore      float64
	MaxCandidates int
}

// ContentRelevanceFilter scores content blocks using BM25-style scoring
// against a query and keeps only relevant blocks. It implements ContentFilter.
type ContentRelevanceFilter struct {
	config RelevanceConfig
}

// NewContentRelevanceFilter creates a ContentRelevanceFilter with the given config.
func NewContentRelevanceFilter(cfg RelevanceConfig) *ContentRelevanceFilter {
	if cfg.MinScore <= 0 {
		cfg.MinScore = 0.5
	}
	if cfg.MaxCandidates <= 0 {
		cfg.MaxCandidates = 50
	}
	return &ContentRelevanceFilter{config: cfg}
}

func (f *ContentRelevanceFilter) Name() string { return "relevance" }

// Filter scores each block against the configured query using BM25 and
// returns blocks that meet the minimum score threshold.
func (f *ContentRelevanceFilter) Filter(_ context.Context, blocks []string, query string) ([]FilteredBlock, error) {
	q := query
	if q == "" {
		q = f.config.Query
	}
	if q == "" || len(blocks) == 0 {
		results := make([]FilteredBlock, len(blocks))
		for i, b := range blocks {
			results[i] = FilteredBlock{Content: b, Index: i, Kept: true, Score: 1.0}
		}
		return results, nil
	}

	// Limit candidates if configured.
	candidates := blocks
	if f.config.MaxCandidates > 0 && len(candidates) > f.config.MaxCandidates {
		candidates = candidates[:f.config.MaxCandidates]
	}

	results := make([]FilteredBlock, len(candidates))
	for i, block := range candidates {
		score := bm25Score(block, q)
		kept := score >= f.config.MinScore
		results[i] = FilteredBlock{
			Content: block,
			Score:   score,
			Index:   i,
			Kept:    kept,
		}
	}

	return results, nil
}

// FilterText is a convenience method that splits text into paragraphs,
// scores each against the query, and returns the relevant ones joined.
func (f *ContentRelevanceFilter) FilterText(text string) string {
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) == 0 || f.config.Query == "" {
		return text
	}

	var kept []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		score := bm25Score(p, f.config.Query)
		if score >= f.config.MinScore {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n\n")
}

// bm25Score computes a BM25-style relevance score for text against query.
func bm25Score(text, query string) float64 {
	textTokens := tokenize(text)
	queryTokens := tokenize(query)

	if len(textTokens) == 0 || len(queryTokens) == 0 {
		return 0
	}

	// Term frequency in document.
	tf := make(map[string]int)
	for _, t := range textTokens {
		tf[t]++
	}

	const (
		k1    = 1.5
		b     = 0.75
		avgDL = 100.0 // Assumed average document length.
	)
	dl := float64(len(textTokens))

	score := 0.0
	for _, qt := range queryTokens {
		termFreq := float64(tf[qt])
		if termFreq == 0 {
			continue
		}
		// Simplified IDF: assume a corpus where the term appears in ~50% of docs.
		idf := math.Log(2.0)
		tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*dl/avgDL))
		score += idf * tfNorm
	}

	return score
}
