package crawl

import (
	"context"
	"testing"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

func TestDiscoverLinksBasic(t *testing.T) {
	result := DeepCrawlResult{
		URL: "https://example.com",
		Links: content.LinkSet{
			Internal: []content.Link{
				{Href: "https://example.com/page1", Text: "Page 1"},
				{Href: "https://example.com/page2", Text: "Page 2"},
			},
			External: []content.Link{
				{Href: "https://other.com/ext", Text: "External"},
			},
		},
	}

	visited := make(map[string]bool)
	visited["https://example.com"] = true
	depths := make(map[string]int)

	opts := CrawlOptions{MaxDepth: 3, MaxPages: 10}

	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	if len(links) != 2 {
		t.Errorf("len(links) = %d, want 2 (internal only)", len(links))
	}

	// Verify visited map was updated.
	for _, link := range links {
		if !visited[link] {
			t.Errorf("link %q should be marked as visited", link)
		}
		if depths[link] != 1 {
			t.Errorf("depth of %q = %d, want 1", link, depths[link])
		}
	}
}

func TestDiscoverLinksIncludeExternal(t *testing.T) {
	result := DeepCrawlResult{
		URL: "https://example.com",
		Links: content.LinkSet{
			Internal: []content.Link{
				{Href: "https://example.com/page1", Text: "Page 1"},
			},
			External: []content.Link{
				{Href: "https://other.com/ext", Text: "External"},
			},
		},
	}

	visited := make(map[string]bool)
	depths := make(map[string]int)

	opts := CrawlOptions{MaxDepth: 3, MaxPages: 10, IncludeExternal: true}

	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	if len(links) != 2 {
		t.Errorf("len(links) = %d, want 2 (internal + external)", len(links))
	}
}

func TestDiscoverLinksSkipsVisited(t *testing.T) {
	result := DeepCrawlResult{
		URL: "https://example.com",
		Links: content.LinkSet{
			Internal: []content.Link{
				{Href: "https://example.com/already-seen", Text: "Already"},
				{Href: "https://example.com/new", Text: "New"},
			},
			External: []content.Link{},
		},
	}

	visited := map[string]bool{
		"https://example.com":              true,
		"https://example.com/already-seen": true,
	}
	depths := make(map[string]int)

	opts := CrawlOptions{MaxDepth: 3, MaxPages: 10}

	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	if len(links) != 1 {
		t.Errorf("len(links) = %d, want 1 (only new link)", len(links))
	}
	if len(links) > 0 && links[0] != "https://example.com/new" {
		t.Errorf("links[0] = %q, want %q", links[0], "https://example.com/new")
	}
}

func TestDiscoverLinksWithFilter(t *testing.T) {
	result := DeepCrawlResult{
		URL: "https://example.com",
		Links: content.LinkSet{
			Internal: []content.Link{
				{Href: "https://example.com/blog/post1", Text: "Blog Post"},
				{Href: "https://example.com/admin/secret", Text: "Admin"},
			},
			External: []content.Link{},
		},
	}

	visited := make(map[string]bool)
	depths := make(map[string]int)

	// Only allow URLs matching *blog*.
	filter := NewURLPatternFilter([]string{"*blog*"})
	opts := CrawlOptions{
		MaxDepth: 3,
		MaxPages: 10,
		Filters:  &FilterChain{Filters: []URLFilter{filter}},
	}

	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	if len(links) != 1 {
		t.Errorf("len(links) = %d, want 1 (only blog link)", len(links))
	}
	if len(links) > 0 && links[0] != "https://example.com/blog/post1" {
		t.Errorf("links[0] = %q, want blog link", links[0])
	}
}

func TestDiscoverLinksWithScorer(t *testing.T) {
	result := DeepCrawlResult{
		URL: "https://example.com",
		Links: content.LinkSet{
			Internal: []content.Link{
				{Href: "https://example.com/go-programming", Text: "Go"},
				{Href: "https://example.com/random-stuff", Text: "Random"},
			},
			External: []content.Link{},
		},
	}

	visited := make(map[string]bool)
	depths := make(map[string]int)

	scorer := NewKeywordRelevanceScorer([]string{"go", "programming"})
	opts := CrawlOptions{
		MaxDepth:       3,
		MaxPages:       10,
		Scorer:         scorer,
		ScoreThreshold: 0.5, // Require at least half the keywords to match.
	}

	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	// Only the URL with "go" and "programming" in it should pass the threshold.
	if len(links) != 1 {
		t.Errorf("len(links) = %d, want 1 (only high-scoring link)", len(links))
	}
	if len(links) > 0 && links[0] != "https://example.com/go-programming" {
		t.Errorf("links[0] = %q, want go-programming link", links[0])
	}
}

func TestDiscoverLinksEmptyResult(t *testing.T) {
	result := DeepCrawlResult{
		URL: "https://example.com",
		Links: content.LinkSet{
			Internal: []content.Link{},
			External: []content.Link{},
		},
	}

	visited := make(map[string]bool)
	depths := make(map[string]int)

	opts := CrawlOptions{MaxDepth: 3, MaxPages: 10}
	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	if len(links) != 0 {
		t.Errorf("len(links) = %d, want 0 for empty links", len(links))
	}
}

func TestDiscoverLinksNilLinks(t *testing.T) {
	result := DeepCrawlResult{
		URL:   "https://example.com",
		Links: content.LinkSet{},
	}

	visited := make(map[string]bool)
	depths := make(map[string]int)

	opts := CrawlOptions{MaxDepth: 3, MaxPages: 10}
	links := discoverLinks(context.Background(), result, visited, depths, 1, opts)

	if len(links) != 0 {
		t.Errorf("len(links) = %d, want 0 for nil internal links", len(links))
	}
}
