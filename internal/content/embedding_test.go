package content

import (
	"context"
	"math"
	"testing"
)

// mockEmbedder cycles through a fixed list of embeddings.
type mockEmbedder struct {
	embeddings [][]float64
	callCount  int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float64, error) {
	idx := m.callCount % len(m.embeddings)
	m.callCount++
	return m.embeddings[idx], nil
}

func TestEmbeddingStrategy_Name(t *testing.T) {
	s := NewEmbeddingStrategy(&mockEmbedder{embeddings: [][]float64{{1}}})
	if got := s.Name(); got != "embedding" {
		t.Errorf("Name() = %q, want %q", got, "embedding")
	}
}

func TestSplitSections(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		minCount int
		maxCount int
	}{
		{
			name:     "empty input",
			html:     "",
			minCount: 0,
			maxCount: 0,
		},
		{
			name:     "short text yields one section",
			html:     "<p>Hello world this is a simple paragraph.</p>",
			minCount: 1,
			maxCount: 1,
		},
		{
			name:     "long text splits into multiple sections",
			html:     buildLongHTML(200),
			minCount: 2,
			maxCount: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitSections(tc.html)
			if len(got) < tc.minCount || len(got) > tc.maxCount {
				t.Errorf("splitSections returned %d sections, want [%d, %d]", len(got), tc.minCount, tc.maxCount)
			}
		})
	}
}

// buildLongHTML creates an HTML string with roughly n words.
func buildLongHTML(words int) string {
	const word = "word "
	result := "<p>"
	for i := 0; i < words; i++ {
		result += word
	}
	result += "</p>"
	return result
}

func TestSemanticCoverage_SingleEmbedding(t *testing.T) {
	got := semanticCoverage([][]float64{{1, 0, 0}})
	if got != 0 {
		t.Errorf("semanticCoverage with single embedding = %f, want 0", got)
	}
}

func TestSemanticCoverage_EmptyEmbeddings(t *testing.T) {
	got := semanticCoverage(nil)
	if got != 0 {
		t.Errorf("semanticCoverage with nil = %f, want 0", got)
	}
}

func TestSemanticCoverage(t *testing.T) {
	tests := []struct {
		name       string
		embeddings [][]float64
		wantMin    float64
		wantMax    float64
	}{
		{
			name: "identical embeddings - low coverage",
			embeddings: [][]float64{
				{1, 0, 0},
				{1, 0, 0},
			},
			wantMin: 0.0,
			wantMax: 0.01,
		},
		{
			name: "orthogonal embeddings - high coverage",
			embeddings: [][]float64{
				{1, 0, 0},
				{0, 1, 0},
			},
			wantMin: 0.99,
			wantMax: 1.01,
		},
		{
			name: "three diverse embeddings",
			embeddings: [][]float64{
				{1, 0, 0},
				{0, 1, 0},
				{0, 0, 1},
			},
			wantMin: 0.9,
			wantMax: 1.1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := semanticCoverage(tc.embeddings)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("semanticCoverage = %f, want [%f, %f]", got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestEmbeddingStrategy_Extract_Empty(t *testing.T) {
	s := NewEmbeddingStrategy(&mockEmbedder{embeddings: [][]float64{{1, 0}}})
	results, err := s.Extract(context.Background(), "", ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty HTML, got %d", len(results))
	}
}

func TestEmbeddingStrategy_Extract_StopsAtCoverage(t *testing.T) {
	// Two orthogonal embeddings produce coverage ~1.0 which exceeds default 0.8.
	embedder := &mockEmbedder{
		embeddings: [][]float64{
			{1, 0, 0},
			{0, 1, 0},
			{0, 0, 1},
			{1, 1, 0},
		},
	}
	s := NewEmbeddingStrategy(embedder)
	s.CoverageThreshold = 0.8

	html := buildLongHTML(300) // ensure multiple sections
	results, err := s.Extract(context.Background(), html, ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should stop early once coverage threshold is met.
	if len(results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestEmbeddingStrategy_Extract_ContextCancel(t *testing.T) {
	embedder := &mockEmbedder{
		embeddings: [][]float64{{1, 0}},
	}
	s := NewEmbeddingStrategy(embedder)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := s.Extract(ctx, buildLongHTML(200), ExtractionConfig{})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

func TestSemanticCoverage_KnownValue(t *testing.T) {
	// Two opposite vectors: cosine sim = -1, distance = 1 - (-1) = 2.
	// Average distance = 2.
	a := []float64{1, 0}
	b := []float64{-1, 0}
	got := semanticCoverage([][]float64{a, b})
	want := 2.0
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("semanticCoverage opposite vectors = %f, want %f", got, want)
	}
}
