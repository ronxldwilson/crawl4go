package crawl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// buildNDJSON returns an NDJSON string from the provided JSON-object lines.
func buildNDJSON(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

// htmlRecord returns a minimal CDX NDJSON line for an HTML URL.
func htmlRecord(u string) string {
	return fmt.Sprintf(`{"url":%q,"timestamp":"20240101120000","mime":"text/html","status":"200","digest":"SHA1:abc","length":"1234"}`, u)
}

// newTestSeeder returns a CDXSeeder wired to the given test-server URL.
func newTestSeeder(serverURL string, maxURLs int) *CDXSeeder {
	return &CDXSeeder{
		Client:    &http.Client{},
		MaxURLs:   maxURLs,
		UserAgent: "test/1.0",
		IndexURL:  serverURL,
	}
}

// TestCDXDiscover_ValidHTMLRecords: 3 valid HTML records → 3 CDXRecord entries.
func TestCDXDiscover_ValidHTMLRecords(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/a"),
		htmlRecord("https://example.com/b"),
		htmlRecord("https://example.com/c"),
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("got %d records, want 3", len(records))
	}
}

// TestCDXDiscover_NonHTMLMIMESkipped: application/pdf records are skipped.
func TestCDXDiscover_NonHTMLMIMESkipped(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/page"),
		`{"url":"https://example.com/doc.pdf","timestamp":"20240101","mime":"application/pdf","status":"200","digest":"SHA1:pdf","length":"5000"}`,
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("got %d records, want 1 (pdf should be filtered)", len(records))
	}
	if records[0].URL != "https://example.com/page" {
		t.Errorf("unexpected URL %q", records[0].URL)
	}
}

// TestCDXDiscover_Non200StatusSkipped: records with status "404" are skipped.
func TestCDXDiscover_Non200StatusSkipped(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/ok"),
		`{"url":"https://example.com/missing","timestamp":"20240101","mime":"text/html","status":"404","digest":"SHA1:x","length":"100"}`,
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("got %d records, want 1 (404 should be filtered)", len(records))
	}
}

// TestCDXDiscover_DuplicateURLsDeduped: duplicate URLs → only first kept.
func TestCDXDiscover_DuplicateURLsDeduped(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/page"),
		htmlRecord("https://example.com/page"), // duplicate
		htmlRecord("https://example.com/other"),
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2 (duplicate should be removed)", len(records))
	}
}

// TestCDXDiscover_MaxURLsCap: MaxURLs=2 with 5 valid records → exactly 2.
func TestCDXDiscover_MaxURLsCap(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/1"),
		htmlRecord("https://example.com/2"),
		htmlRecord("https://example.com/3"),
		htmlRecord("https://example.com/4"),
		htmlRecord("https://example.com/5"),
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 2)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2 (MaxURLs cap)", len(records))
	}
}

// TestCDXDiscover_MalformedJSONLineSkipped: malformed lines are skipped; valid lines still processed.
func TestCDXDiscover_MalformedJSONLineSkipped(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/valid"),
		`{this is not valid json`,
		htmlRecord("https://example.com/also-valid"),
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2 (malformed line skipped)", len(records))
	}
}

// TestCDXDiscover_EmptyBody: empty response body → empty slice, no error.
func TestCDXDiscover_EmptyBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// write nothing
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	records, err := s.Discover(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0 for empty body", len(records))
	}
}

// TestCDXDiscover_HTTPNon2xxReturnsError: non-2xx from CDX API → error returned.
func TestCDXDiscover_HTTPNon2xxReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	_, err := s.Discover(context.Background(), "example.com")
	if err == nil {
		t.Error("expected error for non-2xx HTTP response, got nil")
	}
}

// TestCDXDiscover_ContextCancellation: ctx cancelled before response → error returned.
func TestCDXDiscover_ContextCancellation(t *testing.T) {
	ready := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(ready)
		// Block until the client goes away.
		<-r.Context().Done()
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := newTestSeeder(ts.URL, 0)

	errCh := make(chan error, 1)
	go func() {
		_, err := s.Discover(ctx, "example.com")
		errCh <- err
	}()

	<-ready   // wait until server received the request
	cancel()  // now cancel

	err := <-errCh
	if err == nil {
		t.Error("expected error after context cancellation, got nil")
	}
}

// TestCDXDiscoverURLs_ConvenienceWrapper: DiscoverURLs returns only URL strings.
func TestCDXDiscoverURLs_ConvenienceWrapper(t *testing.T) {
	body := buildNDJSON(
		htmlRecord("https://example.com/a"),
		htmlRecord("https://example.com/b"),
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	urls, err := s.DiscoverURLs(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("DiscoverURLs error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("got %d URLs, want 2", len(urls))
	}
	for _, u := range urls {
		if u == "" {
			t.Error("got empty URL string from DiscoverURLs")
		}
	}
}

// TestCDXDiscoverURLs_PropagatesError: error from Discover is propagated.
func TestCDXDiscoverURLs_PropagatesError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	s := newTestSeeder(ts.URL, 0)
	_, err := s.DiscoverURLs(context.Background(), "example.com")
	if err == nil {
		t.Error("expected error propagated from Discover, got nil")
	}
}

// TestIsHTMLMime covers MIME type matching logic.
func TestIsHTMLMime(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"text/html", true},
		{"TEXT/HTML", true},
		{"text/html; charset=utf-8", true},
		{"text/html;charset=utf-8", true},
		{"application/xhtml+xml", true},
		{"APPLICATION/XHTML+XML", true},
		{"application/pdf", false},
		{"text/plain", false},
		{"image/png", false},
		{"", false},
		{"text/htmlx", false}, // not a prefix match unless followed by ";"
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			got := isHTMLMime(tt.mime)
			if got != tt.want {
				t.Errorf("isHTMLMime(%q) = %v, want %v", tt.mime, got, tt.want)
			}
		})
	}
}

// TestNormalizeRawURL covers URL normalisation behaviour.
func TestNormalizeRawURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "lowercases scheme",
			input: "HTTP://Example.COM/path",
			want:  "http://example.com/path",
		},
		{
			name:  "lowercases host",
			input: "https://EXAMPLE.COM/Path",
			want:  "https://example.com/Path",
		},
		{
			name:  "strips fragment",
			input: "https://example.com/page#section",
			want:  "https://example.com/page",
		},
		{
			name:  "strips trailing slash from path",
			input: "https://example.com/path/",
			want:  "https://example.com/path",
		},
		{
			name:  "preserves root slash",
			input: "https://example.com/",
			want:  "https://example.com/",
		},
		{
			name:  "preserves query string",
			input: "https://example.com/search?q=test",
			want:  "https://example.com/search?q=test",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "URL without host returns empty",
			input: "/relative/path",
			want:  "",
		},
		{
			name:  "strips fragment and trailing slash together",
			input: "https://example.com/page/#anchor",
			want:  "https://example.com/page",
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
