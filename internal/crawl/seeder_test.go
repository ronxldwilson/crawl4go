package crawl

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestSeeder(client *http.Client, maxURLs int) *SitemapSeeder {
	s := NewSitemapSeeder(client, maxURLs)
	return s
}

func urlsetXML(urls []struct{ loc, lastmod, freq string; priority float64 }) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, u := range urls {
		b.WriteString(`<url><loc>`)
		b.WriteString(u.loc)
		b.WriteString(`</loc>`)
		if u.lastmod != "" {
			b.WriteString(`<lastmod>`)
			b.WriteString(u.lastmod)
			b.WriteString(`</lastmod>`)
		}
		if u.freq != "" {
			b.WriteString(`<changefreq>`)
			b.WriteString(u.freq)
			b.WriteString(`</changefreq>`)
		}
		if u.priority != 0 {
			b.WriteString(`<priority>`)
			b.WriteString(floatStr(u.priority))
			b.WriteString(`</priority>`)
		}
		b.WriteString(`</url>`)
	}
	b.WriteString(`</urlset>`)
	return b.String()
}

func floatStr(f float64) string {
	// simple conversion for test purposes
	if f == 0.5 {
		return "0.5"
	}
	if f == 0.8 {
		return "0.8"
	}
	if f == 1.0 {
		return "1.0"
	}
	return "0.0"
}

func sitemapIndexXML(locs []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, loc := range locs {
		b.WriteString(`<sitemap><loc>`)
		b.WriteString(loc)
		b.WriteString(`</loc></sitemap>`)
	}
	b.WriteString(`</sitemapindex>`)
	return b.String()
}

// ---------------------------------------------------------------------------
// TestSitemapParseURLSet – valid urlset with priorities, changefreq, lastmod
// ---------------------------------------------------------------------------

func TestSitemapParseURLSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
			{"https://example.com/a", "2024-03-15", "weekly", 0.8},
			{"https://example.com/b", "2024-01-01T12:00:00Z", "monthly", 0.5},
		})
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	seeds, err := s.ParseSitemap(ctx, srv.URL+"/sitemap.xml")
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds, got %d", len(seeds))
	}

	// Check first URL.
	if !strings.Contains(seeds[0].URL, "/a") {
		t.Errorf("seeds[0].URL = %q, want URL containing /a", seeds[0].URL)
	}
	if seeds[0].Priority != 0.8 {
		t.Errorf("seeds[0].Priority = %v, want 0.8", seeds[0].Priority)
	}
	if seeds[0].ChangeFreq != "weekly" {
		t.Errorf("seeds[0].ChangeFreq = %q, want weekly", seeds[0].ChangeFreq)
	}
	// lastmod 2024-03-15 → date only format
	if seeds[0].LastModified.Year() != 2024 || seeds[0].LastModified.Month() != 3 || seeds[0].LastModified.Day() != 15 {
		t.Errorf("seeds[0].LastModified = %v, want 2024-03-15", seeds[0].LastModified)
	}

	// Check second URL.
	if !strings.Contains(seeds[1].URL, "/b") {
		t.Errorf("seeds[1].URL = %q, want URL containing /b", seeds[1].URL)
	}
	if seeds[1].LastModified.Year() != 2024 || seeds[1].LastModified.Month() != 1 {
		t.Errorf("seeds[1].LastModified = %v, want 2024-01-xx", seeds[1].LastModified)
	}
}

// ---------------------------------------------------------------------------
// TestSitemapLastmodFormats – both date-only and RFC3339
// ---------------------------------------------------------------------------

