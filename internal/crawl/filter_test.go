package crawl

import (
	"regexp"
	"testing"
)

func TestURLPatternFilter(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		url      string
		want     bool
	}{
		{
			name:     "empty patterns allows all",
			patterns: nil,
			url:      "https://example.com/anything",
			want:     true,
		},
		{
			name:     "wildcard matches",
			patterns: []string{"*.example.com/*"},
			url:      "https://www.example.com/page",
			want:     true,
		},
		{
			name:     "no match rejects",
			patterns: []string{"*.other.com/*"},
			url:      "https://example.com/page",
			want:     false,
		},
		{
			name:     "question mark matches single char",
			patterns: []string{"https://example.com/pag?"},
			url:      "https://example.com/page",
			want:     true,
		},
		{
			name:     "case insensitive matching",
			patterns: []string{"*EXAMPLE*"},
			url:      "https://example.com/page",
			want:     true,
		},
		{
			name:     "multiple patterns any match passes",
			patterns: []string{"*.other.com/*", "*.example.com/*"},
			url:      "https://www.example.com/page",
			want:     true,
		},
		{
			name:     "literal dot in pattern",
			patterns: []string{"*example.com*"},
			url:      "https://example.com/page",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewURLPatternFilter(tt.patterns)
			got := f.Apply(tt.url)
			if got != tt.want {
				t.Errorf("Apply(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDomainFilter(t *testing.T) {
	tests := []struct {
		name    string
		blocked []string
		allowed []string
		url     string
		want    bool
	}{
		{
			name:    "no filters allows all",
			blocked: nil,
			allowed: nil,
			url:     "https://example.com/page",
			want:    true,
		},
		{
			name:    "blocked domain rejected",
			blocked: []string{"example.com"},
			allowed: nil,
			url:     "https://example.com/page",
			want:    false,
		},
		{
			name:    "blocked subdomain rejected",
			blocked: []string{"example.com"},
			allowed: nil,
			url:     "https://sub.example.com/page",
			want:    false,
		},
		{
			name:    "non-blocked domain allowed",
			blocked: []string{"other.com"},
			allowed: nil,
			url:     "https://example.com/page",
			want:    true,
		},
		{
			name:    "allowed domain accepted",
			blocked: nil,
			allowed: []string{"example.com"},
			url:     "https://example.com/page",
			want:    true,
		},
		{
			name:    "allowed subdomain accepted",
			blocked: nil,
			allowed: []string{"example.com"},
			url:     "https://sub.example.com/page",
			want:    true,
		},
		{
			name:    "non-allowed domain rejected",
			blocked: nil,
			allowed: []string{"example.com"},
			url:     "https://other.com/page",
			want:    false,
		},
		{
			name:    "case insensitive domains",
			blocked: []string{"EXAMPLE.COM"},
			allowed: nil,
			url:     "https://example.com/page",
			want:    false,
		},
		{
			name:    "invalid URL rejected",
			blocked: nil,
			allowed: nil,
			url:     "://bad-url",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDomainFilter(tt.blocked, tt.allowed)
			got := f.Apply(tt.url)
			if got != tt.want {
				t.Errorf("Apply(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestContentTypeFilter(t *testing.T) {
	tests := []struct {
		name       string
		extensions []string
		url        string
		want       bool
	}{
		{
			name:       "empty extensions allows all",
			extensions: nil,
			url:        "https://example.com/page.html",
			want:       true,
		},
		{
			name:       "matching extension allowed",
			extensions: []string{".html", ".htm"},
			url:        "https://example.com/page.html",
			want:       true,
		},
		{
			name:       "non-matching extension blocked",
			extensions: []string{".html"},
			url:        "https://example.com/image.png",
			want:       false,
		},
		{
			name:       "no extension checked against empty string",
			extensions: []string{""},
			url:        "https://example.com/page",
			want:       true,
		},
		{
			name:       "dot prefix added automatically",
			extensions: []string{"html", "htm"},
			url:        "https://example.com/page.html",
			want:       true,
		},
		{
			name:       "case insensitive",
			extensions: []string{".HTML"},
			url:        "https://example.com/page.html",
			want:       true,
		},
		{
			name:       "no extension and not in allowed list",
			extensions: []string{".html"},
			url:        "https://example.com/page",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewContentTypeFilter(tt.extensions)
			got := f.Apply(tt.url)
			if got != tt.want {
				t.Errorf("Apply(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestFilterChain(t *testing.T) {
	tests := []struct {
		name    string
		filters []URLFilter
		url     string
		want    bool
	}{
		{
			name:    "empty chain allows all",
			filters: nil,
			url:     "https://example.com/page",
			want:    true,
		},
		{
			name: "all filters pass",
			filters: []URLFilter{
				NewURLPatternFilter([]string{"*example.com*"}),
				NewDomainFilter(nil, []string{"example.com"}),
			},
			url:  "https://example.com/page",
			want: true,
		},
		{
			name: "one filter fails rejects",
			filters: []URLFilter{
				NewURLPatternFilter([]string{"*example.com*"}),
				NewDomainFilter([]string{"example.com"}, nil),
			},
			url:  "https://example.com/page",
			want: false,
		},
		{
			name: "first filter fails short-circuits",
			filters: []URLFilter{
				NewDomainFilter([]string{"example.com"}, nil),
				NewURLPatternFilter([]string{"*example.com*"}),
			},
			url:  "https://example.com/page",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FilterChain{Filters: tt.filters}
			got := fc.Apply(tt.url)
			if got != tt.want {
				t.Errorf("Apply(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*.com", "example.com", true},
		{"*.com", "example.org", false},
		{"test?", "test1", true},
		{"test?", "test12", false},
		{"hello.world", "hello.world", true},
		{"hello.world", "helloXworld", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			re := globToRegex(tt.pattern)
			// globToRegex produces a partial match pattern, so anchor it for this test.
			re = "^" + re + "$"
			r, err := regexp.Compile(re)
			if err != nil {
				t.Fatalf("failed to compile regex %q: %v", re, err)
			}
			matched := r.MatchString(tt.input)
			if matched != tt.want {
				t.Errorf("globToRegex(%q) match %q = %v, want %v", tt.pattern, tt.input, matched, tt.want)
			}
		})
	}
}
