package content

import (
	"context"
	"testing"
)

func TestBM25ContentFilter_Name(t *testing.T) {
	f := NewBM25ContentFilter()
	if got := f.Name(); got != "bm25" {
		t.Errorf("Name() = %q, want %q", got, "bm25")
	}
}

func TestBM25ContentFilter_Filter_Empty(t *testing.T) {
	f := NewBM25ContentFilter()
	results, err := f.Filter(context.Background(), nil, "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty blocks, got %v", results)
	}
}

func TestBM25ContentFilter_Filter(t *testing.T) {
	tests := []struct {
		name      string
		blocks    []string
		query     string
		wantCount int
	}{
		{
			name:      "single relevant block",
			blocks:    []string{"golang programming language is great for systems software"},
			query:     "golang",
			wantCount: 1,
		},
		{
			name:      "multiple blocks some relevant",
			blocks:    []string{
				"golang programming language",
				"unrelated text about cooking recipes",
				"go language concurrency features goroutines channels",
			},
			query:     "golang go language",
			wantCount: 3, // all blocks returned, some marked kept
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewBM25ContentFilter()
			results, err := f.Filter(context.Background(), tc.blocks, tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tc.wantCount {
				t.Errorf("got %d results, want %d", len(results), tc.wantCount)
			}
			// Verify index assignment.
			for i, r := range results {
				if r.Index != i {
					t.Errorf("results[%d].Index = %d, want %d", i, r.Index, i)
				}
			}
		})
	}
}

func TestPruningContentFilter_Name(t *testing.T) {
	f := NewPruningContentFilter()
	if got := f.Name(); got != "pruning" {
		t.Errorf("Name() = %q, want %q", got, "pruning")
	}
}

func TestPruningContentFilter_Filter_Empty(t *testing.T) {
	f := NewPruningContentFilter()
	results, err := f.Filter(context.Background(), nil, "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestPruningContentFilter_Filter(t *testing.T) {
	tests := []struct {
		name   string
		blocks []string
		query  string
	}{
		{
			name:   "rich text block kept",
			blocks: []string{"<p>This is a meaningful paragraph with plenty of content.</p>"},
			query:  "",
		},
		{
			name:   "returns result for each block",
			blocks: []string{"<p>Block one</p>", "<p>Block two with content</p>"},
			query:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := NewPruningContentFilter()
			results, err := f.Filter(context.Background(), tc.blocks, tc.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != len(tc.blocks) {
				t.Errorf("got %d results, want %d", len(results), len(tc.blocks))
			}
			for i, r := range results {
				if r.Index != i {
					t.Errorf("results[%d].Index = %d, want %d", i, r.Index, i)
				}
			}
		})
	}
}

func TestFilterPipeline_Run_Empty(t *testing.T) {
	p := NewFilterPipeline()
	results, err := p.Run(context.Background(), nil, "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for empty pipeline + empty blocks, got %v", results)
	}
}

func TestFilterPipeline_Run(t *testing.T) {
	// A passthrough filter that marks everything as kept.
	pass := &alwaysKeptFilter{}
	p := NewFilterPipeline(pass)

	blocks := []string{"block one", "block two", "block three"}
	results, err := p.Run(context.Background(), blocks, "query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != len(blocks) {
		t.Fatalf("expected %d results, got %d", len(blocks), len(results))
	}
	for _, r := range results {
		if !r.Kept {
			t.Errorf("block %d should be kept by passthrough filter", r.Index)
		}
	}
}

func TestFilterPipeline_Run_ChainedFilters(t *testing.T) {
	// First filter keeps all; second filter discards all.
	p := NewFilterPipeline(&alwaysKeptFilter{}, &neverKeptFilter{})

	blocks := []string{"a", "b"}
	results, err := p.Run(context.Background(), blocks, "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The second filter should have received kept blocks from the first.
	// The second filter marks nothing as kept.
	for _, r := range results {
		if r.Kept {
			t.Errorf("expected no blocks kept after neverKeptFilter")
		}
	}
}

func TestFilterPipeline_Run_ContextCancel(t *testing.T) {
	p := NewFilterPipeline(&alwaysKeptFilter{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Run(ctx, []string{"a", "b"}, "q")
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

// alwaysKeptFilter marks every block as kept.
type alwaysKeptFilter struct{}

func (f *alwaysKeptFilter) Name() string { return "always" }
func (f *alwaysKeptFilter) Filter(_ context.Context, blocks []string, _ string) ([]FilteredBlock, error) {
	results := make([]FilteredBlock, len(blocks))
	for i, b := range blocks {
		results[i] = FilteredBlock{Content: b, Index: i, Kept: true, Score: 1.0}
	}
	return results, nil
}

// neverKeptFilter marks every block as not kept.
type neverKeptFilter struct{}

func (f *neverKeptFilter) Name() string { return "never" }
func (f *neverKeptFilter) Filter(_ context.Context, blocks []string, _ string) ([]FilteredBlock, error) {
	results := make([]FilteredBlock, len(blocks))
	for i, b := range blocks {
		results[i] = FilteredBlock{Content: b, Index: i, Kept: false, Score: 0.0}
	}
	return results, nil
}