func TestSitemapLastmodFormats(t *testing.T) {
	tests := []struct {
		name    string
		lastmod string
		wantY   int
		wantM   time.Month
		wantD   int
	}{
		{"date only", "2006-01-02", 2006, time.January, 2},
		{"RFC3339", "2023-07-04T18:30:00Z", 2023, time.July, 4},
		{"datetime no tz", "2022-12-25T00:00:00Z", 2022, time.December, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xmlData := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
				{"https://example.com/page", tt.lastmod, "", 0},
			})
			seeds, err := parseURLSet([]byte(xmlData), "sitemap")
			if err != nil {
				t.Fatalf("parseURLSet error: %v", err)
			}
			if len(seeds) != 1 {
				t.Fatalf("expected 1 seed, got %d", len(seeds))
			}
			got := seeds[0].LastModified
			if got.Year() != tt.wantY || got.Month() != tt.wantM || got.Day() != tt.wantD {
				t.Errorf("LastModified = %v, want %d-%02d-%02d", got, tt.wantY, tt.wantM, tt.wantD)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSitemapIndex – sitemapindex with two child sitemaps
// ---------------------------------------------------------------------------

func TestSitemapIndexTwoChildren(t *testing.T) {
	// Child handler serves a simple urlset.
	childHandler := func(path string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				http.NotFound(w, r)
				return
			}
			body := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
				{"https://example.com" + path, "", "", 0},
			})
			w.Write([]byte(body))
		}
	}

	// We need one server that handles the index + both children.
	mux := http.NewServeMux()
	mux.HandleFunc("/child1.xml", func(w http.ResponseWriter, r *http.Request) {
		body := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
			{"https://example.com/child1-page", "", "", 0},
		})
		w.Write([]byte(body))
	})
	mux.HandleFunc("/child2.xml", func(w http.ResponseWriter, r *http.Request) {
		body := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
			{"https://example.com/child2-page", "", "", 0},
		})
		w.Write([]byte(body))
	})
	_ = childHandler // suppress unused warning

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Register index handler after srv is up so we know srv.URL.
	mux.HandleFunc("/sitemap_index.xml", func(w http.ResponseWriter, r *http.Request) {
		body := sitemapIndexXML([]string{
			srv.URL + "/child1.xml",
			srv.URL + "/child2.xml",
		})
		w.Write([]byte(body))
	})

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	seeds, err := s.ParseSitemap(ctx, srv.URL+"/sitemap_index.xml")
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds (one from each child), got %d", len(seeds))
	}
	urls := map[string]bool{}
	for _, su := range seeds {
		urls[su.URL] = true
	}
	for _, want := range []string{"https://example.com/child1-page", "https://example.com/child2-page"} {
		if !urls[want] {
			t.Errorf("missing expected URL %q in seeds", want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestSitemapRecursionGuard – depth > 3 must not be fetched
// ---------------------------------------------------------------------------

func TestSitemapRecursionGuard(t *testing.T) {
	fetchCount := 0

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		// Always respond with another sitemapindex pointing at itself.
		body := sitemapIndexXML([]string{srv.URL + "/sitemap_index.xml"})
		w.Write([]byte(body))
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	_, err := s.ParseSitemap(ctx, srv.URL+"/sitemap_index.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// depth goes 0 → 1 → 2 → 3 → 4 (stops). So the server is called 4 times (depth 0..3).
	// At depth 4 parseSitemapDepth returns nil, nil before fetching.
	if fetchCount > 4 {
		t.Errorf("fetch count = %d, want ≤ 4 (recursion not guarded)", fetchCount)
	}
}

// ---------------------------------------------------------------------------
// TestSitemapMaxURLs – exactly MaxURLs returned
// ---------------------------------------------------------------------------

func TestSitemapMaxURLs(t *testing.T) {
	const total = 10
	const cap = 3

	var urls []struct{ loc, lastmod, freq string; priority float64 }
	for i := 0; i < total; i++ {
		urls = append(urls, struct{ loc, lastmod, freq string; priority float64 }{
			loc: "https://example.com/page-" + string(rune('a'+i)),
		})
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(urlsetXML(urls)))
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), cap)
	// parseSitemapDepth does not cap; Discover does. Use Discover.
	ctx := context.Background()
	seeds, err := s.Discover(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(seeds) != cap {
		t.Errorf("expected %d seeds (MaxURLs), got %d", cap, len(seeds))
	}
}

// ---------------------------------------------------------------------------
// TestSitemapGzip – gzip-compressed sitemap correctly decompressed
// ---------------------------------------------------------------------------

func TestSitemapGzip(t *testing.T) {
	rawXML := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
		{"https://example.com/gzipped", "", "", 0},
	})

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(rawXML))
	gz.Close()
	compressed := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(compressed)
	}))
	defer srv.Close()

	// Use a plain http.Client (not srv.Client()) but point at srv.URL.
	// We must use srv.Client() for TLS; srv here is plain HTTP so both work.
	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	seeds, err := s.ParseSitemap(ctx, srv.URL+"/sitemap.xml.gz")
	if err != nil {
		t.Fatalf("ParseSitemap error: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed, got %d", len(seeds))
	}
	if !strings.Contains(seeds[0].URL, "gzipped") {
		t.Errorf("URL = %q, want URL containing 'gzipped'", seeds[0].URL)
	}
}

// ---------------------------------------------------------------------------
// TestSitemapsFromRobots – Sitemap: directive extraction
// ---------------------------------------------------------------------------

