package content

import (
	"bytes"
	"context"
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

	body := FindBody(doc)
	if body == nil {
		return htmlContent, nil
	}

	removeExcludedTags(body)
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

	textDensity := textLen / tagLen
	linkDensity := 1.0 - (linkTextLen / textLen)
	if linkDensity < 0 {
		linkDensity = 0
	}

	tw := 0.5
	if w, ok := tagWeights[n.Data]; ok {
		tw = w
	}

	classIDWeight := 1.0
	classVal := GetAttr(n, "class") + " " + GetAttr(n, "id")
	if negativeClassRe.MatchString(classVal) {
		classIDWeight = 0.2
	}

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

func FindBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Body {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := FindBody(c); found != nil {
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

// ---------------------------------------------------------------------------
// PruningConfig-based content filter (#39)
// ---------------------------------------------------------------------------

// PruningConfig holds parameters for the configurable pruning content filter.
type PruningConfig struct {
	MinTextLength      int      `json:"min_text_length"`
	MinWordCount       int      `json:"min_word_count"`
	MaxLinkDensity     float64  `json:"max_link_density"`
	BoilerplatePatterns []string `json:"boilerplate_patterns"`
}

// DefaultPruningConfig returns a PruningConfig with sensible defaults.
func DefaultPruningConfig() PruningConfig {
	return PruningConfig{
		MinTextLength:  25,
		MinWordCount:   3,
		MaxLinkDensity: 0.7,
		BoilerplatePatterns: []string{
			"cookie", "privacy policy", "terms of service",
			"subscribe", "newsletter", "sign up", "log in",
			"advertisement", "sponsored", "all rights reserved",
			"copyright", "follow us",
		},
	}
}

// ConfigurablePruningFilter implements ContentFilter using configurable
// heuristics that score DOM-like text blocks by text length, word count,
// link density, and boilerplate pattern presence.
type ConfigurablePruningFilter struct {
	Config PruningConfig
}

// NewConfigurablePruningFilter creates a filter with the given config.
func NewConfigurablePruningFilter(cfg PruningConfig) *ConfigurablePruningFilter {
	return &ConfigurablePruningFilter{Config: cfg}
}

func (f *ConfigurablePruningFilter) Name() string { return "configurable-pruning" }

func (f *ConfigurablePruningFilter) Filter(_ context.Context, blocks []string, _ string) ([]FilteredBlock, error) {
	results := make([]FilteredBlock, len(blocks))
	for i, block := range blocks {
		score := f.scoreBlock(block)
		results[i] = FilteredBlock{
			Content: block,
			Score:   score,
			Index:   i,
			Kept:    score > 0,
		}
	}
	return results, nil
}

// scoreBlock computes a relevance score for a text block.
// Returns 0 if the block fails hard thresholds, otherwise a positive score.
func (f *ConfigurablePruningFilter) scoreBlock(block string) float64 {
	text := HTMLToText(block)
	textLen := len(text)
	words := strings.Fields(text)
	wordCount := len(words)

	// Hard threshold: minimum text length.
	if textLen < f.Config.MinTextLength {
		return 0
	}
	// Hard threshold: minimum word count.
	if wordCount < f.Config.MinWordCount {
		return 0
	}

	var score float64

	// Text length component (log-scaled, max 0.3).
	lenScore := math.Log(float64(textLen)+1) / 20.0
	if lenScore > 0.3 {
		lenScore = 0.3
	}
	score += lenScore

	// Word count component (max 0.2).
	wcScore := float64(wordCount) / 100.0
	if wcScore > 0.2 {
		wcScore = 0.2
	}
	score += wcScore

	// Link density penalty.
	linkTextLen := countLinkText(block)
	linkDensity := 0.0
	if textLen > 0 {
		linkDensity = float64(linkTextLen) / float64(textLen)
	}
	if linkDensity > f.Config.MaxLinkDensity {
		return 0
	}
	// Reward low link density (max 0.3).
	score += (1.0 - linkDensity) * 0.3

	// Boilerplate pattern penalty.
	lower := strings.ToLower(text)
	boilerplateHits := 0
	for _, pat := range f.Config.BoilerplatePatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			boilerplateHits++
		}
	}
	if boilerplateHits > 0 {
		penalty := float64(boilerplateHits) * 0.15
		score -= penalty
	}

	if score < 0 {
		score = 0
	}
	return score
}

// countLinkText counts the total length of text inside <a> tags in an HTML block.
func countLinkText(block string) int {
	doc, err := html.Parse(strings.NewReader(block))
	if err != nil {
		return 0
	}
	return nodeLinkTextLength(doc)
}
