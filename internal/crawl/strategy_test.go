package crawl

import (
	"context"
	"fmt"
	"testing"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

// mockCrawlPages builds a CrawlFunc from a map of URL -> page content with links.
func mockCrawlPages(pages map[string]*DeepCrawlResult) CrawlFunc {
	return func(ctx context.Context, pageURL string) (*DeepCrawlResult, error) {
		if r, ok := pages[pageURL]; ok {
			cp := *r
			return &cp, nil
		}
		return nil, fmt.Errorf("not found: %s", pageURL)
	}
}

func buildLinearSite() map[string]*DeepCrawlResult {
	// A -> B -> C (linear chain)
	return map[string]*DeepCrawlResult{
		"https://example.com": {
			URL:     "https://example.com",
			Content: "root page",
			Links: content.LinkSet{
				Internal: []content.Link{
					{Href: "https://example.com/page1", Text: "page1"},
				},
				External: []content.Link{},
			},
		},
		"https://example.com/page1": {
			URL:     "https://example.com/page1",
			Content: "page 1",
			Links: content.LinkSet{
				Internal: []content.Link{
					{Href: "https://example.com/page2", Text: "page2"},
				},
				External: []content.Link{},
			},
		},
		"https://example.com/page2": {
			URL:     "https://example.com/page2",
			Content: "page 2",
			Links: content.LinkSet{
				Internal: []content.Link{},
				External: []content.Link{},
			},
		},
	}
}

func buildBranchingSite() map[string]*DeepCrawlResult {
	// Root links to A and B; A links to C.
	return map[string]*DeepCrawlResult{
		"https://example.com": {
			URL:     "https://example.com",
			Content: "root",
			Links: content.LinkSet{
				Internal: []content.Link{
					{Href: "https://example.com/a", Text: "A"},
					{Href: "https://example.com/b", Text: "B"},
				},
				External: []content.Link{},
			},
		},
		"https://example.com/a": {
			URL:     "https://example.com/a",
			Content: "page a",
			Links: content.LinkSet{
				Internal: []content.Link{
					{Href: "https://example.com/c", Text: "C"},
				},
				External: []content.Link{},
			},
		},
		"https://example.com/b": {
			URL:     "https://example.com/b",
			Content: "page b",
			Links: content.LinkSet{
				Internal: []content.Link{},
				External: []content.Link{},
			},
		},
		"https://example.com/c": {
			URL:     "https://example.com/c",
			Content: "page c",
			Links: content.LinkSet{
				Internal: []content.Link{},
				External: []content.Link{},
			},
		},
	}
}

func TestBFSStrategy(t *testing.T) {
	tests := []struct {
		name          string
		pages         map[string]*DeepCrawlResult
		opts          CrawlOptions
		wantMinPages  int
		wantMaxPages  int
		wantMaxDepth  int
	}{
		{
			name:         "linear site depth 2",
			pages:        buildLinearSite(),
			opts:         CrawlOptions{MaxDepth: 2, MaxPages: 10},
			wantMinPages: 3,
			wantMaxPages: 3,
			wantMaxDepth: 2,
		},
		{
			name:         "branching site depth 1",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 1, MaxPages: 10},
			wantMinPages: 3,
			wantMaxPages: 3,
			wantMaxDepth: 1,
		},
		{
			name:         "max pages limits crawl",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 5, MaxPages: 2},
			wantMinPages: 1,
			wantMaxPages: 2,
		},
		{
			name:         "depth 0 only crawls start",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 0, MaxPages: 10},
			wantMinPages: 1,
			wantMaxPages: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &BFSStrategy{}
			results, stats := s.Run(context.Background(), "https://example.com", mockCrawlPages(tt.pages), tt.opts)

			if stats.PagesCrawled < tt.wantMinPages {
				t.Errorf("PagesCrawled = %d, want >= %d", stats.PagesCrawled, tt.wantMinPages)
			}
			if stats.PagesCrawled > tt.wantMaxPages {
				t.Errorf("PagesCrawled = %d, want <= %d", stats.PagesCrawled, tt.wantMaxPages)
			}
			if len(results) < tt.wantMinPages {
				t.Errorf("len(results) = %d, want >= %d", len(results), tt.wantMinPages)
			}
			if tt.wantMaxDepth > 0 && stats.MaxDepthReached > tt.wantMaxDepth {
				t.Errorf("MaxDepthReached = %d, want <= %d", stats.MaxDepthReached, tt.wantMaxDepth)
			}
		})
	}
}

