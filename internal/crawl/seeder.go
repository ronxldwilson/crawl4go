package crawl

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

// SeedURL represents a discovered URL with its sitemap metadata.
type SeedURL struct {
	URL          string
	LastModified time.Time
	ChangeFreq   string
	Priority     float64
	Source       string // "sitemap", "sitemap-index", "robots-txt"
}

// SitemapSeeder discovers and parses sitemaps for a given base URL.
type SitemapSeeder struct {
	Client    *http.Client
	MaxURLs   int
	UserAgent string
}

// NewSitemapSeeder returns a SitemapSeeder with the given HTTP client and URL cap.
func NewSitemapSeeder(client *http.Client, maxURLs int) *SitemapSeeder {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &SitemapSeeder{
		Client:    client,
		MaxURLs:   maxURLs,
		UserAgent: "crawl4go/1.0",
	}
}

// Discover finds all seed URLs for baseURL by consulting robots.txt and
// common sitemap locations. Results are deduplicated and capped at MaxURLs.
func (s *SitemapSeeder) Discover(ctx context.Context, baseURL string) ([]SeedURL, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	origin := base.Scheme + "://" + base.Host

	seen := make(map[string]struct{})
	var results []SeedURL

	add := func(seeds []SeedURL) bool {
		for _, su := range seeds {
			if _, dup := seen[su.URL]; dup {
				continue
			}
			seen[su.URL] = struct{}{}
			results = append(results, su)
			if s.MaxURLs > 0 && len(results) >= s.MaxURLs {
				return false
			}
		}
		return true
	}

	// 1. Check robots.txt for Sitemap: directives.
	robotsSitemaps := s.sitemapsFromRobots(ctx, origin)
	for _, loc := range robotsSitemaps {
		if s.MaxURLs > 0 && len(results) >= s.MaxURLs {
			break
		}
		seeds, err := s.parseSitemapDepth(ctx, loc, "robots-txt", 0)
		if err != nil {
			continue
		}
		if !add(seeds) {
			break
		}
	}

	if s.MaxURLs > 0 && len(results) >= s.MaxURLs {
		return results, nil
	}

	// 2. Try common sitemap locations.
	commonPaths := []string{
		"/sitemap.xml",
		"/sitemap_index.xml",
		"/sitemap/sitemap.xml",
		"/sitemap.xml.gz",
	}
	for _, p := range commonPaths {
		if s.MaxURLs > 0 && len(results) >= s.MaxURLs {
			break
		}
		loc := origin + p
		// Skip if already processed via robots.txt.
		if _, already := seen[loc]; already {
			continue
		}
		seeds, err := s.parseSitemapDepth(ctx, loc, "sitemap", 0)
		if err != nil {
			continue
		}
		if !add(seeds) {
			break
		}
	}

	return results, nil
}

// ParseSitemap parses a single sitemap URL (index or urlset) and returns its
// seed URLs. Sitemap index entries are followed recursively up to depth 3.
func (s *SitemapSeeder) ParseSitemap(ctx context.Context, sitemapURL string) ([]SeedURL, error) {
	return s.parseSitemapDepth(ctx, sitemapURL, "sitemap", 0)
}

// parseSitemapDepth is the recursive implementation of ParseSitemap with a
// depth guard (max depth = 3).
func (s *SitemapSeeder) parseSitemapDepth(ctx context.Context, sitemapURL, source string, depth int) ([]SeedURL, error) {
	const maxDepth = 3
	if depth > maxDepth {
		return nil, nil
	}

	body, err := s.fetch(ctx, sitemapURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, 50*1024*1024))
	if err != nil {
		return nil, err
	}

	// Detect whether this is a sitemap index or a urlset.
	if isSitemapIndex(data) {
		return s.parseSitemapIndex(ctx, data, depth)
	}
	return parseURLSet(data, source)
}

// fetch performs an HTTP GET for the given URL and returns the response body,
// transparently decompressing gzip content if necessary.
func (s *SitemapSeeder) fetch(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.UserAgent)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &fetchError{code: resp.StatusCode, url: rawURL}
	}

	isGzip := strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") ||
		strings.HasSuffix(strings.ToLower(rawURL), ".gz")

	if isGzip {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		return &gzipBody{gr: gr, underlying: resp.Body}, nil
	}

	return resp.Body, nil
}

