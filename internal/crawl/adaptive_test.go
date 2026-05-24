package crawl

import (
	"context"
	"fmt"
	"testing"
)

func TestTokenizeContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]int
	}{
		{
			name:  "empty string",
			input: "",
			want:  map[string]int{},
		},
		{
			name:  "single char tokens are skipped",
			input: "a b c",
			want:  map[string]int{},
		},
		{
			name:  "simple words",
			input: "hello world hello",
			want:  map[string]int{"hello": 2, "world": 1},
		},
		{
			name:  "mixed case lowered",
			input: "Hello WORLD",
			want:  map[string]int{"hello": 1, "world": 1},
		},
		{
			name:  "punctuation splits tokens",
			input: "foo-bar, baz.qux",
			want:  map[string]int{"foo": 1, "bar": 1, "baz": 1, "qux": 1},
		},
		{
			name:  "digits are kept",
			input: "page123 test42",
			want:  map[string]int{"page123": 1, "test42": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeContent(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("tokenizeContent(%q) returned %d tokens, want %d: got=%v", tt.input, len(got), len(tt.want), got)
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("tokenizeContent(%q)[%q] = %d, want %d", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestNewAdaptiveStrategy(t *testing.T) {
	terms := []string{"Go", "Crawl"}
	s := NewAdaptiveStrategy(terms)

	if s.ConfidenceThreshold != 0.85 {
		t.Errorf("ConfidenceThreshold = %v, want 0.85", s.ConfidenceThreshold)
	}
	if s.MinPages != 5 {
		t.Errorf("MinPages = %d, want 5", s.MinPages)
	}
	// QueryTerms should be lowercased.
	for i, qt := range s.QueryTerms {
		if qt != []string{"go", "crawl"}[i] {
			t.Errorf("QueryTerms[%d] = %q, want lowercased", i, qt)
		}
	}
}

func TestKnowledgeBaseUpdate(t *testing.T) {
	kb := newKnowledgeBase()

	kb.update(map[string]int{"go": 3, "web": 1})
	kb.update(map[string]int{"go": 2, "crawl": 1})

	if kb.docCount != 2 {
		t.Errorf("docCount = %d, want 2", kb.docCount)
	}
	if kb.termFreqs["go"] != 5 {
		t.Errorf("termFreqs[go] = %d, want 5", kb.termFreqs["go"])
	}
	if kb.docFreqs["go"] != 2 {
		t.Errorf("docFreqs[go] = %d, want 2", kb.docFreqs["go"])
	}
	if kb.docFreqs["crawl"] != 1 {
		t.Errorf("docFreqs[crawl] = %d, want 1", kb.docFreqs["crawl"])
	}
	if len(kb.uniqueTermsPerDoc) != 2 {
		t.Errorf("uniqueTermsPerDoc length = %d, want 2", len(kb.uniqueTermsPerDoc))
	}
}

func TestComputeCoverage(t *testing.T) {
	tests := []struct {
		name       string
		queryTerms []string
		termFreqs  map[string]int
		want       float64
	}{
		{
			name:       "no query terms returns 1.0",
			queryTerms: nil,
			termFreqs:  map[string]int{},
			want:       1.0,
		},
		{
			name:       "all terms found",
			queryTerms: []string{"go", "web"},
			termFreqs:  map[string]int{"go": 5, "web": 3, "extra": 1},
			want:       1.0,
		},
		{
			name:       "half terms found",
			queryTerms: []string{"go", "missing"},
			termFreqs:  map[string]int{"go": 5},
			want:       0.5,
		},
		{
			name:       "no terms found",
			queryTerms: []string{"missing"},
			termFreqs:  map[string]int{"go": 5},
			want:       0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &AdaptiveStrategy{QueryTerms: tt.queryTerms}
			kb := &knowledgeBase{termFreqs: tt.termFreqs}
			got := s.computeCoverage(kb)
			if got != tt.want {
				t.Errorf("computeCoverage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeConsistency(t *testing.T) {
	s := &AdaptiveStrategy{}

	// Empty KB should return 0.
	kb := &knowledgeBase{termFreqs: map[string]int{}}
	if got := s.computeConsistency(kb); got != 0 {
		t.Errorf("computeConsistency(empty) = %v, want 0", got)
	}

	// Uniform frequencies should give high consistency.
	kb = &knowledgeBase{
		docCount:  10,
		termFreqs: map[string]int{"go": 10, "web": 10, "crawl": 10},
	}
	got := s.computeConsistency(kb)
	if got < 0.9 {
		t.Errorf("computeConsistency(uniform) = %v, want >= 0.9", got)
	}
}

func TestComputeSaturation(t *testing.T) {
	s := &AdaptiveStrategy{}

	// Empty KB returns 0.
	kb := &knowledgeBase{}
	if got := s.computeSaturation(kb); got != 0 {
		t.Errorf("computeSaturation(empty) = %v, want 0", got)
	}

	// When recent docs have fewer unique terms than average, saturation is positive.
	kb = &knowledgeBase{
		uniqueTermsPerDoc: []int{100, 80, 60, 40, 20, 10},
	}
	got := s.computeSaturation(kb)
	if got <= 0 {
		t.Errorf("computeSaturation(decreasing) = %v, want > 0", got)
	}

	// When recent docs have same terms as average, saturation is ~0.
	kb = &knowledgeBase{
		uniqueTermsPerDoc: []int{50, 50, 50, 50},
	}
	got = s.computeSaturation(kb)
	if got != 0 {
		t.Errorf("computeSaturation(flat) = %v, want 0", got)
	}
}

func TestConverged(t *testing.T) {
	s := &AdaptiveStrategy{
		ConfidenceThreshold: 0.5,
		CoverageWeight:      0.4,
		ConsistencyWeight:   0.3,
		SaturationWeight:    0.3,
		QueryTerms:          []string{"go"},
	}

	// KB where "go" is found, consistency is high, saturation is moderate.
	kb := &knowledgeBase{
		docCount:          10,
		termFreqs:         map[string]int{"go": 10, "web": 10},
		docFreqs:          map[string]int{"go": 10, "web": 10},
		uniqueTermsPerDoc: []int{100, 80, 60, 40, 20, 10, 5, 3, 2, 1},
	}

	if !s.converged(kb) {
		t.Error("expected converged=true for high coverage+consistency+saturation KB")
	}

	// KB where "go" is NOT found.
	s2 := &AdaptiveStrategy{
		ConfidenceThreshold: 0.99,
		CoverageWeight:      1.0,
		ConsistencyWeight:   0.0,
		SaturationWeight:    0.0,
		QueryTerms:          []string{"missing"},
	}
	if s2.converged(kb) {
		t.Error("expected converged=false when query term is not found")
	}
}

func TestAdaptiveScoreURL(t *testing.T) {
	s := NewAdaptiveStrategy([]string{"blog", "post"})
	kb := newKnowledgeBase()

	opts := CrawlOptions{MaxDepth: 3, MaxPages: 10}

	// URL with keywords in path should score higher than one without.
	withKeywords := s.scoreURL("https://example.com/blog/post/123", 0, kb, opts)
	withoutKeywords := s.scoreURL("https://example.com/about/contact", 0, kb, opts)

	if withKeywords <= withoutKeywords {
		t.Errorf("URL with keywords scored %v, without scored %v; expected higher with keywords", withKeywords, withoutKeywords)
	}

	// Deeper pages should have a depth penalty.
	shallow := s.scoreURL("https://example.com/blog", 0, kb, opts)
	deep := s.scoreURL("https://example.com/blog", 5, kb, opts)
	if deep >= shallow {
		t.Errorf("deep score %v >= shallow score %v; expected depth penalty", deep, shallow)
	}
}

func TestAdaptiveStrategyRun(t *testing.T) {
	pages := map[string]*DeepCrawlResult{
		"https://example.com": {
			URL:     "https://example.com",
			Content: "go web crawl programming",
		},
		"https://example.com/page1": {
			URL:     "https://example.com/page1",
			Content: "go web crawl programming",
		},
	}

	crawlFn := func(ctx context.Context, pageURL string) (*DeepCrawlResult, error) {
		if r, ok := pages[pageURL]; ok {
			return r, nil
		}
		return nil, fmt.Errorf("not found")
	}

	s := NewAdaptiveStrategy([]string{"go", "crawl"})
	s.MinPages = 1

	opts := CrawlOptions{MaxDepth: 2, MaxPages: 5}
	results, stats := s.Run(context.Background(), "https://example.com", crawlFn, opts)

	if stats.PagesCrawled == 0 {
		t.Error("expected at least one page crawled")
	}
	if len(results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestAdaptivePriorityQueue(t *testing.T) {
	pq := &adaptivePriorityQueue{}

	items := []adaptivePQItem{
		{url: "low", score: 0.1},
		{url: "high", score: 0.9},
		{url: "mid", score: 0.5},
	}

	for _, item := range items {
		*pq = append(*pq, item)
	}

	// After sorting, highest score should be first (max-heap).
	if pq.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", pq.Len())
	}

	// Less returns true when i has higher score than j.
	if !pq.Less(1, 0) {
		t.Error("expected item with score 0.9 > item with score 0.1")
	}
}