func TestDFSStrategy(t *testing.T) {
	tests := []struct {
		name         string
		pages        map[string]*DeepCrawlResult
		opts         CrawlOptions
		wantMinPages int
		wantMaxPages int
	}{
		{
			name:         "linear site full traversal",
			pages:        buildLinearSite(),
			opts:         CrawlOptions{MaxDepth: 3, MaxPages: 10},
			wantMinPages: 3,
			wantMaxPages: 3,
		},
		{
			name:         "branching site full traversal",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 3, MaxPages: 10},
			wantMinPages: 4,
			wantMaxPages: 4,
		},
		{
			name:         "max pages limits crawl",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 5, MaxPages: 2},
			wantMinPages: 2,
			wantMaxPages: 2,
		},
		{
			name:         "depth 0 only crawls start",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 0, MaxPages: 10},
			wantMinPages: 1,
			wantMaxPages: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &DFSStrategy{}
			results, stats := s.Run(context.Background(), "https://example.com", mockCrawlPages(tt.pages), tt.opts)

			if stats.PagesCrawled < tt.wantMinPages {
				t.Errorf("PagesCrawled = %d, want >= %d", stats.PagesCrawled, tt.wantMinPages)
			}
			if stats.PagesCrawled > tt.wantMaxPages {
				t.Errorf("PagesCrawled = %d, want <= %d", stats.PagesCrawled, tt.wantMaxPages)
			}
			if len(results) < tt.wantMinPages {
				t.Errorf("len(results) = %d, want >= %d", len(results), tt.wantMinPages)
			}
		})
	}
}

func TestBestFirstStrategy(t *testing.T) {
	tests := []struct {
		name         string
		pages        map[string]*DeepCrawlResult
		opts         CrawlOptions
		wantMinPages int
		wantMaxPages int
	}{
		{
			name:         "linear site without scorer",
			pages:        buildLinearSite(),
			opts:         CrawlOptions{MaxDepth: 3, MaxPages: 10},
			wantMinPages: 3,
			wantMaxPages: 3,
		},
		{
			name:  "with keyword scorer",
			pages: buildBranchingSite(),
			opts: CrawlOptions{
				MaxDepth: 3,
				MaxPages: 10,
				Scorer:   NewKeywordRelevanceScorer([]string{"page"}),
			},
			wantMinPages: 4,
			wantMaxPages: 4,
		},
		{
			name:         "max pages limits crawl",
			pages:        buildBranchingSite(),
			opts:         CrawlOptions{MaxDepth: 5, MaxPages: 2},
			wantMinPages: 2,
			wantMaxPages: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &BestFirstStrategy{}
			results, stats := s.Run(context.Background(), "https://example.com", mockCrawlPages(tt.pages), tt.opts)

			if stats.PagesCrawled < tt.wantMinPages {
				t.Errorf("PagesCrawled = %d, want >= %d", stats.PagesCrawled, tt.wantMinPages)
			}
			if stats.PagesCrawled > tt.wantMaxPages {
				t.Errorf("PagesCrawled = %d, want <= %d", stats.PagesCrawled, tt.wantMaxPages)
			}
			if len(results) < tt.wantMinPages {
				t.Errorf("len(results) = %d, want >= %d", len(results), tt.wantMinPages)
			}
		})
	}
}

func TestBFSContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	s := &BFSStrategy{}
	_, stats := s.Run(ctx, "https://example.com", mockCrawlPages(buildLinearSite()), CrawlOptions{MaxDepth: 3, MaxPages: 10})

	if stats.PagesCrawled > 1 {
		t.Errorf("PagesCrawled = %d after cancellation, expected <= 1", stats.PagesCrawled)
	}
}

func TestDFSContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &DFSStrategy{}
	_, stats := s.Run(ctx, "https://example.com", mockCrawlPages(buildLinearSite()), CrawlOptions{MaxDepth: 3, MaxPages: 10})

	if stats.PagesCrawled > 0 {
		t.Errorf("PagesCrawled = %d after cancellation, expected 0", stats.PagesCrawled)
	}
}

func TestBestFirstContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &BestFirstStrategy{}
	_, stats := s.Run(ctx, "https://example.com", mockCrawlPages(buildLinearSite()), CrawlOptions{MaxDepth: 3, MaxPages: 10})

	if stats.PagesCrawled > 0 {
		t.Errorf("PagesCrawled = %d after cancellation, expected 0", stats.PagesCrawled)
	}
}

func TestStrategiesTrackBlockedPages(t *testing.T) {
	pages := map[string]*DeepCrawlResult{
		"https://example.com": {
			URL:     "https://example.com",
			Content: "root",
			Blocked: true,
			Links: content.LinkSet{
				Internal: []content.Link{},
				External: []content.Link{},
			},
		},
	}

	strategies := []struct {
		name     string
		strategy CrawlStrategy
	}{
		{"BFS", &BFSStrategy{}},
		{"DFS", &DFSStrategy{}},
		{"BestFirst", &BestFirstStrategy{}},
	}

	for _, st := range strategies {
		t.Run(st.name, func(t *testing.T) {
			_, stats := st.strategy.Run(
				context.Background(),
				"https://example.com",
				mockCrawlPages(pages),
				CrawlOptions{MaxDepth: 0, MaxPages: 10},
			)
			if stats.PagesBlocked != 1 {
				t.Errorf("%s: PagesBlocked = %d, want 1", st.name, stats.PagesBlocked)
			}
		})
	}
}

func TestPriorityQueueOrdering(t *testing.T) {
	pq := &priorityQueue{}

	items := []pqItem{
		{url: "low", score: 0.1},
		{url: "high", score: 0.9},
		{url: "mid", score: 0.5},
	}
	for _, item := range items {
		pq.Push(item)
	}

	if pq.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", pq.Len())
	}

	// Less should prefer higher scores.
	if !pq.Less(1, 0) {
		t.Error("Less should return true when i has higher score than j")
	}
}
