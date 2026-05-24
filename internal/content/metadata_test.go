package content

import (
	"encoding/json"
	"testing"
)

func TestExtractMetadata(t *testing.T) {
	tests := []struct {
		name string
		html string
		check func(t *testing.T, m *PageMetadata)
	}{
		{
			name: "title",
			html: `<html><head><title>My Page</title></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Title != "My Page" {
					t.Errorf("Title = %q, want %q", m.Title, "My Page")
				}
			},
		},
		{
			name: "meta description",
			html: `<html><head><meta name="description" content="A great page"></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Description != "A great page" {
					t.Errorf("Description = %q, want %q", m.Description, "A great page")
				}
			},
		},
		{
			name: "meta keywords",
			html: `<html><head><meta name="keywords" content="go,testing,code"></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Keywords != "go,testing,code" {
					t.Errorf("Keywords = %q, want %q", m.Keywords, "go,testing,code")
				}
			},
		},
		{
			name: "meta author",
			html: `<html><head><meta name="author" content="Jane Doe"></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Author != "Jane Doe" {
					t.Errorf("Author = %q, want %q", m.Author, "Jane Doe")
				}
			},
		},
		{
			name: "html lang attribute",
			html: `<html lang="en"><head></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Language != "en" {
					t.Errorf("Language = %q, want %q", m.Language, "en")
				}
			},
		},
		{
			name: "opengraph properties",
			html: `<html><head>
				<meta property="og:title" content="OG Title">
				<meta property="og:description" content="OG Desc">
				<meta property="og:image" content="https://example.com/og.jpg">
			</head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.OpenGraph == nil {
					t.Fatal("OpenGraph is nil")
				}
				if m.OpenGraph["title"] != "OG Title" {
					t.Errorf("og:title = %q, want %q", m.OpenGraph["title"], "OG Title")
				}
				if m.OpenGraph["description"] != "OG Desc" {
					t.Errorf("og:description = %q, want %q", m.OpenGraph["description"], "OG Desc")
				}
				if m.OpenGraph["image"] != "https://example.com/og.jpg" {
					t.Errorf("og:image = %q", m.OpenGraph["image"])
				}
			},
		},
		{
			name: "twitter card via property",
			html: `<html><head>
				<meta property="twitter:card" content="summary_large_image">
				<meta property="twitter:title" content="Twitter Title">
			</head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.TwitterCard == nil {
					t.Fatal("TwitterCard is nil")
				}
				if m.TwitterCard["card"] != "summary_large_image" {
					t.Errorf("twitter:card = %q", m.TwitterCard["card"])
				}
				if m.TwitterCard["title"] != "Twitter Title" {
					t.Errorf("twitter:title = %q", m.TwitterCard["title"])
				}
			},
		},
		{
			name: "twitter card via name",
			html: `<html><head>
				<meta name="twitter:site" content="@example">
			</head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.TwitterCard == nil {
					t.Fatal("TwitterCard is nil")
				}
				if m.TwitterCard["site"] != "@example" {
					t.Errorf("twitter:site = %q", m.TwitterCard["site"])
				}
			},
		},
		{
			name: "canonical link",
			html: `<html><head><link rel="canonical" href="https://example.com/page"></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Canonical != "https://example.com/page" {
					t.Errorf("Canonical = %q", m.Canonical)
				}
			},
		},
		{
			name: "json-ld",
			html: `<html><head><script type="application/ld+json">{"@type":"Article","name":"Test"}</script></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if len(m.JSONLD) != 1 {
					t.Fatalf("JSONLD count = %d, want 1", len(m.JSONLD))
				}
				var obj map[string]interface{}
				if err := json.Unmarshal(m.JSONLD[0], &obj); err != nil {
					t.Fatalf("invalid JSON-LD: %v", err)
				}
				if obj["@type"] != "Article" {
					t.Errorf("@type = %v, want Article", obj["@type"])
				}
			},
		},
		{
			name: "invalid json-ld ignored",
			html: `<html><head><script type="application/ld+json">{not valid json</script></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if len(m.JSONLD) != 0 {
					t.Errorf("JSONLD count = %d, want 0 for invalid JSON", len(m.JSONLD))
				}
			},
		},
		{
			name: "title fallback to og:title",
			html: `<html><head><meta property="og:title" content="Fallback Title"></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Title != "Fallback Title" {
					t.Errorf("Title = %q, want %q", m.Title, "Fallback Title")
				}
			},
		},
		{
			name: "description fallback to og:description",
			html: `<html><head><meta property="og:description" content="Fallback Desc"></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.Description != "Fallback Desc" {
					t.Errorf("Description = %q, want %q", m.Description, "Fallback Desc")
				}
			},
		},
		{
			name: "empty maps nilled out",
			html: `<html><head><title>Plain</title></head><body></body></html>`,
			check: func(t *testing.T, m *PageMetadata) {
				if m.OpenGraph != nil {
					t.Error("OpenGraph should be nil when empty")
				}
				if m.TwitterCard != nil {
					t.Error("TwitterCard should be nil when empty")
				}
			},
		},
		{
			name: "empty html",
			html: ``,
			check: func(t *testing.T, m *PageMetadata) {
				if m == nil {
					t.Fatal("metadata should not be nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ExtractMetadata(tt.html)
			tt.check(t, m)
		})
	}
}
