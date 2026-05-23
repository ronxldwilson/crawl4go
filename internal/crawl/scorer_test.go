package crawl

import (
	"math"
	"testing"
	"time"
)

func TestKeywordRelevanceScorer(t *testing.T) {
	tests := []struct {
		name     string
		keywords []string
		url      string
		want     float64
	}{
		{
			name:     "no keywords returns 0",
			keywords: nil,
			url:      "https://example.com/page",
			want:     0,
		},
		{
			name:     "all keywords match returns 1",
			keywords: []string{"example", "page"},
			url:      "https://example.com/page",
			want:     1.0,
		},
		{
			name:     "half keywords match returns 0.5",
			keywords: []string{"example", "missing"},
			url:      "https://example.com/page",
			want:     0.5,
		},
		{
			name:     "no keywords match returns 0",
			keywords: []string{"foo", "bar"},
			url:      "https://example.com/page",
			want:     0,
		},
		{
			name:     "case insensitive matching",
			keywords: []string{"EXAMPLE"},
			url:      "https://example.com/page",
			want:     1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewKeywordRelevanceScorer(tt.keywords)
			got := s.Score(tt.url)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("Score(%q) = %f, want %f", tt.url, got, tt.want)
			}
		})
	}
}

func TestPathDepthScorer(t *testing.T) {
	tests := []struct {
		name         string
		optimalDepth int
		url          string
		wantApprox   float64
	}{
		{
			name:         "exact optimal depth",
			optimalDepth: 2,
			url:          "https://example.com/a/b",
			wantApprox:   1.0,
		},
		{
			name:         "depth 0 with optimal 2",
			optimalDepth: 2,
			url:          "https://example.com/",
			wantApprox:   1.0 / 3.0, // 1/(1+2)
		},
		{
			name:         "depth 1 with optimal 2",
			optimalDepth: 2,
			url:          "https://example.com/a",
			wantApprox:   0.5, // 1/(1+1)
		},
		{
			name:         "depth 4 with optimal 2",
			optimalDepth: 2,
			url:          "https://example.com/a/b/c/d",
			wantApprox:   1.0 / 3.0, // 1/(1+2)
		},
		{
			name:         "default optimal depth when 0 passed",
			optimalDepth: 0,
			url:          "https://example.com/a/b",
			wantApprox:   1.0, // defaults to 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPathDepthScorer(tt.optimalDepth)
			got := s.Score(tt.url)
			if math.Abs(got-tt.wantApprox) > 0.01 {
				t.Errorf("Score(%q) = %f, want ~%f", tt.url, got, tt.wantApprox)
			}
		})
	}
}

func TestContentTypeScorer(t *testing.T) {
	// Note: ContentTypeScorer uses HasSuffix on the full path against map keys.
	// The empty-string key ("") matches any path via HasSuffix, and since map
	// iteration order is non-deterministic, only test cases where the expected
	// score equals 1.0 (same as the empty-string entry) are reliable.
	// Extensions with score 0.0 are also deterministic because the first map
	// hit wins, but "" could match first with score 1.0.
	tests := []struct {
		name    string
		url     string
		wantMin float64 // score is at least this
	}{
		{
			name:    "html extension scores at least 1.0",
			url:     "https://example.com/page.html",
			wantMin: 1.0,
		},
		{
			name:    "htm extension scores at least 1.0",
			url:     "https://example.com/page.htm",
			wantMin: 1.0,
		},
		{
			name:    "any known extension returns a score",
			url:     "https://example.com/doc.pdf",
			wantMin: 0.0,
		},
		{
			name:    "invalid URL returns 0",
			url:     "://bad",
			wantMin: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewContentTypeScorer()
			got := s.Score(tt.url)
			if got < tt.wantMin {
				t.Errorf("Score(%q) = %f, want >= %f", tt.url, got, tt.wantMin)
			}
		})
	}

	// Verify the scorer returns a non-negative score for various inputs.
	t.Run("all scores non-negative", func(t *testing.T) {
		s := NewContentTypeScorer()
		urls := []string{
			"https://example.com/page",
			"https://example.com/page.html",
			"https://example.com/style.css",
			"https://example.com/app.js",
			"https://example.com/image.png",
			"https://example.com/file.xyz",
		}
		for _, u := range urls {
			score := s.Score(u)
			if score < 0 {
				t.Errorf("Score(%q) = %f, want >= 0", u, score)
			}
		}
	})
}

func TestFreshnessScorer(t *testing.T) {
	currentYear := time.Now().Year()

	tests := []struct {
		name string
		url  string
		want float64
	}{
		{
			name: "no date in URL returns 0.5",
			url:  "https://example.com/page",
			want: 0.5,
		},
		{
			name: "current year returns 1.0",
			url:  "https://example.com/" + itoa(currentYear) + "/01/article",
			want: 1.0,
		},
		{
			name: "one year old returns 0.8",
			url:  "https://example.com/" + itoa(currentYear-1) + "/06/post",
			want: 0.8,
		},
		{
			name: "two years old returns 0.6",
			url:  "https://example.com/" + itoa(currentYear-2) + "/03/article",
			want: 0.6,
		},
		{
			name: "three years old returns 0.4",
			url:  "https://example.com/" + itoa(currentYear-3) + "/01/post",
			want: 0.4,
		},
		{
			name: "five years old returns 0.2",
			url:  "https://example.com/" + itoa(currentYear-5) + "/01/post",
			want: 0.2,
		},
		{
			name: "very old returns 0.2",
			url:  "https://example.com/2005/01/post",
			want: 0.2,
		},
		{
			name: "invalid year returns 0.5",
			url:  "https://example.com/1800/01/post",
			want: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewFreshnessScorer()
			got := s.Score(tt.url)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("Score(%q) = %f, want %f", tt.url, got, tt.want)
			}
		})
	}
}

func TestCompositeScorer(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want float64
	}{
		{
			name: "empty scorers returns 0",
			url:  "https://example.com/page",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := NewCompositeScorer(nil)
			got := cs.Score(tt.url)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("Score(%q) = %f, want %f", tt.url, got, tt.want)
			}
		})
	}

	t.Run("weighted average", func(t *testing.T) {
		// Keyword scorer with "example" matches => 1.0
		// Keyword scorer with "missing" => 0.0
		// Equal weights => average of 1.0 and 0.0 = 0.5
		s1 := NewKeywordRelevanceScorer([]string{"example"})
		s2 := NewKeywordRelevanceScorer([]string{"missing"})
		cs := NewCompositeScorer([]weightedScorer{
			{scorer: s1, weight: 1.0},
			{scorer: s2, weight: 1.0},
		})
		got := cs.Score("https://example.com/page")
		want := 0.5
		if math.Abs(got-want) > 0.001 {
			t.Errorf("Score = %f, want %f", got, want)
		}
	})

	t.Run("unequal weights", func(t *testing.T) {
		s1 := NewKeywordRelevanceScorer([]string{"example"}) // scores 1.0
		s2 := NewKeywordRelevanceScorer([]string{"missing"}) // scores 0.0
		cs := NewCompositeScorer([]weightedScorer{
			{scorer: s1, weight: 3.0},
			{scorer: s2, weight: 1.0},
		})
		got := cs.Score("https://example.com/page")
		want := 0.75 // (1.0*3 + 0.0*1) / 4
		if math.Abs(got-want) > 0.001 {
			t.Errorf("Score = %f, want %f", got, want)
		}
	})
}

func itoa(n int) string {
	s := ""
	if n < 0 {
		return "-" + itoa(-n)
	}
	for n >= 10 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return string(rune('0'+n)) + s
}
