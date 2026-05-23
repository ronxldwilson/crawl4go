package content

import (
	"net/url"
	"testing"
)

func mustParse(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}

func TestNormalizeURL(t *testing.T) {
	base := mustParse("https://example.com/page")

	tests := []struct {
		name string
		href string
		want string
	}{
		{
			name: "absolute URL unchanged",
			href: "https://example.com/about",
			want: "https://example.com/about",
		},
		{
			name: "relative URL resolved",
			href: "/about",
			want: "https://example.com/about",
		},
		{
			name: "fragment stripped",
			href: "https://example.com/about#section",
			want: "https://example.com/about",
		},
		{
			name: "tracking params removed - utm_source",
			href: "https://example.com/about?utm_source=twitter&real=yes",
			want: "https://example.com/about?real=yes",
		},
		{
			name: "tracking params removed - fbclid",
			href: "https://example.com/page?fbclid=abc123",
			want: "https://example.com/page",
		},
		{
			name: "tracking params removed - gclid",
			href: "https://example.com/page?gclid=xyz&foo=bar",
			want: "https://example.com/page?foo=bar",
		},
		{
			name: "multiple tracking params removed",
			href: "https://example.com/page?utm_source=a&utm_medium=b&utm_campaign=c&keep=1",
			want: "https://example.com/page?keep=1",
		},
		{
			name: "host lowercased",
			href: "https://EXAMPLE.COM/About",
			want: "https://example.com/About",
		},
		{
			name: "trailing slash removed",
			href: "https://example.com/about/",
			want: "https://example.com/about",
		},
		{
			name: "root path keeps slash",
			href: "https://example.com/",
			want: "https://example.com/",
		},
		{
			name: "empty path becomes slash",
			href: "https://example.com",
			want: "https://example.com/",
		},
		{
			name: "whitespace trimmed",
			href: "  https://example.com/about  ",
			want: "https://example.com/about",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.href, base)
			if got != tt.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tt.href, got, tt.want)
			}
		})
	}
}

func TestExtractLinks(t *testing.T) {
	tests := []struct {
		name         string
		html         string
		baseURL      string
		wantInternal int
		wantExternal int
	}{
		{
			name:         "internal link",
			html:         `<html><body><a href="/about">About</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 1,
			wantExternal: 0,
		},
		{
			name:         "external link",
			html:         `<html><body><a href="https://other.com/page">Other</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 0,
			wantExternal: 1,
		},
		{
			name:         "mixed links",
			html:         `<html><body><a href="/about">About</a><a href="https://other.com">Other</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 1,
			wantExternal: 1,
		},
		{
			name:         "skips javascript href",
			html:         `<html><body><a href="javascript:void(0)">Click</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 0,
			wantExternal: 0,
		},
		{
			name:         "skips mailto href",
			html:         `<html><body><a href="mailto:test@example.com">Email</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 0,
			wantExternal: 0,
		},
		{
			name:         "skips empty and hash-only href",
			html:         `<html><body><a href="">Empty</a><a href="#">Hash</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 0,
			wantExternal: 0,
		},
		{
			name:         "deduplicates same URL",
			html:         `<html><body><a href="/about">About</a><a href="/about">About Again</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 1,
			wantExternal: 0,
		},
		{
			name:         "subdomain is internal",
			html:         `<html><body><a href="https://blog.example.com/post">Blog</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 1,
			wantExternal: 0,
		},
		{
			name:         "www prefix treated as same domain",
			html:         `<html><body><a href="https://www.example.com/page">Page</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 1,
			wantExternal: 0,
		},
		{
			name:         "extracts link text",
			html:         `<html><body><a href="/about">About Us</a></body></html>`,
			baseURL:      "https://example.com/",
			wantInternal: 1,
			wantExternal: 0,
		},
		{
			name:         "empty html returns empty slices",
			html:         ``,
			baseURL:      "https://example.com/",
			wantInternal: 0,
			wantExternal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractLinks(tt.html, tt.baseURL)
			if len(got.Internal) != tt.wantInternal {
				t.Errorf("internal links = %d, want %d", len(got.Internal), tt.wantInternal)
			}
			if len(got.External) != tt.wantExternal {
				t.Errorf("external links = %d, want %d", len(got.External), tt.wantExternal)
			}
		})
	}

	// Verify link text extraction.
	t.Run("link text content", func(t *testing.T) {
		html := `<html><body><a href="/about">About Us</a></body></html>`
		got := ExtractLinks(html, "https://example.com/")
		if len(got.Internal) != 1 {
			t.Fatal("expected 1 internal link")
		}
		if got.Internal[0].Text != "About Us" {
			t.Errorf("link text = %q, want %q", got.Internal[0].Text, "About Us")
		}
	})

	// Verify empty html returns non-nil slices.
	t.Run("non-nil slices on empty input", func(t *testing.T) {
		got := ExtractLinks("", "https://example.com/")
		if got.Internal == nil {
			t.Error("Internal should be non-nil empty slice")
		}
		if got.External == nil {
			t.Error("External should be non-nil empty slice")
		}
	})
}
