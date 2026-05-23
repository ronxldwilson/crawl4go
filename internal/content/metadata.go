package content

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// PageMetadata holds structured metadata extracted from an HTML page.
type PageMetadata struct {
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Keywords    string            `json:"keywords,omitempty"`
	Author      string            `json:"author,omitempty"`
	Language    string            `json:"language,omitempty"`
	OpenGraph   map[string]string `json:"open_graph,omitempty"`
	TwitterCard map[string]string `json:"twitter_card,omitempty"`
	JSONLD      []json.RawMessage `json:"json_ld,omitempty"`
	Canonical   string            `json:"canonical,omitempty"`
}

// ExtractMetadata parses htmlContent and returns structured page metadata.
func ExtractMetadata(htmlContent string) *PageMetadata {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return &PageMetadata{}
	}

	m := &PageMetadata{
		OpenGraph:   make(map[string]string),
		TwitterCard: make(map[string]string),
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.ElementNode:
			switch n.Data {
			case "html":
				if lang := GetAttr(n, "lang"); lang != "" {
					m.Language = lang
				}

			case "title":
				if m.Title == "" {
					if text := ExtractText(n); text != "" {
						m.Title = text
					}
				}

			case "meta":
				property := strings.ToLower(GetAttr(n, "property"))
				name := strings.ToLower(GetAttr(n, "name"))
				content := GetAttr(n, "content")

				switch {
				case strings.HasPrefix(property, "og:"):
					key := property[len("og:"):]
					m.OpenGraph[key] = content

				case strings.HasPrefix(property, "twitter:"):
					key := property[len("twitter:"):]
					m.TwitterCard[key] = content

				case strings.HasPrefix(name, "twitter:"):
					key := name[len("twitter:"):]
					m.TwitterCard[key] = content

				case name == "description":
					if m.Description == "" {
						m.Description = content
					}

				case name == "keywords":
					if m.Keywords == "" {
						m.Keywords = content
					}

				case name == "author":
					if m.Author == "" {
						m.Author = content
					}
				}

			case "link":
				if strings.EqualFold(GetAttr(n, "rel"), "canonical") {
					if href := GetAttr(n, "href"); href != "" && m.Canonical == "" {
						m.Canonical = href
					}
				}

			case "script":
				if strings.EqualFold(GetAttr(n, "type"), "application/ld+json") {
					raw := ExtractText(n)
					raw = strings.TrimSpace(raw)
					if raw != "" && json.Valid([]byte(raw)) {
						m.JSONLD = append(m.JSONLD, json.RawMessage(raw))
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	walk(doc)

	// Fallbacks: use og: values when primary fields are empty.
	if m.Title == "" {
		if ogTitle, ok := m.OpenGraph["title"]; ok {
			m.Title = ogTitle
		}
	}
	if m.Description == "" {
		if ogDesc, ok := m.OpenGraph["description"]; ok {
			m.Description = ogDesc
		}
	}

	// Nil out empty maps so JSON output stays clean.
	if len(m.OpenGraph) == 0 {
		m.OpenGraph = nil
	}
	if len(m.TwitterCard) == 0 {
		m.TwitterCard = nil
	}

	return m
}
