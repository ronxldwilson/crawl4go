package main

import (
	"bytes"
	"math"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var excludedTags = map[atom.Atom]bool{
	atom.Nav: true, atom.Footer: true, atom.Header: true,
	atom.Aside: true, atom.Script: true, atom.Style: true,
	atom.Form: true, atom.Iframe: true, atom.Noscript: true,
}

var tagWeights = map[string]float64{
	"article": 1.5, "main": 1.5,
	"h1": 1.2, "h2": 1.1, "h3": 1.0,
	"p": 1.0, "section": 1.0,
	"blockquote": 0.8, "pre": 0.8, "code": 0.8,
	"ul": 0.5, "ol": 0.5, "li": 0.5,
	"div": 0.5, "table": 0.5, "tr": 0.5, "td": 0.5,
	"span": 0.3,
}

var negativeClassRe = regexp.MustCompile(`(?i)nav|footer|header|sidebar|ads?[\-_]|comment|promo|advert|social|share|widget|popup|modal|menu|breadcrumb|pagination`)

type PruningFilter struct {
	Threshold float64
}

func NewPruningFilter() *PruningFilter {
	return &PruningFilter{Threshold: 0.48}
}

func (pf *PruningFilter) Filter(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	body := findBody(doc)
	if body == nil {
		return htmlContent, nil
	}

	// Remove excluded tags first
	removeExcludedTags(body)

	// Recursive pruning
	pf.pruneTree(body)

	var buf bytes.Buffer
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&buf, c)
	}
	return buf.String(), nil
}

func (pf *PruningFilter) pruneTree(n *html.Node) {
	var children []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}

	for _, child := range children {
		if child.Type != html.ElementNode {
			continue
		}
		score := pf.computeScore(child)
		threshold := pf.dynamicThreshold(child)

		if score < threshold {
			n.RemoveChild(child)
		} else {
			pf.pruneTree(child)
		}
	}
}

func (pf *PruningFilter) computeScore(n *html.Node) float64 {
	textLen := float64(nodeTextLength(n))
	tagLen := float64(nodeHTMLLength(n))
	linkTextLen := float64(nodeLinkTextLength(n))

	if tagLen == 0 || textLen == 0 {
		return -1.0
	}

	// Text density (weight: 0.4)
	textDensity := textLen / tagLen

	// Link density (weight: 0.2) — penalizes link-heavy regions
	linkDensity := 1.0 - (linkTextLen / textLen)
	if linkDensity < 0 {
		linkDensity = 0
	}

	// Tag weight (weight: 0.2)
	tw := 0.5
	if w, ok := tagWeights[n.Data]; ok {
		tw = w
	}

	// Class/ID weight (weight: 0.1)
	classIDWeight := 1.0
	classVal := getAttr(n, "class") + " " + getAttr(n, "id")
	if negativeClassRe.MatchString(classVal) {
		classIDWeight = 0.2
	}

	// Text length score (weight: 0.1)
	textLenScore := math.Log(textLen+1) / 10.0
	if textLenScore > 1.0 {
		textLenScore = 1.0
	}

	return textDensity*0.4 + linkDensity*0.2 + tw*0.2 + classIDWeight*0.1 + textLenScore*0.1
}

func (pf *PruningFilter) dynamicThreshold(n *html.Node) float64 {
	threshold := pf.Threshold

	if w, ok := tagWeights[n.Data]; ok && w > 1.0 {
		threshold *= 0.8
	}

	textLen := float64(nodeTextLength(n))
	tagLen := float64(nodeHTMLLength(n))
	if tagLen > 0 && textLen/tagLen > 0.4 {
		threshold *= 0.9
	}

	linkTextLen := float64(nodeLinkTextLength(n))
	if textLen > 0 && linkTextLen/textLen > 0.6 {
		threshold *= 1.2
	}

	return threshold
}

func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findBody(c); found != nil {
			return found
		}
	}
	return nil
}

func removeExcludedTags(n *html.Node) {
	var toRemove []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && excludedTags[c.DataAtom] {
			toRemove = append(toRemove, c)
		} else {
			removeExcludedTags(c)
		}
	}
	for _, c := range toRemove {
		n.RemoveChild(c)
	}
}

func nodeTextLength(n *html.Node) int {
	if n.Type == html.TextNode {
		return len(strings.TrimSpace(n.Data))
	}
	total := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		total += nodeTextLength(c)
	}
	return total
}

func nodeHTMLLength(n *html.Node) int {
	var buf bytes.Buffer
	html.Render(&buf, n)
	return buf.Len()
}

func nodeLinkTextLength(n *html.Node) int {
	if n.Type == html.ElementNode && n.Data == "a" {
		return nodeTextLength(n)
	}
	total := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		total += nodeLinkTextLength(c)
	}
	return total
}
