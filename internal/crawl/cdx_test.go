package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsHTMLMime(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"text/html", true},
		{"TEXT/HTML", true},
		{"text/html; charset=utf-8", true},
		{"application/xhtml+xml", true},
		{"application/json", false},
		{"image/png", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			if got := isHTMLMime(tt.mime); got != tt.want {
				t.Errorf("isHTMLMime(%q) = %v, want %v", tt.mime, got, tt.want)
			}
		})
	}
}

func TestNormalizeRawURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic URL",
			input: "https://Example.COM/Page",
			want:  "https://example.com/Page",
		},
		{
			name:  "strips fragment",
			input: "https://example.com/page#section",
			want:  "https://example.com/page",
		},
		{
			name:  "strips trailing slash",
			input: "https://example.com/page/",
			want:  "https://example.com/page",
		},
		{
			name:  "root path preserved",
			input: "https://example.com",
			want:  "https://example.com/",
		},
		{
			name:  "empty host returns empty",
			input: "/relative/path",
			want:  "",
		},
		{
			name:  "invalid URL returns empty",
			input: "://broken",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRawURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRawURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewCDXSeeder(t *testing.T) {
	s := NewCDXSeeder(nil, 100)
	if s.Client == nil {
		t.Error("Client should not be nil when constructed with nil")
	}
	if s.MaxURLs != 100 {
		t.Errorf("MaxURLs = %d, want 100", s.MaxURLs)
	}
	if s.IndexURL != DefaultCDXIndex {
		t.Errorf("IndexURL = %q, want %q", s.IndexURL, DefaultCDXIndex)
	}
	if s.UserAgent != "crawl4go/1.0" {
		t.Errorf("UserAgent = %q, want %q", s.UserAgent, "crawl4go/1.0")
	}

	client := &http.Client{}
	s2 := NewCDXSeeder(client, 50)
	if s2.Client != client {
		t.Error("Client should be the provided client")
	}
}

func TestCDXSeederDiscover(t *testing.T) {
	ndjson := strings.Join([]string{
		`{"url":"https://example.com/page1","timestamp":"20240101","mime":"text/html","status":"200","digest":"abc","length":"1234"}`,
		`{"url":"https://example.com/page2","timestamp":"20240102","mime":"text/html","status":"200","digest":"def","length":"5678"}`,
		`{"url":"https://example.com/image.png","timestamp":"20240103","mime":"image/png","status":"200","digest":"ghi","length":"9999"}`,
		`{"url":"https://example.com/page3","timestamp":"20240104","mime":"text/html","status":"404","digest":"jkl","length":"100"}`,
		`{"url":"https://example.com/page1","timestamp":"20240105","mime":"text/html","status":"200","digest":"mno","length":"1234"}`,
		`not valid json at all`,
		``,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ndjson))
	}))
	defer server.Close()

	seeder := &CDXSeeder{
		Client:    server.Client(),
		MaxURLs:   100,
		UserAgent: "test/1.0",
		IndexURL:  server.URL,
	}

	records, err := seeder.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	// Should have 2 unique HTML 200 records (page1 deduped, image/png filtered, 404 filtered).
	if len(records) != 2 {
		t.Errorf("len(records) = %d, want 2", len(records))
		for _, r := range records {
			t.Logf("  record: %+v", r)
		}
		return
	}

	if records[0].MimeType != "text/html" {
		t.Errorf("records[0].MimeType = %q, want text/html", records[0].MimeType)
	}
	if records[0].StatusCode != "200" {
		t.Errorf("records[0].StatusCode = %q, want 200", records[0].StatusCode)
	}
}

func TestCDXSeederDiscoverMaxURLs(t *testing.T) {
	ndjson := strings.Join([]string{
		`{"url":"https://example.com/a","timestamp":"20240101","mime":"text/html","status":"200","digest":"a","length":"1"}`,
		`{"url":"https://example.com/b","timestamp":"20240102","mime":"text/html","status":"200","digest":"b","length":"2"}`,
		`{"url":"https://example.com/c","timestamp":"20240103","mime":"text/html","status":"200","digest":"c","length":"3"}`,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ndjson))
	}))
	defer server.Close()

	seeder := &CDXSeeder{
		Client:    server.Client(),
		MaxURLs:   2,
		UserAgent: "test/1.0",
		IndexURL:  server.URL,
	}

	records, err := seeder.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("len(records) = %d, want 2 (MaxURLs=2)", len(records))
	}
}

func TestCDXSeederDiscoverURLs(t *testing.T) {
	ndjson := `{"url":"https://example.com/page","timestamp":"20240101","mime":"text/html","status":"200","digest":"a","length":"1"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ndjson))
	}))
	defer server.Close()

	seeder := &CDXSeeder{
		Client:    server.Client(),
		MaxURLs:   100,
		UserAgent: "test/1.0",
		IndexURL:  server.URL,
	}

	urls, err := seeder.DiscoverURLs(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("DiscoverURLs() error: %v", err)
	}

	if len(urls) != 1 {
		t.Fatalf("len(urls) = %d, want 1", len(urls))
	}
	if !strings.Contains(urls[0], "example.com/page") {
		t.Errorf("urls[0] = %q, want to contain example.com/page", urls[0])
	}
}

func TestCDXSeederQueryError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	seeder := &CDXSeeder{
		Client:    server.Client(),
		MaxURLs:   100,
		UserAgent: "test/1.0",
		IndexURL:  server.URL,
	}

	_, err := seeder.Discover(context.Background(), "example.com")
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestCDXSeederEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
	}))
	defer server.Close()

	seeder := &CDXSeeder{
		Client:    server.Client(),
		MaxURLs:   100,
		UserAgent: "test/1.0",
		IndexURL:  server.URL,
	}

	records, err := seeder.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0 for empty response", len(records))
	}
}
