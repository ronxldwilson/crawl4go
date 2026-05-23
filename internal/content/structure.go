package content

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type PageStructure struct {
	Title       string            `json:"title"`
	H1          []string          `json:"h1"`
	H2          []string          `json:"h2"`
	H3          []string          `json:"h3"`
	FormCount   int               `json:"form_count"`
	LinkCount   int               `json:"link_count"`
	ImageCount  int               `json:"image_count"`
	ScriptCount int               `json:"script_count"`
	StyleCount  int               `json:"style_count"`
	IframeCount int               `json:"iframe_count"`
	NavCount    int               `json:"nav_count"`
	HasMainTag  bool              `json:"has_main_tag"`
	HasArticle  bool              `json:"has_article_tag"`
	HasAside    bool              `json:"has_aside_tag"`
	HasHeader   bool              `json:"has_header_tag"`
	HasFooter   bool              `json:"has_footer_tag"`
	MetaTags    map[string]string `json:"meta_tags"`
}

func AnalyzeStructure(htmlContent string) PageStructure {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return PageStructure{MetaTags: map[string]string{}}
	}

	ps := PageStructure{
		MetaTags: make(map[string]string),
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Title:
				if ps.Title == "" {
					ps.Title = ExtractText(n)
				}
			case atom.H1:
				ps.H1 = append(ps.H1, strings.TrimSpace(ExtractText(n)))
			case atom.H2:
				ps.H2 = append(ps.H2, strings.TrimSpace(ExtractText(n)))
			case atom.H3:
				ps.H3 = append(ps.H3, strings.TrimSpace(ExtractText(n)))
			case atom.Form:
				ps.FormCount++
			case atom.A:
				ps.LinkCount++
			case atom.Img:
				ps.ImageCount++
			case atom.Script:
				ps.ScriptCount++
			case atom.Style:
				ps.StyleCount++
			case atom.Iframe:
				ps.IframeCount++
			case atom.Nav:
				ps.NavCount++
			case atom.Main:
				ps.HasMainTag = true
			case atom.Article:
				ps.HasArticle = true
			case atom.Aside:
				ps.HasAside = true
			case atom.Header:
				ps.HasHeader = true
			case atom.Footer:
				ps.HasFooter = true
			case atom.Meta:
				name := GetAttr(n, "name")
				property := GetAttr(n, "property")
				cont := GetAttr(n, "content")
				if name != "" && cont != "" {
					ps.MetaTags[name] = cont
				}
				if property != "" && cont != "" {
					ps.MetaTags[property] = cont
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return ps
}