// sitemapsFromRobots fetches robots.txt and extracts Sitemap: directive values.
func (s *SitemapSeeder) sitemapsFromRobots(ctx context.Context, origin string) []string {
	robotsURL := origin + "/robots.txt"
	body, err := s.fetch(ctx, robotsURL)
	if err != nil {
		return nil
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, 500_000))
	if err != nil {
		return nil
	}

	var sitemaps []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "sitemap:") {
			loc := strings.TrimSpace(line[len("sitemap:"):])
			if loc != "" {
				sitemaps = append(sitemaps, loc)
			}
		}
	}
	return sitemaps
}

// isSitemapIndex returns true when the XML data looks like a sitemapindex document.
func isSitemapIndex(data []byte) bool {
	// Quick string scan before full parse.
	return strings.Contains(string(data), "<sitemapindex")
}

// parseSitemapIndex decodes a sitemapindex document and recursively fetches
// each child sitemap.
func (s *SitemapSeeder) parseSitemapIndex(ctx context.Context, data []byte, depth int) ([]SeedURL, error) {
	var idx xmlSitemapIndex
	if err := xml.Unmarshal(data, &idx); err != nil {
		return nil, err
	}

	var results []SeedURL
	for _, entry := range idx.Sitemaps {
		if s.MaxURLs > 0 && len(results) >= s.MaxURLs {
			break
		}
		loc := strings.TrimSpace(entry.Loc)
		if loc == "" {
			continue
		}
		seeds, err := s.parseSitemapDepth(ctx, loc, "sitemap-index", depth+1)
		if err != nil {
			continue
		}
		results = append(results, seeds...)
	}
	return results, nil
}

// parseURLSet decodes a urlset sitemap document into SeedURL slice.
func parseURLSet(data []byte, source string) ([]SeedURL, error) {
	var us xmlURLSet
	if err := xml.Unmarshal(data, &us); err != nil {
		return nil, err
	}

	base := &url.URL{} // placeholder for NormalizeURL; locs should already be absolute
	var results []SeedURL
	for _, u := range us.URLs {
		loc := strings.TrimSpace(u.Loc)
		if loc == "" {
			continue
		}

		// Resolve and normalise the URL using the content package helper.
		parsed, err := url.Parse(loc)
		if err != nil {
			continue
		}
		if parsed.IsAbs() {
			base = &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
		}
		normalized := content.NormalizeURL(loc, base)
		if normalized == "" {
			continue
		}

		su := SeedURL{
			URL:      normalized,
			Source:   source,
			Priority: u.Priority,
		}

		if u.ChangeFreq != "" {
			su.ChangeFreq = u.ChangeFreq
		}

		if u.LastMod != "" {
			// Try common date/datetime formats.
			for _, layout := range []string{
				time.RFC3339,
				"2006-01-02T15:04:05Z",
				"2006-01-02",
			} {
				if t, err := time.Parse(layout, strings.TrimSpace(u.LastMod)); err == nil {
					su.LastModified = t
					break
				}
			}
		}

		results = append(results, su)
	}
	return results, nil
}

// ---------------------------------------------------------------------------
// XML types
// ---------------------------------------------------------------------------

type xmlSitemapIndex struct {
	XMLName  xml.Name         `xml:"sitemapindex"`
	Sitemaps []xmlSitemapEntry `xml:"sitemap"`
}

type xmlSitemapEntry struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod"`
}

type xmlURLSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []xmlURL   `xml:"url"`
}

type xmlURL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod"`
	ChangeFreq string  `xml:"changefreq"`
	Priority   float64 `xml:"priority"`
}

// ---------------------------------------------------------------------------
// Helper types
// ---------------------------------------------------------------------------

// gzipBody wraps a gzip.Reader and its underlying io.ReadCloser so that
// closing either flushes both.
type gzipBody struct {
	gr         *gzip.Reader
	underlying io.ReadCloser
}

func (g *gzipBody) Read(p []byte) (int, error) { return g.gr.Read(p) }

func (g *gzipBody) Close() error {
	err1 := g.gr.Close()
	err2 := g.underlying.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

// fetchError is a lightweight error for non-2xx HTTP responses.
type fetchError struct {
	code int
	url  string
}

func (e *fetchError) Error() string {
	return "seeder: HTTP " + http.StatusText(e.code) + " fetching " + e.url
}
