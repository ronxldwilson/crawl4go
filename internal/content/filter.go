package content

import (
	"context"
	"fmt"
	"strings"
)

// ContentFilter defines the interface for filtering extracted content
// by relevance. Implementations score or prune content blocks.
type ContentFilter interface {
	// Filter takes content blocks and returns only relevant ones.
	Filter(ctx context.Context, blocks []string, query string) ([]FilteredBlock, error)
	// Name returns the filter identifier.
	Name() string
}

// FilteredBlock is a content block with a relevance score.
type FilteredBlock struct {
	Content string  `json:"content"`
	Score   float64 `json:"score"`
	Index   int     `json:"index"`
	Kept    bool    `json:"kept"`
}

// FilterPipeline chains multiple content filters in sequence.
type FilterPipeline struct {
	filters []ContentFilter
}

// NewFilterPipeline creates a pipeline from the given filters.
func NewFilterPipeline(filters ...ContentFilter) *FilterPipeline {
	return &FilterPipeline{filters: filters}
}

// Run applies each filter in sequence, passing kept blocks to the next.
func (p *FilterPipeline) Run(ctx context.Context, blocks []string, query string) ([]FilteredBlock, error) {
	current := blocks
	var lastResult []FilteredBlock

	for _, f := range p.filters {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := f.Filter(ctx, current, query)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name(), err)
		}
		lastResult = result

		// Collect kept blocks for the next stage.
		current = current[:0]
		for _, b := range result {
			if b.Kept {
				current = append(current, b.Content)
			}
		}
		if len(current) == 0 {
			break
		}
	}

	return lastResult, nil
}

// ---------------------------------------------------------------------------
// BM25ContentFilter wraps the existing BM25Filter to implement ContentFilter.
// ---------------------------------------------------------------------------

// BM25ContentFilter scores content blocks using BM25 relevance ranking.
type BM25ContentFilter struct {
	bm25 *BM25Filter
}

// NewBM25ContentFilter creates a BM25ContentFilter with default parameters.
func NewBM25ContentFilter() *BM25ContentFilter {
	return &BM25ContentFilter{bm25: NewBM25Filter()}
}

// NewBM25ContentFilterWith creates a BM25ContentFilter with custom parameters.
func NewBM25ContentFilterWith(k1, b, threshold float64) *BM25ContentFilter {
	return &BM25ContentFilter{bm25: &BM25Filter{K1: k1, B: b, Threshold: threshold}}
}

func (f *BM25ContentFilter) Name() string { return "bm25" }

func (f *BM25ContentFilter) Filter(_ context.Context, blocks []string, query string) ([]FilteredBlock, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	// Convert plain-text blocks into TextChunks.
	chunks := make([]TextChunk, len(blocks))
	for i, b := range blocks {
		chunks[i] = TextChunk{Index: i, Text: b, TagName: "p"}
	}

	kept := f.bm25.FilterByRelevance(chunks, query)

	// Build a set of kept indices for O(1) lookup.
	keptSet := make(map[int]bool, len(kept))
	for _, k := range kept {
		keptSet[k.Index] = true
	}

	results := make([]FilteredBlock, len(blocks))
	for i, b := range blocks {
		results[i] = FilteredBlock{
			Content: b,
			Index:   i,
			Kept:    keptSet[i],
		}
		if results[i].Kept {
			results[i].Score = 1.0 // block passed the BM25 threshold
		}
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// PruningContentFilter wraps the existing PruningFilter to implement
// ContentFilter. Each block is wrapped in a minimal HTML document, pruned,
// and kept only if meaningful text survives.
// ---------------------------------------------------------------------------

// PruningContentFilter removes boilerplate from content blocks using the
// tree-pruning algorithm.
type PruningContentFilter struct {
	pruner *PruningFilter
}

// NewPruningContentFilter creates a PruningContentFilter with default parameters.
func NewPruningContentFilter() *PruningContentFilter {
	return &PruningContentFilter{pruner: NewPruningFilter()}
}

// NewPruningContentFilterWith creates a PruningContentFilter with a custom threshold.
func NewPruningContentFilterWith(threshold float64) *PruningContentFilter {
	return &PruningContentFilter{pruner: &PruningFilter{Threshold: threshold}}
}

func (f *PruningContentFilter) Name() string { return "pruning" }

func (f *PruningContentFilter) Filter(_ context.Context, blocks []string, _ string) ([]FilteredBlock, error) {
	results := make([]FilteredBlock, len(blocks))
	for i, b := range blocks {
		// Wrap the block in a minimal HTML body so the pruner can operate on it.
		wrapped := "<html><body><div>" + b + "</div></body></html>"
		pruned, err := f.pruner.Filter(wrapped)
		if err != nil {
			return nil, err
		}

		remaining := strings.TrimSpace(pruned)
		kept := len(remaining) > 0

		score := 0.0
		if kept {
			score = 1.0
		}

		results[i] = FilteredBlock{
			Content: b,
			Score:   score,
			Index:   i,
			Kept:    kept,
		}
	}
	return results, nil
}
