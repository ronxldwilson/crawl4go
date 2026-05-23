package crawl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// CacheEntry holds HTTP caching metadata for a previously fetched URL.
type CacheEntry struct {
	URL             string
	ETag            string
	LastModified    string
	HeadFingerprint string
	CachedAt        time.Time
}

// CacheValidator manages conditional HTTP requests to determine whether
// cached content is still fresh.
type CacheValidator struct {
	Client  *http.Client
	entries map[string]*CacheEntry
	mu      sync.RWMutex
}

// NewCacheValidator creates a CacheValidator using the supplied HTTP client.
func NewCacheValidator(client *http.Client) *CacheValidator {
	return &CacheValidator{
		Client:  client,
		entries: make(map[string]*CacheEntry),
	}
}

// IsFresh sends a conditional HEAD request for rawURL and reports whether the
// cached content is still current.
//
// Returns (false, nil) when no cache entry exists for the URL.
// Returns (true, nil) when the server confirms the content has not changed.
// Returns (false, nil) when the content has changed.
// Returns (false, err) on network or parse errors.
func (cv *CacheValidator) IsFresh(ctx context.Context, rawURL string) (bool, error) {
	cv.mu.RLock()
	entry, ok := cv.entries[rawURL]
	cv.mu.RUnlock()

	if !ok {
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return false, err
	}

	if entry.ETag != "" {
		req.Header.Set("If-None-Match", entry.ETag)
	}
	if entry.LastModified != "" {
		req.Header.Set("If-Modified-Since", entry.LastModified)
	}

	resp, err := cv.Client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Server confirmed content has not changed.
		return true, nil

	case http.StatusOK:
		// Server did not honour conditional headers; fall back to fingerprint comparison.
		getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return false, err
		}

		getResp, err := cv.Client.Do(getReq)
		if err != nil {
			return false, err
		}
		defer getResp.Body.Close()

		const maxHeadBytes = 50 * 1024
		limited := io.LimitReader(getResp.Body, maxHeadBytes)
		body, err := io.ReadAll(limited)
		if err != nil {
			return false, err
		}

		fingerprint := computeHeadFingerprint(string(body))
		return fingerprint == entry.HeadFingerprint, nil

	default:
		// Any other status (e.g. 404, 5xx) — treat as changed / unavailable.
		return false, nil
	}
}

// Update stores or replaces the cache entry for rawURL.
// headContent should be the raw HTML of the page (or at minimum its <head> section).
func (cv *CacheValidator) Update(rawURL string, etag, lastModified, headContent string) {
	fingerprint := computeHeadFingerprint(headContent)

	cv.mu.Lock()
	defer cv.mu.Unlock()

	cv.entries[rawURL] = &CacheEntry{
		URL:             rawURL,
		ETag:            etag,
		LastModified:    lastModified,
		HeadFingerprint: fingerprint,
		CachedAt:        time.Now(),
	}
}

// Get returns the cached entry for rawURL, or nil if none exists.
func (cv *CacheValidator) Get(rawURL string) *CacheEntry {
	cv.mu.RLock()
	defer cv.mu.RUnlock()
	return cv.entries[rawURL]
}

// Remove deletes the cache entry for rawURL.
func (cv *CacheValidator) Remove(rawURL string) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	delete(cv.entries, rawURL)
}

// computeHeadFingerprint parses the HTML head section of content and returns a
// SHA-256 hex digest built from selected meta values (title, description,
// og:title, canonical link).
func computeHeadFingerprint(headContent string) string {
	doc, err := html.Parse(strings.NewReader(headContent))
	if err != nil {
		// Fall back to hashing the raw content.
		sum := sha256.Sum256([]byte(headContent))
		return hex.EncodeToString(sum[:])
	}

	var (
		title       string
		description string
		ogTitle     string
		canonical   string
	)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "title":
				if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					title = strings.TrimSpace(n.FirstChild.Data)
				}

			case "meta":
				name := attrVal(n, "name")
				prop := attrVal(n, "property")
				content := attrVal(n, "content")

				switch strings.ToLower(name) {
				case "description":
					description = content
				}
				switch strings.ToLower(prop) {
				case "og:title":
					ogTitle = content
				}

			case "link":
				if strings.ToLower(attrVal(n, "rel")) == "canonical" {
					canonical = attrVal(n, "href")
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	combined := strings.Join([]string{title, description, ogTitle, canonical}, "|")
	sum := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(sum[:])
}

// attrVal returns the value of attribute key on node n, or an empty string.
func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}
