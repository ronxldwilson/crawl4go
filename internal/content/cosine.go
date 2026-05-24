package content

import (
	"context"
	"math"
	"sort"
	"strings"
)

// Embedder is the interface for computing text embeddings.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}

// textBlock is an internal representation of a text segment with its embedding.
type textBlock struct {
	text      string
	embedding []float64
	index     int
}

// CosineStrategy extracts the most relevant content from HTML by computing
// embeddings, clustering by cosine similarity, and returning the top-N
// clusters. It implements the ExtractionStrategy interface.
type CosineStrategy struct {
	Threshold float64
	TopN      int
	embedFunc Embedder
}

// NewCosineStrategy creates a CosineStrategy with sensible defaults.
func NewCosineStrategy(embedder Embedder) *CosineStrategy {
	return &CosineStrategy{
		Threshold: 0.6,
		TopN:      5,
		embedFunc: embedder,
	}
}

func (s *CosineStrategy) Name() string { return "cosine" }

// Extract splits the HTML into text blocks, embeds each block, clusters them
// by cosine similarity, and returns the top-N most relevant clusters.
func (s *CosineStrategy) Extract(ctx context.Context, html string, _ ExtractionConfig) ([]ExtractionResult, error) {
	rawBlocks := splitTextBlocks(html)
	if len(rawBlocks) == 0 {
		return nil, nil
	}

	blocks := make([]textBlock, 0, len(rawBlocks))
	for i, raw := range rawBlocks {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emb, err := s.embedFunc.Embed(ctx, raw)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, textBlock{
			text:      raw,
			embedding: emb,
			index:     i,
		})
	}

	clusters := clusterBlocks(blocks, s.Threshold)

	// Sort clusters by size descending so the largest (most coherent) come first.
	sort.Slice(clusters, func(i, j int) bool {
		return len(clusters[i]) > len(clusters[j])
	})

	topN := s.TopN
	if topN <= 0 {
		topN = 5
	}
	if topN > len(clusters) {
		topN = len(clusters)
	}

	var results []ExtractionResult
	for ci := 0; ci < topN; ci++ {
		var parts []string
		for _, b := range clusters[ci] {
			parts = append(parts, b.text)
		}
		results = append(results, ExtractionResult{
			Content: strings.Join(parts, "\n\n"),
			Index:   ci,
		})
	}

	return results, nil
}

// splitTextBlocks splits HTML into meaningful text segments by stripping tags,
// splitting on double-newlines, and discarding blanks.
func splitTextBlocks(html string) []string {
	plain := HTMLToText(html)
	parts := strings.Split(plain, "  ") // HTMLToText collapses whitespace; split on double-space boundaries
	var blocks []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if len(t) > 20 { // skip very short fragments
			blocks = append(blocks, t)
		}
	}
	return blocks
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// clusterBlocks groups text blocks using single-linkage agglomerative
// clustering based on cosine similarity. Two blocks are in the same cluster
// if any pair within the clusters exceeds the threshold.
func clusterBlocks(blocks []textBlock, threshold float64) [][]textBlock {
	n := len(blocks)
	if n == 0 {
		return nil
	}

	// Each block starts in its own cluster.
	clusterID := make([]int, n)
	for i := range clusterID {
		clusterID[i] = i
	}

	// find returns the root of the cluster for element i.
	var find func(int) int
	find = func(i int) int {
		for clusterID[i] != i {
			clusterID[i] = clusterID[clusterID[i]]
			i = clusterID[i]
		}
		return i
	}

	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			clusterID[rb] = ra
		}
	}

	// Merge blocks that are similar enough.
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if cosineSimilarity(blocks[i].embedding, blocks[j].embedding) >= threshold {
				union(i, j)
			}
		}
	}

	// Collect clusters.
	groups := make(map[int][]textBlock)
	for i := range blocks {
		root := find(i)
		groups[root] = append(groups[root], blocks[i])
	}

	result := make([][]textBlock, 0, len(groups))
	for _, g := range groups {
		result = append(result, g)
	}
	return result
}