func TestSitemapsFromRobotsDirective(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Write([]byte("User-agent: *\nDisallow: /private/\nSitemap: https://example.com/sitemap.xml\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	result := s.sitemapsFromRobots(ctx, srv.URL)
	if len(result) != 1 {
		t.Fatalf("expected 1 sitemap URL, got %d: %v", len(result), result)
	}
	if result[0] != "https://example.com/sitemap.xml" {
		t.Errorf("sitemap URL = %q, want https://example.com/sitemap.xml", result[0])
	}
}

// ---------------------------------------------------------------------------
// TestSitemapsFromRobotsNon2xx – non-2xx robots.txt → empty slice
// ---------------------------------------------------------------------------

func TestSitemapsFromRobotsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	result := s.sitemapsFromRobots(ctx, srv.URL)
	if len(result) != 0 {
		t.Errorf("expected empty slice for non-2xx robots.txt, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// TestDiscoverFallbackToCommonPaths – no Sitemap: in robots.txt → common paths tried
// ---------------------------------------------------------------------------

func TestDiscoverFallbackToCommonPaths(t *testing.T) {
	servedPaths := make(map[string]int)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servedPaths[r.URL.Path]++
		switch r.URL.Path {
		case "/robots.txt":
			// No Sitemap: directive.
			w.Write([]byte("User-agent: *\nDisallow:\n"))
		case "/sitemap.xml":
			body := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
				{"https://example.com/discovered", "", "", 0},
			})
			w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	seeds, err := s.Discover(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	// /sitemap.xml should have been tried via common paths.
	if servedPaths["/sitemap.xml"] == 0 {
		t.Error("expected /sitemap.xml to be fetched via common paths, but it was not")
	}

	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed from common-path sitemap, got %d", len(seeds))
	}
}

// ---------------------------------------------------------------------------
// TestDiscoverDeduplication – duplicate URLs across sitemaps deduplicated
// ---------------------------------------------------------------------------

func TestDiscoverDeduplication(t *testing.T) {
	dupURL := "https://example.com/same-page"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			// No Sitemap: directives.
			w.Write([]byte("User-agent: *\n"))
		case "/sitemap.xml", "/sitemap_index.xml", "/sitemap/sitemap.xml", "/sitemap.xml.gz":
			body := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
				{dupURL, "", "", 0},
			})
			w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	seeds, err := s.Discover(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	// All sitemaps return the same URL; it should appear only once.
	for _, su := range seeds {
		if su.URL == dupURL {
			// count occurrences
			count := 0
			for _, s2 := range seeds {
				if s2.URL == dupURL {
					count++
				}
			}
			if count > 1 {
				t.Errorf("duplicate URL %q appeared %d times, expected 1", dupURL, count)
			}
			return
		}
	}
}

// ---------------------------------------------------------------------------
// TestIsSitemapIndex
// ---------------------------------------------------------------------------

func TestIsSitemapIndex(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		want  bool
	}{
		{"sitemapindex document", `<?xml version="1.0"?><sitemapindex>...</sitemapindex>`, true},
		{"urlset document", `<?xml version="1.0"?><urlset>...</urlset>`, false},
		{"empty", "", false},
		{"partial sitemapindex tag", `<sitemapindex xmlns="...">`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSitemapIndex([]byte(tt.data))
			if got != tt.want {
				t.Errorf("isSitemapIndex(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestSitemapMalformedXML – malformed XML returns an error
// ---------------------------------------------------------------------------

func TestSitemapMalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<urlset><url><loc>https://example.com</loc></url`)) // truncated
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	_, err := s.ParseSitemap(ctx, srv.URL+"/sitemap.xml")
	if err == nil {
		t.Error("expected error for malformed XML, got nil")
	}
}

// ---------------------------------------------------------------------------
// TestSitemapEmpty – empty sitemap returns empty slice with no error
// ---------------------------------------------------------------------------

func TestSitemapEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"></urlset>`))
	}))
	defer srv.Close()

	s := newTestSeeder(srv.Client(), 0)
	ctx := context.Background()
	seeds, err := s.ParseSitemap(ctx, srv.URL+"/sitemap.xml")
	if err != nil {
		t.Fatalf("unexpected error for empty sitemap: %v", err)
	}
	if len(seeds) != 0 {
		t.Errorf("expected 0 seeds, got %d", len(seeds))
	}
}

// ---------------------------------------------------------------------------
// TestSitemapSourceLabel – source field propagated correctly
// ---------------------------------------------------------------------------

func TestSitemapSourceLabel(t *testing.T) {
	xmlData := urlsetXML([]struct{ loc, lastmod, freq string; priority float64 }{
		{"https://example.com/page", "", "", 0},
	})
	seeds, err := parseURLSet([]byte(xmlData), "robots-txt")
	if err != nil {
		t.Fatalf("parseURLSet error: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed, got %d", len(seeds))
	}
	if seeds[0].Source != "robots-txt" {
		t.Errorf("Source = %q, want robots-txt", seeds[0].Source)
	}
}
