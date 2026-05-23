package content

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/ronxldwilson/crawl4go/internal/ua"
	"golang.org/x/net/html"
)

// LinkPreview holds metadata extracted from a URL via HEAD + optional GET.
type LinkPreview struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	ImageURL      string `json:"image_url"`
	SiteName      string `json:"site_name"`
	Type          string `json:"type"`
	StatusCode    int    `json:"status_code"`
	ContentType   string `json:"content_type"`
	ContentLength int64  `json:"content_length"`
}

const previewMaxBytes = 50 * 1024 // 50 KB

// FetchLinkPreview performs a HEAD request to gather response metadata, then
// conditionally performs a limited GET to extract OpenGraph / fallback metadata
// when the resource is HTML.
func FetchLinkPreview(ctx context.Context, rawURL string, client *http.Client) (*LinkPreview, error) {
	uaResult := ua.RandomUA()

	preview := &LinkPreview{URL: rawURL}

	// --- HEAD request -------------------------------------------------------
	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(headReq, uaResult)

	headResp, err := client.Do(headReq)
	if err != nil {
		return nil, err
	}
	headResp.Body.Close()

	preview.StatusCode = headResp.StatusCode
	preview.ContentType = headResp.Header.Get("Content-Type")

	if cl := headResp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			preview.ContentLength = n
		}
	}

	// Only parse metadata for HTML responses.
	ct := strings.ToLower(preview.ContentType)
	if !strings.Contains(ct, "text/html") {
		return preview, nil
	}

	// --- GET request (limited to previewMaxBytes) ---------------------------
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		// HEAD data is still valid; return what we have.
		return preview, nil
	}
	applyHeaders(getReq, uaResult)

	getResp, err := client.Do(getReq)
	if err != nil {
		return preview, nil
	}
	defer getResp.Body.Close()

	// Override status code from the GET response (handles redirects etc.).
	preview.StatusCode = getResp.StatusCode

	limited := io.LimitReader(getResp.Body, previewMaxBytes)
	doc, err := html.Parse(limited)
	if err != nil {
		return preview, nil
	}

	extractMetadata(doc, preview)
	return preview, nil
}

// FetchLinkPreviews fetches previews for multiple URLs concurrently, bounded
// by maxConcurrent goroutines. Results are returned in the same order as urls.
func FetchLinkPreviews(ctx context.Context, urls []string, client *http.Client, maxConcurrent int) []*LinkPreview {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	results := make([]*LinkPreview, len(urls))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, rawURL := range urls {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p, err := FetchLinkPreview(ctx, u, client)
			if err != nil {
				// Return a minimal stub so callers always get an entry.
				p = &LinkPreview{URL: u}
			}
			results[idx] = p
		}(i, rawURL)
	}

	wg.Wait()
	return results
}

// applyHeaders sets realistic browser-like request headers.
func applyHeaders(req *http.Request, uaResult ua.UAResult) {
	req.Header.Set("User-Agent", uaResult.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if uaResult.SecCHUA != "" {
		req.Header.Set("Sec-CH-UA", uaResult.SecCHUA)
		req.Header.Set("Sec-CH-UA-Platform", uaResult.SecCHUAPlat)
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")
	}
}

// extractMetadata walks the parsed HTML tree to populate preview fields.
// OpenGraph tags take priority; <title> and <meta name=…> are used as fallbacks.
func extractMetadata(doc *html.Node, preview *LinkPreview) {
	var (
		ogTitle       string
		ogDescription string
		ogImage       string
		ogSiteName    string
		ogType        string
		metaTitle     string
		metaDesc      string
		titleText     string
		firstImg      string
	)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "title":
				if titleText == "" {
					titleText = strings.TrimSpace(ExtractText(n))
				}

			case "meta":
				prop := strings.ToLower(GetAttr(n, "property"))
				name := strings.ToLower(GetAttr(n, "name"))
				content := GetAttr(n, "content")

				switch prop {
				case "og:title":
					ogTitle = content
				case "og:description":
					ogDescription = content
				case "og:image":
					ogImage = content
				case "og:site_name":
					ogSiteName = content
				case "og:type":
					ogType = content
				}

				switch name {
				case "title":
					metaTitle = content
				case "description":
					metaDesc = content
				}

			case "img":
				// Track the first <img> src encountered; upgrade to one with declared
				// dimensions >= 100×100 if the current candidate lacks them.
				src := GetAttr(n, "src")
				if src != "" {
					w, _ := strconv.Atoi(GetAttr(n, "width"))
					h, _ := strconv.Atoi(GetAttr(n, "height"))
					hasDims := w >= 100 && h >= 100
					if firstImg == "" {
						firstImg = src
					} else if hasDims {
						// Upgrade: replace any previously stored candidate that lacked
						// sufficient declared dimensions.
						firstImg = src
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Populate preview with priority: OG > meta > <title> element.
	if ogTitle != "" {
		preview.Title = ogTitle
	} else if metaTitle != "" {
		preview.Title = metaTitle
	} else {
		preview.Title = titleText
	}

	if ogDescription != "" {
		preview.Description = ogDescription
	} else {
		preview.Description = metaDesc
	}

	if ogImage != "" {
		preview.ImageURL = ogImage
	} else {
		preview.ImageURL = firstImg
	}

	preview.SiteName = ogSiteName
	preview.Type = ogType
}
