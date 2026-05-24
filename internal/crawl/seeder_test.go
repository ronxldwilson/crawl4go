package crawl

import (
	"testing"
	"time"
)

func TestIsSitemapIndex(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "sitemap index",
			data: `<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://example.com/sitemap1.xml</loc></sitemap></sitemapindex>`,
			want: true,
		},
		{
			name: "urlset",
			data: `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.com/page</loc></url></urlset>`,
			want: false,
		},
		{
			name: "empty data",
			data: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSitemapIndex([]byte(tt.data)); got != tt.want {
				t.Errorf("isSitemapIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseURLSet(t *testing.T) {
	tests := []struct {
		name      string
		xml       string
		source    string
		wantCount int
		wantFirst string
	}{
		{
			name: "basic urlset",
			xml: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2024-01-15</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.8</priority>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
    <priority>0.5</priority>
  </url>
</urlset>`,
			source:    "sitemap",
			wantCount: 2,
			wantFirst: "https://example.com/page1",
		},
		{
			name: "empty urlset",
			xml: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`,
			source:    "sitemap",
			wantCount: 0,
		},
		{
			name: "empty loc skipped",
			xml: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc></loc></url>
  <url><loc>https://example.com/valid</loc></url>
</urlset>`,
			source:    "sitemap",
			wantCount: 1,
		},
		{
			name:      "invalid XML",
			xml:       "not xml at all",
			source:    "sitemap",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := parseURLSet([]byte(tt.xml), tt.source)
			if tt.name == "invalid XML" {
				if err == nil {
					t.Error("expected error for invalid XML")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseURLSet() error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("len(results) = %d, want %d", len(results), tt.wantCount)
				return
			}
			if tt.wantCount > 0 && tt.wantFirst != "" {
				if results[0].URL != tt.wantFirst {
					t.Errorf("results[0].URL = %q, want %q", results[0].URL, tt.wantFirst)
				}
			}
		})
	}
}

func TestParseURLSetMetadata(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page</loc>
    <lastmod>2024-06-15T10:30:00Z</lastmod>
    <changefreq>daily</changefreq>
    <priority>0.9</priority>
  </url>
</urlset>`

	results, err := parseURLSet([]byte(xml), "test-source")
	if err != nil {
		t.Fatalf("parseURLSet() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	su := results[0]
	if su.Source != "test-source" {
		t.Errorf("Source = %q, want %q", su.Source, "test-source")
	}
	if su.ChangeFreq != "daily" {
		t.Errorf("ChangeFreq = %q, want %q", su.ChangeFreq, "daily")
	}
	if su.Priority != 0.9 {
		t.Errorf("Priority = %v, want 0.9", su.Priority)
	}
	if su.LastModified.IsZero() {
		t.Error("LastModified should be parsed, got zero time")
	}
	expectedTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	if !su.LastModified.Equal(expectedTime) {
		t.Errorf("LastModified = %v, want %v", su.LastModified, expectedTime)
	}
}

func TestParseURLSetDateFormats(t *testing.T) {
	tests := []struct {
		name    string
		lastmod string
		wantOK  bool
	}{
		{"RFC3339", "2024-06-15T10:30:00Z", true},
		{"RFC3339 variant", "2024-06-15T10:30:00Z", true},
		{"date only", "2024-06-15", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xml := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page</loc>
    <lastmod>` + tt.lastmod + `</lastmod>
  </url>
</urlset>`
			results, err := parseURLSet([]byte(xml), "sitemap")
			if err != nil {
				t.Fatalf("parseURLSet() error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("len(results) = %d, want 1", len(results))
			}
			if tt.wantOK && results[0].LastModified.IsZero() {
				t.Errorf("expected parsed date for %q, got zero time", tt.lastmod)
			}
		})
	}
}

func TestNewSitemapSeeder(t *testing.T) {
	s := NewSitemapSeeder(nil, 500)
	if s.Client == nil {
		t.Error("Client should not be nil when constructed with nil")
	}
	if s.MaxURLs != 500 {
		t.Errorf("MaxURLs = %d, want 500", s.MaxURLs)
	}
	if s.UserAgent != "crawl4go/1.0" {
		t.Errorf("UserAgent = %q, want %q", s.UserAgent, "crawl4go/1.0")
	}
}

func TestFetchErrorMessage(t *testing.T) {
	e := &fetchError{code: 404, url: "https://example.com/missing"}
	msg := e.Error()
	if msg == "" {
		t.Error("fetchError.Error() should not be empty")
	}
	if !contains(msg, "404") && !contains(msg, "Not Found") {
		t.Errorf("fetchError.Error() = %q, expected to mention 404 or Not Found", msg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
