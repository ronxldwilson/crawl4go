package content

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractPreviewMetadata(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		wantTitle   string
		wantDesc    string
		wantImage   string
		wantSite    string
		wantType    string
	}{
		{
			name: "OpenGraph tags",
			html: `<html><head>
				<meta property="og:title" content="OG Title">
				<meta property="og:description" content="OG Desc">
				<meta property="og:image" content="https://img.example.com/og.png">
				<meta property="og:site_name" content="Example Site">
				<meta property="og:type" content="article">
				<title>Fallback Title</title>
			</head><body></body></html>`,
			wantTitle: "OG Title",
			wantDesc:  "OG Desc",
			wantImage: "https://img.example.com/og.png",
			wantSite:  "Example Site",
			wantType:  "article",
		},
		{
			name: "meta name fallbacks",
			html: `<html><head>
				<meta name="title" content="Meta Title">
				<meta name="description" content="Meta Desc">
				<title>Title Element</title>
			</head><body></body></html>`,
			wantTitle: "Meta Title",
			wantDesc:  "Meta Desc",
			wantImage: "",
			wantSite:  "",
			wantType:  "",
		},
		{
			name: "title element fallback",
			html: `<html><head>
				<title>Only Title</title>
			</head><body></body></html>`,
			wantTitle: "Only Title",
			wantDesc:  "",
			wantImage: "",
			wantSite:  "",
			wantType:  "",
		},
		{
			name: "OG takes priority over meta and title",
			html: `<html><head>
				<meta property="og:title" content="OG Wins">
				<meta name="title" content="Meta Loses">
				<title>Title Loses</title>
				<meta property="og:description" content="OG Desc Wins">
				<meta name="description" content="Meta Desc Loses">
			</head><body></body></html>`,
			wantTitle: "OG Wins",
			wantDesc:  "OG Desc Wins",
		},
		{
			name: "image from img tag fallback",
			html: `<html><head></head><body>
				<img src="https://img.example.com/photo.jpg">
			</body></html>`,
			wantTitle: "",
			wantImage: "https://img.example.com/photo.jpg",
		},
		{
			name: "image upgraded by dimensions",
			html: `<html><head></head><body>
				<img src="https://img.example.com/small.jpg">
				<img src="https://img.example.com/large.jpg" width="200" height="200">
			</body></html>`,
			wantImage: "https://img.example.com/large.jpg",
		},
		{
			name: "OG image takes priority over img tag",
			html: `<html><head>
				<meta property="og:image" content="https://img.example.com/og.png">
			</head><body>
				<img src="https://img.example.com/body.jpg" width="200" height="200">
			</body></html>`,
			wantImage: "https://img.example.com/og.png",
		},
		{
			name:      "empty document",
			html:      "<html><head></head><body></body></html>",
			wantTitle: "",
			wantDesc:  "",
			wantImage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseHTML(tt.html)
			if err != nil {
				t.Fatalf("failed to parse HTML: %v", err)
			}

			preview := &LinkPreview{}
			extractMetadata(doc, preview)

			if tt.wantTitle != "" && preview.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", preview.Title, tt.wantTitle)
			}
			if tt.wantDesc != "" && preview.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", preview.Description, tt.wantDesc)
			}
			if tt.wantImage != "" && preview.ImageURL != tt.wantImage {
				t.Errorf("ImageURL = %q, want %q", preview.ImageURL, tt.wantImage)
			}
			if tt.wantSite != "" && preview.SiteName != tt.wantSite {
				t.Errorf("SiteName = %q, want %q", preview.SiteName, tt.wantSite)
			}
			if tt.wantType != "" && preview.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", preview.Type, tt.wantType)
			}
		})
	}
}

func TestFetchLinkPreview_WithTestServer(t *testing.T) {
	ogHTML := `<html><head>
		<meta property="og:title" content="Test Page">
		<meta property="og:description" content="A test description">
		<title>Fallback</title>
	</head><body><p>Content</p></body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(ogHTML)))
		if r.Method == http.MethodHead {
			return
		}
		w.Write([]byte(ogHTML))
	}))
	defer server.Close()

	ctx := context.Background()
	preview, err := FetchLinkPreview(ctx, server.URL, server.Client())
	if err != nil {
		t.Fatalf("FetchLinkPreview() error: %v", err)
	}

	if preview.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", preview.Title, "Test Page")
	}
	if preview.Description != "A test description" {
		t.Errorf("Description = %q, want %q", preview.Description, "A test description")
	}
	if preview.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", preview.StatusCode)
	}
	if !strings.Contains(preview.ContentType, "text/html") {
		t.Errorf("ContentType = %q, want to contain 'text/html'", preview.ContentType)
	}
}

func TestFetchLinkPreview_NonHTMLContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"key": "value"}`))
	}))
	defer server.Close()

	ctx := context.Background()
	preview, err := FetchLinkPreview(ctx, server.URL, server.Client())
	if err != nil {
		t.Fatalf("FetchLinkPreview() error: %v", err)
	}

	// Non-HTML: should have status and content type but no parsed metadata.
	if preview.Title != "" {
		t.Errorf("Title = %q, want empty for non-HTML", preview.Title)
	}
	if !strings.Contains(preview.ContentType, "application/json") {
		t.Errorf("ContentType = %q, want 'application/json'", preview.ContentType)
	}
}

func TestFetchLinkPreviews_Concurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Batch</title></head><body></body></html>`))
	}))
	defer server.Close()

	urls := []string{server.URL + "/a", server.URL + "/b", server.URL + "/c"}
	ctx := context.Background()
	results := FetchLinkPreviews(ctx, urls, server.Client(), 2)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	for i, r := range results {
		if r == nil {
			t.Errorf("results[%d] is nil", i)
			continue
		}
		if r.URL != urls[i] {
			t.Errorf("results[%d].URL = %q, want %q", i, r.URL, urls[i])
		}
		if r.Title != "Batch" {
			t.Errorf("results[%d].Title = %q, want 'Batch'", i, r.Title)
		}
	}
}

func TestFetchLinkPreviews_InvalidURL(t *testing.T) {
	urls := []string{"http://this-host-does-not-exist.invalid/page"}
	ctx := context.Background()
	results := FetchLinkPreviews(ctx, urls, http.DefaultClient, 1)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	// Should get a stub with URL set even on error.
	if results[0].URL != urls[0] {
		t.Errorf("URL = %q, want %q", results[0].URL, urls[0])
	}
}

