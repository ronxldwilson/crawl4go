package content

import (
	"bytes"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// Schema types
// ---------------------------------------------------------------------------

// XPathField describes how to extract a single named value using an XPath
// expression relative to a base node.
type XPathField struct {
	Name  string `json:"name"`
	XPath string `json:"xpath"`
	Type  string `json:"type"` // text | attribute | html
}

// XPathExtractionSchema ties a base XPath to a set of fields to extract from
// each matched element.
type XPathExtractionSchema struct {
	BaseXPath string       `json:"base_xpath"`
	Fields    []XPathField `json:"fields"`
}

// XPathExtractor performs structured XPath-based extraction.
type XPathExtractor struct {
	Schema XPathExtractionSchema
}

// NewXPathExtractor creates an XPathExtractor for the given schema.
func NewXPathExtractor(schema XPathExtractionSchema) *XPathExtractor {
	return &XPathExtractor{Schema: schema}
}

// Extract parses htmlContent, finds every element matching Schema.BaseXPath,
// and returns a slice of objects built according to Schema.Fields.
func (e *XPathExtractor) Extract(htmlContent string) ([]map[string]any, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	roots := xpathSelect(doc, e.Schema.BaseXPath)
	results := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		obj := xpathExtractFields(root, e.Schema.Fields)
		results = append(results, obj)
	}
	return results, nil
}

// xpathExtractFields builds a map by applying each field's XPath+type against
// the given node.
func xpathExtractFields(node *html.Node, fields []XPathField) map[string]any {
	obj := make(map[string]any, len(fields))
	for _, f := range fields {
		obj[f.Name] = xpathExtractField(node, f)
	}
	return obj
}

func xpathExtractField(node *html.Node, f XPathField) any {
	nodes := xpathSelect(node, f.XPath)
	if len(nodes) == 0 {
		return ""
	}
	target := nodes[0]

	switch f.Type {
	case "attribute":
		// The XPath itself should end with @attr; we extract from the last step.
		attr := xpathTrailingAttr(f.XPath)
		if attr != "" {
			return GetAttr(target, attr)
		}
		return ""
	case "html":
		return xpathRenderHTML(target)
	default: // "text" and anything unrecognised
		return ExtractText(target)
	}
}

// xpathTrailingAttr returns the attribute name if the XPath ends with /@attr.
func xpathTrailingAttr(xpath string) string {
	idx := strings.LastIndex(xpath, "/@")
	if idx < 0 {
		return ""
	}
	attr := xpath[idx+2:]
	// Strip any trailing predicates.
	if bracket := strings.IndexByte(attr, '['); bracket >= 0 {
		attr = attr[:bracket]
	}
	return strings.TrimSpace(attr)
}

// xpathRenderHTML serialises a node back to an HTML string.
func xpathRenderHTML(n *html.Node) string {
	var buf bytes.Buffer
	_ = html.Render(&buf, n)
	return buf.String()
}

// ---------------------------------------------------------------------------
// XPath evaluation engine
// ---------------------------------------------------------------------------

// xpathSelect evaluates a subset of XPath against the parsed HTML tree rooted
// at root and returns all matching nodes.
//
// Supported syntax:
//   - //tag            descendant-or-self axis
//   - /tag             direct child
//   - //tag[@attr]     attribute presence
//   - //tag[@attr="v"] attribute value
//   - text()           text node selector
//   - [N]              1-based position predicate
//   - [last()]         last position predicate
//   - //a//b           chained descendant steps
//   - /a/b             chained child steps
func xpathSelect(root *html.Node, xpath string) []*html.Node {
	xpath = strings.TrimSpace(xpath)
	if xpath == "" {
		return nil
	}

	steps := parseXPathSteps(xpath)
	if len(steps) == 0 {
		return nil
	}

	cur := []*html.Node{root}
	for _, step := range steps {
		cur = applyXPathStep(cur, step)
		if len(cur) == 0 {
			return nil
		}
	}
	return cur
}

// ---------------------------------------------------------------------------
// Step representation
// ---------------------------------------------------------------------------

type xpathAxis int

const (
	axisChild xpathAxis = iota
	axisDescendant
)

type xpathStep struct {
	axis      xpathAxis
	tag       string // "" means any (*), "text()" is special
	textNode  bool
	attrs     []xpathAttrPred
	position  int  // 0 = no position predicate; positive = 1-based index; -1 = last()
}

