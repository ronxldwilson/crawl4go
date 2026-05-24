package content

import (
	"context"
	"math"
	"strings"
)

// EmbeddingStrategy extracts content by embedding document sections and
// tracking semantic coverage of the embedding space. It stops collecting
// sections once the coverage threshold is met. Implements ExtractionStrategy.
type EmbeddingStrategy struct {
	embedFunc         Embedder
	CoverageThreshold float64
	MaxIterations     int
}

// NewEmbeddingStrategy creates an EmbeddingStrategy with sensible defaults.
func NewEmbeddingStrategy(embedder Embedder) *EmbeddingStrategy {
	return &EmbeddingStrategy{
		embedFunc:         embedder,
		CoverageThreshold: 0.8,
		MaxIterations:     100,
	}
}

func (s *EmbeddingStrategy) Name() string { return "embedding" }

// Extract splits HTML into sections, embeds them, and collects sections until
// the semantic coverage of the accumulated embeddings reaches the threshold
// or MaxIterations is hit.
func (s *EmbeddingStrategy) Extract(ctx context.Context, html string, _ ExtractionConfig) ([]ExtractionResult, error) {
	sections := splitSections(html)
	if len(sections) == 0 {
		return nil, nil
	}

	maxIter := s.MaxIterations
	if maxIter <= 0 {
		maxIter = len(sections)
	}
	if maxIter > len(sections) {
		maxIter = len(sections)
	}

	var embeddings [][]float64
	var results []ExtractionResult

	for i := 0; i < maxIter; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emb, err := s.embedFunc.Embed(ctx, sections[i])
		if err != nil {
			return nil, err
		}

		embeddings = append(embeddings, emb)
		results = append(results, ExtractionResult{
			Content: sections[i],
			Index:   i,
		})

		if len(embeddings) >= 2 {
			coverage := semanticCoverage(embeddings)
			if coverage >= s.CoverageThreshold {
				break
			}
		}
	}

	return results, nil
}

// splitSections splits HTML into meaningful text sections using paragraph-like
// boundaries.
func splitSections(html string) []string {
	plain := HTMLToText(html)
	// Split on sentence-like boundaries (double space from collapsed whitespace).
	parts := strings.Fields(plain)

	// Group words into sections of roughly 50 words each.
	const wordsPerSection = 50
	var sections []string
	for i := 0; i < len(parts); i += wordsPerSection {
		end := i + wordsPerSection
		if end > len(parts) {
			end = len(parts)
		}
		section := strings.Join(parts[i:end], " ")
		if len(strings.TrimSpace(section)) > 0 {
			sections = append(sections, section)
		}
	}
	return sections
}

// semanticCoverage measures how spread out a set of embeddings are by
// computing the average pairwise cosine distance. A value near 1.0 means
// the embeddings cover diverse semantic territory; near 0.0 means they are
// all very similar.
func semanticCoverage(embeddings [][]float64) float64 {
	n := len(embeddings)
	if n < 2 {
		return 0
	}

	var totalDist float64
	pairs := 0

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			sim := cosineSimilarity(embeddings[i], embeddings[j])
			// Clamp similarity to [-1, 1] before converting to distance.
			sim = math.Max(-1, math.Min(1, sim))
			totalDist += 1 - sim
			pairs++
		}
	}

	if pairs == 0 {
		return 0
	}
	return totalDist / float64(pairs)
}
