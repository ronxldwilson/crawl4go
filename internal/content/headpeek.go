package content

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// HeadPeekResult holds metadata extracted from the <head> section.
type HeadPeekResult struct {
	URL         string            `json:"url"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	OGTitle     string            `json:"og_title,omitempty"`
	OGImage     string            `json:"og_image,omitempty"`
	OGType      string            `json:"og_type,omitempty"`
	Canonical   string            `json:"canonical,omitempty"`
	Language    string            `json:"language,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	StatusCode  int               `json:"status_code"`
	Meta        map[string]string `json:"meta,omitempty"`
}

// PeekHead performs a partial HTTP fetch, reading only enough bytes to
// capture the <head> section. This saves bandwidth compared to fetching
// the entire page when only metadata is needed.
func PeekHead(ctx context.Context, url string, client *http.Client) (*HeadPeekResult, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	// Request partial content — most servers ignore Range for HTML but worth trying
	req.Header.Set("Range", "bytes=0-32767")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read up to 32KB — enough for virtually any <head> section
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err != nil {
		return nil, err
	}

	html := string(body)
	result := &HeadPeekResult{
		URL:         url,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Meta:        make(map[string]string),
	}

	// Extract <head> content
	headContent := html
	if idx := strings.Index(strings.ToLower(html), "</head>"); idx > 0 {
		headContent = html[:idx]
	}

	// Title
	if m := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`).FindStringSubmatch(headContent); len(m) > 1 {
		result.Title = strings.TrimSpace(m[1])
	}

	// Meta tags
	metaPattern := regexp.MustCompile(`(?i)<meta\s+([^>]+)>`)
	namePattern := regexp.MustCompile(`(?i)(?:name|property)\s*=\s*"([^"]*)"`)
	contentPattern := regexp.MustCompile(`(?i)content\s*=\s*"([^"]*)"`)

	for _, match := range metaPattern.FindAllStringSubmatch(headContent, -1) {
		if len(match) < 2 {
			continue
		}
		attrs := match[1]
		nameMatch := namePattern.FindStringSubmatch(attrs)
		contentMatch := contentPattern.FindStringSubmatch(attrs)
		if len(nameMatch) > 1 && len(contentMatch) > 1 {
			name := strings.ToLower(nameMatch[1])
			value := contentMatch[1]
			result.Meta[name] = value

			switch name {
			case "description":
				result.Description = value
			case "og:title":
				result.OGTitle = value
			case "og:image":
				result.OGImage = value
			case "og:type":
				result.OGType = value
			}
		}
	}

	// Canonical
	if m := regexp.MustCompile(`(?i)<link[^>]+rel\s*=\s*"canonical"[^>]+href\s*=\s*"([^"]*)"[^>]*>`).FindStringSubmatch(headContent); len(m) > 1 {
		result.Canonical = m[1]
	}

	// Language
	if m := regexp.MustCompile(`(?i)<html[^>]+lang\s*=\s*"([^"]*)"[^>]*>`).FindStringSubmatch(html); len(m) > 1 {
		result.Language = m[1]
	}

	return result, nil
}
