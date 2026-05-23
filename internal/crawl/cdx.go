package crawl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CDXRecord represents a single record returned by the Common Crawl CDX API.
type CDXRecord struct {
	URL        string `json:"url"`
	Timestamp  string `json:"timestamp"`
	MimeType   string `json:"mime"`
	StatusCode string `json:"status"`
	Digest     string `json:"digest"`
	Length     string `json:"length"`
}

// cdxAPIResponse mirrors the JSON fields emitted by the CDX API (one object
// per line in NDJSON format).
type cdxAPIResponse struct {
	URL       string `json:"url"`
	Timestamp string `json:"timestamp"`
	Mime      string `json:"mime"`
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Length    string `json:"length"`
}

// CDXSeeder queries the Common Crawl CDX index API to discover previously
// crawled URLs for a domain. This provides an alternative URL seeding method
// to sitemaps.
type CDXSeeder struct {
	Client    *http.Client
	MaxURLs   int
	UserAgent string
	IndexURL  string // e.g. "https://index.commoncrawl.org/CC-MAIN-2024-51-index"
}

// DefaultCDXIndex is the latest Common Crawl CDX index endpoint.
const DefaultCDXIndex = "https://index.commoncrawl.org/CC-MAIN-2024-51-index"

// NewCDXSeeder returns a CDXSeeder with the given HTTP client and URL cap.
func NewCDXSeeder(client *http.Client, maxURLs int) *CDXSeeder {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &CDXSeeder{
		Client:    client,
		MaxURLs:   maxURLs,
		UserAgent: "crawl4go/1.0",
		IndexURL:  DefaultCDXIndex,
	}
}

// Discover queries the CDX API for the given domain, parses the NDJSON
// response, deduplicates by URL, filters to HTML content, and returns up to
// MaxURLs records.
func (c *CDXSeeder) Discover(ctx context.Context, domain string) ([]CDXRecord, error) {
	body, err := c.query(ctx, domain)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	seen := make(map[string]struct{})
	var results []CDXRecord

	scanner := bufio.NewScanner(body)
	// Allow up to 1 MB per line; CDX records can be long.
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var rec cdxAPIResponse
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // skip malformed lines
		}

		// Filter to HTML content only.
		if !isHTMLMime(rec.Mime) {
			continue
		}

		// Only keep successful responses.
		if rec.Status != "" && rec.Status != "200" {
			continue
		}

		normalized := normalizeRawURL(rec.URL)
		if normalized == "" {
			continue
		}

		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}

		results = append(results, CDXRecord{
			URL:        normalized,
			Timestamp:  rec.Timestamp,
			MimeType:   rec.Mime,
			StatusCode: rec.Status,
			Digest:     rec.Digest,
			Length:     rec.Length,
		})

		if c.MaxURLs > 0 && len(results) >= c.MaxURLs {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		// Return what we have so far alongside the error.
		if len(results) > 0 {
			return results, nil
		}
		return nil, fmt.Errorf("cdx: reading response: %w", err)
	}

	return results, nil
}

// DiscoverURLs is a convenience wrapper around Discover that returns just the
// deduplicated URL strings.
func (c *CDXSeeder) DiscoverURLs(ctx context.Context, domain string) ([]string, error) {
	records, err := c.Discover(ctx, domain)
	if err != nil {
		return nil, err
	}

	urls := make([]string, len(records))
	for i, r := range records {
		urls[i] = r.URL
	}
	return urls, nil
}

// query performs the HTTP GET against the CDX API for the given domain and
// returns the response body.
func (c *CDXSeeder) query(ctx context.Context, domain string) (io.ReadCloser, error) {
	// Build the query URL: ?url=*.example.com&output=json&limit=<N>
	limit := c.MaxURLs
	if limit <= 0 {
		limit = 10000 // sensible upper bound
	}

	queryURL := fmt.Sprintf("%s?url=%s&output=json&limit=%d",
		c.IndexURL,
		url.QueryEscape("*."+domain),
		limit,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdx: building request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdx: fetching index: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, &fetchError{code: resp.StatusCode, url: queryURL}
	}

	return resp.Body, nil
}

// isHTMLMime returns true when the MIME type indicates HTML content.
func isHTMLMime(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	return m == "text/html" ||
		strings.HasPrefix(m, "text/html;") ||
		m == "application/xhtml+xml"
}

// normalizeRawURL performs basic URL normalisation: lowercases the scheme and
// host, strips fragments, and trims trailing slashes from the path.
func normalizeRawURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}
