package content

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// MediaItem represents a single media resource found in an HTML document.
type MediaItem struct {
	URL       string  `json:"url"`
	Type      string  `json:"type"` // "image", "video", or "audio"
	Alt       string  `json:"alt,omitempty"`
	Title     string  `json:"title,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	SourceTag string  `json:"source_tag"` // e.g. "img", "picture", "og:image", "video", "audio"
	Score     float64 `json:"score"`
}

// MediaSet groups extracted media items by type.
type MediaSet struct {
	Images []MediaItem `json:"images"`
	Videos []MediaItem `json:"videos"`
	Audio  []MediaItem `json:"audio"`
}

var bgImageRe = regexp.MustCompile(`url\(['"]?([^'")\s]+)['"]?\)`)

// iconPatterns are URL or class substrings that suggest non-content images.
var iconPatterns = []string{
	"icon", "logo", "avatar", "sprite", "favicon", "badge", "thumb/",
	"button", "arrow", "bullet", "pixel", "1x1", "blank",
}

// noiseAncestors are element names that suggest peripheral page regions.
var noiseAncestors = []string{"nav", "footer", "header", "aside"}

// contentAncestors are element names that suggest primary content regions.
var contentAncestors = []string{"article", "main", "content"}

// ExtractMedia parses htmlContent and returns all media items found.
// baseURL is used to resolve relative URLs.
func ExtractMedia(htmlContent string, baseURL string) MediaSet {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return MediaSet{Images: []MediaItem{}, Videos: []MediaItem{}, Audio: []MediaItem{}}
	}

	base, _ := url.Parse(baseURL)

	seenImages := make(map[string]bool)
	seenVideos := make(map[string]bool)
	seenAudio := make(map[string]bool)

	var ms MediaSet

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)

			switch tag {
			case "img":
				if item, ok := extractImgItem(n, base); ok {
					item.Score = scoreImage(item, n)
					if !seenImages[item.URL] {
						seenImages[item.URL] = true
						ms.Images = append(ms.Images, item)
					}
				}

			case "picture":
				// Collect <source> children inside <picture>
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && strings.ToLower(c.Data) == "source" {
						if item, ok := extractPictureSourceItem(c, base); ok {
							item.Score = scoreImage(item, n)
							if !seenImages[item.URL] {
								seenImages[item.URL] = true
								ms.Images = append(ms.Images, item)
							}
						}
					}
				}

			case "video":
				// The <video> tag itself may have a src attribute.
				if src := resolveAttr(n, "src", base); src != "" && !seenVideos[src] {
					seenVideos[src] = true
					ms.Videos = append(ms.Videos, MediaItem{
						URL:       src,
						Type:      "video",
						Title:     GetAttr(n, "title"),
						SourceTag: "video",
					})
				}
				// <source> children of <video>
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && strings.ToLower(c.Data) == "source" {
						if src := resolveAttr(c, "src", base); src != "" && !seenVideos[src] {
							seenVideos[src] = true
							ms.Videos = append(ms.Videos, MediaItem{
								URL:       src,
								Type:      "video",
								SourceTag: "source",
							})
						}
					}
				}

			case "audio":
				if src := resolveAttr(n, "src", base); src != "" && !seenAudio[src] {
					seenAudio[src] = true
					ms.Audio = append(ms.Audio, MediaItem{
						URL:       src,
						Type:      "audio",
						Title:     GetAttr(n, "title"),
						SourceTag: "audio",
					})
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && strings.ToLower(c.Data) == "source" {
						if src := resolveAttr(c, "src", base); src != "" && !seenAudio[src] {
							seenAudio[src] = true
							ms.Audio = append(ms.Audio, MediaItem{
								URL:       src,
								Type:      "audio",
								SourceTag: "source",
							})
						}
					}
				}

			case "meta":
				// OpenGraph image
				if strings.EqualFold(GetAttr(n, "property"), "og:image") {
					content := strings.TrimSpace(GetAttr(n, "content"))
					if content != "" {
						if base != nil {
							if resolved := NormalizeURL(content, base); resolved != "" {
								content = resolved
							}
						}
						if !seenImages[content] {
							seenImages[content] = true
							ms.Images = append(ms.Images, MediaItem{
								URL:       content,
								Type:      "image",
								SourceTag: "og:image",
								Score:     0.6, // og:image is generally meaningful; scored directly
							})
						}
					}
				}
			}

			// CSS background-image in inline style
			if style := GetAttr(n, "style"); style != "" {
				matches := bgImageRe.FindAllStringSubmatch(style, -1)
				for _, m := range matches {
					if len(m) < 2 {
						continue
					}
					rawURL := strings.TrimSpace(m[1])
					if rawURL == "" {
						continue
					}
					resolved := rawURL
					if base != nil {
						if r := NormalizeURL(rawURL, base); r != "" {
							resolved = r
						}
					}
					if !seenImages[resolved] {
						seenImages[resolved] = true
						item := MediaItem{
							URL:       resolved,
							Type:      "image",
							SourceTag: "css-background",
						}
						item.Score = scoreImage(item, n)
						ms.Images = append(ms.Images, item)
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	sort.Slice(ms.Images, func(i, j int) bool {
		return ms.Images[i].Score > ms.Images[j].Score
	})

	if ms.Images == nil {
		ms.Images = []MediaItem{}
	}
	if ms.Videos == nil {
		ms.Videos = []MediaItem{}
	}
	if ms.Audio == nil {
		ms.Audio = []MediaItem{}
	}

	return ms
}

// FilterMedia returns a new MediaSet containing only items with Score >= minScore.
func FilterMedia(ms MediaSet, minScore float64) MediaSet {
	out := MediaSet{
		Images: []MediaItem{},
		Videos: []MediaItem{},
		Audio:  []MediaItem{},
	}
	for _, item := range ms.Images {
		if item.Score >= minScore {
			out.Images = append(out.Images, item)
		}
	}
	for _, item := range ms.Videos {
		if item.Score >= minScore {
			out.Videos = append(out.Videos, item)
		}
	}
	for _, item := range ms.Audio {
		if item.Score >= minScore {
			out.Audio = append(out.Audio, item)
		}
	}
	return out
}

// extractImgItem builds a MediaItem from an <img> node.
func extractImgItem(n *html.Node, base *url.URL) (MediaItem, bool) {
	src := resolveAttr(n, "src", base)
	if src == "" {
		// Try data-src for lazy-loaded images
		src = resolveAttr(n, "data-src", base)
	}
	if src == "" {
		return MediaItem{}, false
	}

	item := MediaItem{
		URL:       src,
		Type:      "image",
		Alt:       GetAttr(n, "alt"),
		Title:     GetAttr(n, "title"),
		SourceTag: "img",
	}
	item.Width = attrInt(n, "width")
	item.Height = attrInt(n, "height")
	return item, true
}

// extractPictureSourceItem builds a MediaItem from a <source> inside <picture>.
func extractPictureSourceItem(n *html.Node, base *url.URL) (MediaItem, bool) {
	// Prefer srcset first candidate, fall back to src
	src := ""
	if srcset := strings.TrimSpace(GetAttr(n, "srcset")); srcset != "" {
		// Take the first URL in the srcset value (may be "url descriptor, ...")
		first := strings.Fields(srcset)[0]
		first = strings.TrimRight(first, ",")
		if base != nil {
			if r := NormalizeURL(first, base); r != "" {
				src = r
			} else {
				src = first
			}
		} else {
			src = first
		}
	}
	if src == "" {
		src = resolveAttr(n, "src", base)
	}
	if src == "" {
		return MediaItem{}, false
	}
	return MediaItem{
		URL:       src,
		Type:      "image",
		SourceTag: "picture",
	}, true
}

// resolveAttr reads an attribute from n and resolves it against base.
func resolveAttr(n *html.Node, attr string, base *url.URL) string {
	val := strings.TrimSpace(GetAttr(n, attr))
	if val == "" || base == nil {
		return val
	}
	if resolved := NormalizeURL(val, base); resolved != "" {
		return resolved
	}
	return val
}

// attrInt reads an integer attribute from n, returning 0 on failure.
func attrInt(n *html.Node, attr string) int {
	val := strings.TrimSpace(GetAttr(n, attr))
	if val == "" {
		return 0
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return i
}

// scoreImage computes a relevance score [0.0, 1.0] for an image MediaItem.
// n is the HTML node associated with the image (used for ancestor inspection).
func scoreImage(item MediaItem, n *html.Node) float64 {
	var score float64

	// +0.2 if has alt text
	if strings.TrimSpace(item.Alt) != "" {
		score += 0.2
	}

	// +0.2 if dimensions suggest a real image (not a tiny tracker/spacer)
	if item.Width >= 200 || item.Height >= 200 {
		score += 0.2
	}

	// +0.2 if NOT inside a noisy peripheral section (nav/footer/header/aside)
	if !hasAncestorTag(n, noiseAncestors) {
		score += 0.2
	}

	// +0.2 if URL and class attributes do not suggest icon/logo/sprite
	urlLower := strings.ToLower(item.URL)
	classLower := strings.ToLower(GetAttr(n, "class"))
	isIcon := false
	for _, pat := range iconPatterns {
		if strings.Contains(urlLower, pat) || strings.Contains(classLower, pat) {
			isIcon = true
			break
		}
	}
	if !isIcon {
		score += 0.2
	}

	// +0.2 if inside a primary content area (article/main/[class*=content])
	if hasAncestorTag(n, contentAncestors) || hasAncestorClass(n, "content") {
		score += 0.2
	}

	return score
}

// hasAncestorTag reports whether n has an ancestor whose tag is in tags.
func hasAncestorTag(n *html.Node, tags []string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			tag := strings.ToLower(p.Data)
			for _, t := range tags {
				if tag == t {
					return true
				}
			}
		}
	}
	return false
}

// hasAncestorClass reports whether n has an ancestor whose class attribute
// contains substr (case-insensitive).
func hasAncestorClass(n *html.Node, substr string) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode {
			if strings.Contains(strings.ToLower(GetAttr(p, "class")), substr) {
				return true
			}
			if strings.Contains(strings.ToLower(GetAttr(p, "id")), substr) {
				return true
			}
		}
	}
	return false
}