type xpathAttrPred struct {
	key   string
	value string // "" means presence check only
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// parseXPathSteps splits an XPath expression into a slice of steps.
func parseXPathSteps(xpath string) []xpathStep {
	// Normalise: trim leading slash(es) and split into raw segments.
	// We iterate character-by-character to correctly handle // vs /.
	type rawSeg struct {
		descendant bool
		expr       string
	}

	var segments []rawSeg
	i := 0
	n := len(xpath)

	for i < n {
		desc := false
		if i < n && xpath[i] == '/' {
			i++
			if i < n && xpath[i] == '/' {
				desc = true
				i++
			}
		}
		// Collect until next unbracketed '/'.
		start := i
		depth := 0
		for i < n {
			switch xpath[i] {
			case '[':
				depth++
			case ']':
				depth--
			case '/':
				if depth == 0 {
					goto segDone
				}
			}
			i++
		}
	segDone:
		expr := strings.TrimSpace(xpath[start:i])
		if expr != "" {
			segments = append(segments, rawSeg{descendant: desc, expr: expr})
		}
	}

	steps := make([]xpathStep, 0, len(segments))
	for _, seg := range segments {
		step := parseOneStep(seg.expr)
		if seg.descendant {
			step.axis = axisDescendant
		} else {
			step.axis = axisChild
		}
		steps = append(steps, step)
	}
	return steps
}

// parseOneStep parses a single step expression like "div[@class=\"foo\"][2]".
func parseOneStep(expr string) xpathStep {
	var step xpathStep

	// Check for text() node test.
	if expr == "text()" || strings.HasPrefix(expr, "text()[") {
		step.textNode = true
		expr = strings.TrimPrefix(expr, "text()")
		return parsePredicates(step, expr)
	}

	// Handle @attr (bare attribute axis — used in field XPaths like "/@href").
	if strings.HasPrefix(expr, "@") {
		// Return a pseudo-step; callers handle attribute extraction at a
		// higher level, but we still need to resolve the parent element.
		// We treat @attr as selecting the parent element itself (the last
		// element step will have matched it).
		step.tag = ""
		return step
	}

	// Split tag from predicates.
	bracketIdx := strings.IndexByte(expr, '[')
	tagPart := expr
	predPart := ""
	if bracketIdx >= 0 {
		tagPart = expr[:bracketIdx]
		predPart = expr[bracketIdx:]
	}

	step.tag = strings.TrimSpace(tagPart)
	if step.tag == "*" {
		step.tag = ""
	}

	return parsePredicates(step, predPart)
}

// parsePredicates extracts [...] predicates from the remainder of a step
// expression.
func parsePredicates(step xpathStep, s string) xpathStep {
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		if len(s) == 0 || s[0] != '[' {
			break
		}
		close := findMatchingBracket(s)
		if close < 0 {
			break
		}
		inner := strings.TrimSpace(s[1:close])
		s = s[close+1:]

		// Position predicate?
		if inner == "last()" {
			step.position = -1
			continue
		}
		if pos, err := strconv.Atoi(inner); err == nil && pos > 0 {
			step.position = pos
			continue
		}

		// Attribute predicate: @attr or @attr="value".
		if strings.HasPrefix(inner, "@") {
			inner = inner[1:]
			if eqIdx := strings.IndexByte(inner, '='); eqIdx >= 0 {
				key := strings.TrimSpace(inner[:eqIdx])
				val := strings.TrimSpace(inner[eqIdx+1:])
				val = strings.Trim(val, `"'`)
				step.attrs = append(step.attrs, xpathAttrPred{key: key, value: val})
			} else {
				step.attrs = append(step.attrs, xpathAttrPred{key: strings.TrimSpace(inner)})
			}
		}
	}
	return step
}

// findMatchingBracket returns the index of the ']' that closes the '[' at
// position 0.
func findMatchingBracket(s string) int {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Step application
// ---------------------------------------------------------------------------

// applyXPathStep applies a single step to each node in the current set and
// returns the combined result.
func applyXPathStep(nodes []*html.Node, step xpathStep) []*html.Node {
	var results []*html.Node

	for _, n := range nodes {
		var candidates []*html.Node

		switch step.axis {
		case axisDescendant:
			walkDescendants(n, func(c *html.Node) {
				if xpathNodeMatches(c, step) {
					candidates = append(candidates, c)
				}
			})
		case axisChild:
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if xpathNodeMatches(c, step) {
					candidates = append(candidates, c)
				}
			}
		}

		candidates = applyPositionPredicate(candidates, step.position)
		results = append(results, candidates...)
	}
	return results
}

// xpathNodeMatches checks whether a single node matches the step's tag and
// attribute predicates (position is handled separately).
func xpathNodeMatches(n *html.Node, step xpathStep) bool {
	if step.textNode {
		return n.Type == html.TextNode && strings.TrimSpace(n.Data) != ""
	}

	if n.Type != html.ElementNode {
		return false
	}

	if step.tag != "" && n.Data != step.tag {
		return false
	}

	for _, pred := range step.attrs {
		if pred.value == "" {
			// Presence check.
			found := false
			for _, a := range n.Attr {
				if a.Key == pred.key {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else {
			if GetAttr(n, pred.key) != pred.value {
				return false
			}
		}
	}
	return true
}

// applyPositionPredicate filters candidates by a [N] or [last()] predicate.
func applyPositionPredicate(nodes []*html.Node, pos int) []*html.Node {
	if pos == 0 || len(nodes) == 0 {
		return nodes
	}
	if pos == -1 {
		return []*html.Node{nodes[len(nodes)-1]}
	}
	if pos > len(nodes) {
		return nil
	}
	return []*html.Node{nodes[pos-1]}
}

