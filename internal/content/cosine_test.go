package content

import (
	"context"
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float64
		b    []float64
		want float64
		eps  float64
	}{
		{
			name: "identical vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{1, 0, 0},
			want: 1.0,
			eps:  1e-9,
		},
		{
			name: "orthogonal vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{0, 1, 0},
			want: 0.0,
			eps:  1e-9,
		},
		{
			name: "opposite vectors",
			a:    []float64{1, 0, 0},
			b:    []float64{-1, 0, 0},
			want: -1.0,
			eps:  1e-9,
		},
		{
			name: "45 degree angle",
			a:    []float64{1, 0},
			b:    []float64{1, 1},
			want: 1.0 / math.Sqrt(2),
			eps:  1e-9,
		},
		{
			name: "empty vectors",
			a:    []float64{},
			b:    []float64{},
			want: 0.0,
			eps:  1e-9,
		},
		{
			name: "mismatched lengths",
			a:    []float64{1, 2},
			b:    []float64{1},
			want: 0.0,
			eps:  1e-9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if math.Abs(got-tc.want) > tc.eps {
				t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float64{0, 0, 0}
	b := []float64{1, 2, 3}
	got := cosineSimilarity(a, b)
	if got != 0.0 {
		t.Errorf("cosineSimilarity(zero, b) = %f, want 0.0", got)
	}
}

func TestSplitTextBlocks(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		minCount int
	}{
		{
			name:     "paragraph HTML",
			html:     "<p>This is the first paragraph with more than twenty characters.</p><p>This is the second paragraph also long enough.</p>",
			minCount: 1,
		},
		{
			name:     "empty input",
			html:     "",
			minCount: 0,
		},
		{
			name:     "very short text filtered",
			html:     "<p>Hi</p>",
			minCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitTextBlocks(tc.html)
			if len(got) < tc.minCount {
				t.Errorf("splitTextBlocks(%q) returned %d blocks, want at least %d", tc.html, len(got), tc.minCount)
			}
		})
	}
}

func TestClusterBlocks(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		got := clusterBlocks(nil, 0.6)
		if got != nil {
			t.Errorf("expected nil for empty input, got %v", got)
		}
	})

	t.Run("single block", func(t *testing.T) {
		blocks := []textBlock{
			{text: "hello", embedding: []float64{1, 0}, index: 0},
		}
		got := clusterBlocks(blocks, 0.6)
		if len(got) != 1 {
			t.Errorf("expected 1 cluster, got %d", len(got))
		}
	})

	t.Run("identical embeddings merge into one cluster", func(t *testing.T) {
		emb := []float64{1, 0, 0}
		blocks := []textBlock{
			{text: "a", embedding: emb, index: 0},
			{text: "b", embedding: emb, index: 1},
			{text: "c", embedding: emb, index: 2},
		}
		got := clusterBlocks(blocks, 0.5)
		if len(got) != 1 {
			t.Errorf("identical embeddings should produce 1 cluster, got %d", len(got))
		}
		if len(got[0]) != 3 {
			t.Errorf("cluster should have 3 blocks, got %d", len(got[0]))
		}
	})

	t.Run("orthogonal embeddings remain separate", func(t *testing.T) {
		blocks := []textBlock{
			{text: "a", embedding: []float64{1, 0, 0}, index: 0},
			{text: "b", embedding: []float64{0, 1, 0}, index: 1},
			{text: "c", embedding: []float64{0, 0, 1}, index: 2},
		}
		got := clusterBlocks(blocks, 0.5)
		if len(got) != 3 {
			t.Errorf("orthogonal embeddings should produce 3 clusters, got %d", len(got))
		}
	})
}

func TestCosineStrategy_Name(t *testing.T) {
	s := NewCosineStrategy(&mockEmbedderCosine{})
	if got := s.Name(); got != "cosine" {
		t.Errorf("Name() = %q, want %q", got, "cosine")
	}
}

func TestCosineStrategy_Extract_EmptyHTML(t *testing.T) {
	s := NewCosineStrategy(&mockEmbedderCosine{})
	results, err := s.Extract(context.Background(), "", ExtractionConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty HTML, got %d", len(results))
	}
}

// mockEmbedderCosine returns a fixed embedding for testing.
type mockEmbedderCosine struct {
	callCount int
}

func (m *mockEmbedderCosine) Embed(_ context.Context, _ string) ([]float64, error) {
	m.callCount++
	// Return distinct-enough embeddings by cycling through basis vectors.
	switch m.callCount % 3 {
	case 0:
		return []float64{1, 0, 0}, nil
	case 1:
		return []float64{0, 1, 0}, nil
	default:
		return []float64{0, 0, 1}, nil
	}
}
