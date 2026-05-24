package content

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns an *http.Client that uses the provided transport (or
// http.DefaultTransport when nil).
func newTestClient(transport http.RoundTripper) *http.Client {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{Transport: transport}
}

func TestFetchLinkPreview_NonHTMLContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(p.ContentType, "application/pdf") {
		t.Errorf("ContentType = %q, want application/pdf", p.ContentType)
	}
	if p.Title != "" {
		t.Errorf("Title should be empty for non-HTML, got %q", p.Title)
	}
	if p.ContentLength != 1024 {
		t.Errorf("ContentLength = %d, want 1024", p.ContentLength)
	}
}

func TestFetchLinkPreview_HTMLGetFallbackTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<html><head><title>My Page</title></head><body></body></html>`)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Title != "My Page" {
		t.Errorf("Title = %q, want %q", p.Title, "My Page")
	}
}

func TestFetchLinkPreview_OGTitleOverridesTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><head>
			<title>Plain Title</title>
			<meta property="og:title" content="OG Title">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Title != "OG Title" {
		t.Errorf("Title = %q, want OG Title (og:title overrides <title>)", p.Title)
	}
}

func TestFetchLinkPreview_OGDescription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><head>
			<meta property="og:description" content="OG description text">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Description != "OG description text" {
		t.Errorf("Description = %q, want %q", p.Description, "OG description text")
	}
}

func TestFetchLinkPreview_OGImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><head>
			<meta property="og:image" content="https://example.com/og.png">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ImageURL != "https://example.com/og.png" {
		t.Errorf("ImageURL = %q, want og image", p.ImageURL)
	}
}

func TestFetchLinkPreview_ImgWithDimensionsPreferred(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><body>
			<img src="small.png">
			<img src="large.png" width="200" height="150">
		</body></html>`)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The image with declared dimensions >=100x100 should win.
	if p.ImageURL != "large.png" {
		t.Errorf("ImageURL = %q, want large.png (with dimensions)", p.ImageURL)
	}
}

func TestFetchLinkPreview_ContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", "2048")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ContentLength != 2048 {
		t.Errorf("ContentLength = %d, want 2048", p.ContentLength)
	}
}

func TestFetchLinkPreview_Non2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `<html><body>Not Found</body></html>`)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	// The function should not return a hard error for 4xx responses.
	if err != nil {
		t.Fatalf("unexpected error for non-2xx: %v", err)
	}
	if p == nil {
		t.Fatal("preview should not be nil")
	}
	// Status code should be reflected.
	if p.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", p.StatusCode, http.StatusNotFound)
	}
}

func TestFetchLinkPreview_ContextCancellation(t *testing.T) {
	// Server that delays so the client context can be cancelled first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><head><title>Late</title></head><body></body></html>`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := FetchLinkPreview(ctx, srv.URL, srv.Client())
	if err == nil {
		t.Error("expected error due to context cancellation, got nil")
	}
}

func TestFetchLinkPreviews_OrderPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return the path as the title so we can identify each URL.
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head><title>%s</title></head><body></body></html>`, r.URL.Path)
	}))
	defer srv.Close()

	urls := []string{
		srv.URL + "/page1",
		srv.URL + "/page2",
		srv.URL + "/page3",
	}
	results := FetchLinkPreviews(context.Background(), urls, srv.Client(), 2)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, u := range urls {
		if results[i].URL != u {
			t.Errorf("results[%d].URL = %q, want %q", i, results[i].URL, u)
		}
	}
}

func TestFetchLinkPreviews_OneURLErroring(t *testing.T) {
	// Use a URL that will refuse connections.
	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><head><title>Good</title></head><body></body></html>`)
	}))
	defer goodSrv.Close()

	urls := []string{
		goodSrv.URL,
		"http://127.0.0.1:1", // should fail (connection refused)
	}
	results := FetchLinkPreviews(context.Background(), urls, goodSrv.Client(), 2)

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	// Good URL should succeed
	if results[0] == nil {
		t.Error("results[0] should not be nil")
	}
	// Erroring URL should return a stub (not nil)
	if results[1] == nil {
		t.Error("results[1] should not be nil even on error")
	}
	if results[1].URL != "http://127.0.0.1:1" {
		t.Errorf("results[1].URL = %q, want stub with original URL", results[1].URL)
	}
}

func TestFetchLinkPreviews_MaxConcurrentZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `<html><head><title>T</title></head><body></body></html>`)
	}))
	defer srv.Close()

	urls := []string{srv.URL, srv.URL}
	// maxConcurrent=0 should default to 1 and not panic.
	results := FetchLinkPreviews(context.Background(), urls, srv.Client(), 0)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if r == nil {
			t.Errorf("results[%d] is nil", i)
		}
	}
}

func TestFetchLinkPreview_ApplyHeaders(t *testing.T) {
	var gotUA string
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUA == "" {
		t.Error("User-Agent header was not set")
	}
	if gotAccept == "" {
		t.Error("Accept header was not set")
	}
}

func TestFetchLinkPreview_BodyReadLimit(t *testing.T) {
	// Serve a very large body (well above previewMaxBytes) and ensure the
	// function still returns without hanging and produces a valid preview.
	bigBody := strings.Repeat("x", previewMaxBytes*3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Write a title before the large payload so metadata extraction works.
		fmt.Fprintf(w, `<html><head><title>Big</title></head><body>%s</body></html>`, bigBody)
	}))
	defer srv.Close()

	p, err := FetchLinkPreview(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("preview should not be nil")
	}
	// Title may or may not be parsed depending on where LimitReader cuts off,
	// but we should at least have a valid (non-error) preview object.
	_ = p.Title
}
