package content

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ContentScrapingStrategy defines the interface for scraping content from HTML.
type ContentScrapingStrategy interface {
	Scrape(html string, config ScrapingConfig) (*ScrapingResult, error)
}

// ScrapingConfig holds parameters for a content scraping operation.
type ScrapingConfig struct {
	URL             string   `json:"url"`
	WaitForSelector string   `json:"wait_for_selector,omitempty"`
	RemoveSelectors []string `json:"remove_selectors,omitempty"`
	OnlyMainContent bool     `json:"only_main_content"`
}

// ScrapingResult holds the output of a content scraping operation.
type ScrapingResult struct {
	CleanHTML string            `json:"clean_html"`
	Markdown  string            `json:"markdown"`
	PlainText string            `json:"plain_text"`
	Links     []ScrapedLink     `json:"links"`
	Images    []ScrapedImage    `json:"images"`
	Metadata  map[string]string `json:"metadata"`
}

// ScrapedLink represents a hyperlink found during scraping.
type ScrapedLink struct {
	URL        string `json:"url"`
	Text       string `json:"text"`
	IsExternal bool   `json:"is_external"`
}

// ScrapedImage represents an image found during scraping.
type ScrapedImage struct {
	URL    string  `json:"url"`
	Alt    string  `json:"alt"`
	Width  string  `json:"width"`
	Height string  `json:"height"`
	Score  float64 `json:"score"`
}

// DefaultScrapingStrategy is a simple implementation of ContentScrapingStrategy
// that strips script/style tags, extracts links and images, and returns cleaned HTML.
type DefaultScrapingStrategy struct{}

// NewDefaultScrapingStrategy creates a new DefaultScrapingStrategy.
func NewDefaultScrapingStrategy() *DefaultScrapingStrategy {
	return &DefaultScrapingStrategy{}
}

func (s *DefaultScrapingStrategy) Scrape(htmlContent string, config ScrapingConfig) (*ScrapingResult, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	// Strip script and style elements.
	stripElements(doc, map[atom.Atom]bool{
		atom.Script:   true,
		atom.Style:    true,
		atom.Noscript: true,
	})

	// If OnlyMainContent, try to find <main> or <article>, otherwise use body.
	root := FindBody(doc)
	if root == nil {
		root = doc
	}
	if config.OnlyMainContent {
		if main := findElement(root, atom.Main); main != nil {
			root = main
		} else if article := findElement(root, atom.Article); article != nil {
			root = article
		}
	}

	// Render cleaned HTML.
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&buf, c)
	}
	cleanHTML := buf.String()

	// Extract plain text.
	plainText := HTMLToText(cleanHTML)

	// Extract links.
	var baseURL *url.URL
	if config.URL != "" {
		baseURL, _ = url.Parse(config.URL)
	}
	baseDomain := getBaseDomain(config.URL)
	links := extractScrapedLinks(root, baseURL, baseDomain)

	// Extract images.
	images := extractScrapedImages(root, baseURL)

	// Extract metadata from <meta> tags in the full document.
	metadata := extractScrapedMetadata(doc)

	return &ScrapingResult{
		CleanHTML: cleanHTML,
		Markdown:  "", // Markdown conversion is delegated to MarkdownStrategy.
		PlainText: plainText,
		Links:     links,
		Images:    images,
		Metadata:  metadata,
	}, nil
}

// stripElements removes all elements with the given atoms from the tree.
func stripElements(n *html.Node, atoms map[atom.Atom]bool) {
	var toRemove []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && atoms[c.DataAtom] {
			toRemove = append(toRemove, c)
		} else {
			stripElements(c, atoms)
		}
	}
	for _, c := range toRemove {
		n.RemoveChild(c)
	}
}

// findElement performs a depth-first search for the first element with the given atom.
func findElement(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, a); found != nil {
			return found
		}
	}
	return nil
}

// extractScrapedLinks walks the tree and returns all scraped links.
func extractScrapedLinks(n *html.Node, base *url.URL, baseDomain string) []ScrapedLink {
	var links []ScrapedLink
	seen := make(map[string]bool)

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.DataAtom == atom.A {
			href := GetAttr(node, "href")
			if href != "" && href != "#" && base != nil {
				normalized := NormalizeURL(href, base)
				if normalized != "" && !seen[normalized] {
					seen[normalized] = true
					links = append(links, ScrapedLink{
						URL:        normalized,
						Text:       ExtractText(node),
						IsExternal: isExternalURL(normalized, baseDomain),
					})
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if links == nil {
		links = []ScrapedLink{}
	}
	return links
}

// extractScrapedImages walks the tree and returns all scraped images.
func extractScrapedImages(n *html.Node, base *url.URL) []ScrapedImage {
	var images []ScrapedImage
	seen := make(map[string]bool)

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.DataAtom == atom.Img {
			src := GetAttr(node, "src")
			if src == "" {
				src = GetAttr(node, "data-src")
			}
			if src != "" {
				if base != nil {
					if resolved := NormalizeURL(src, base); resolved != "" {
						src = resolved
					}
				}
				if !seen[src] {
					seen[src] = true
					images = append(images, ScrapedImage{
						URL:    src,
						Alt:    GetAttr(node, "alt"),
						Width:  GetAttr(node, "width"),
						Height: GetAttr(node, "height"),
						Score:  scoreScrapedImage(node),
					})
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)

	if images == nil {
		images = []ScrapedImage{}
	}
	return images
}

// scoreScrapedImage assigns a simple relevance score to an image node.
func scoreScrapedImage(n *html.Node) float64 {
	score := 0.5 // baseline

	if strings.TrimSpace(GetAttr(n, "alt")) != "" {
		score += 0.2
	}

	w, _ := strconv.Atoi(GetAttr(n, "width"))
	h, _ := strconv.Atoi(GetAttr(n, "height"))
	if w >= 200 || h >= 200 {
		score += 0.2
	}

	if !hasAncestorTag(n, noiseAncestors) {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// extractScrapedMetadata extracts <meta> tag content from the document.
func extractScrapedMetadata(doc *html.Node) map[string]string {
	meta := make(map[string]string)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Meta {
			name := GetAttr(n, "name")
			if name == "" {
				name = GetAttr(n, "property")
			}
			content := GetAttr(n, "content")
			if name != "" && content != "" {
				meta[name] = content
			}
		}
		if n.Type == html.ElementNode && n.DataAtom == atom.Title {
			meta["title"] = ExtractText(n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return meta
}
