package content

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// ExtractionField describes how to extract a single named value from an element.
type ExtractionField struct {
	Name      string            `json:"name"`
	Selector  string            `json:"selector"`
	Type      string            `json:"type"`      // text | attribute | html | nested | list
	Attribute string            `json:"attribute,omitempty"`
	Fields    []ExtractionField `json:"fields,omitempty"`
}

// ExtractionSchema ties a base CSS selector to a set of fields to extract from
// each matched element.
type ExtractionSchema struct {
	BaseSelector string            `json:"base_selector"`
	Fields       []ExtractionField `json:"fields"`
}

// CSSExtractor performs structured CSS-selector-based extraction.
type CSSExtractor struct {
	Schema ExtractionSchema
}

// NewCSSExtractor creates a CSSExtractor for the given schema.
func NewCSSExtractor(schema ExtractionSchema) *CSSExtractor {
	return &CSSExtractor{Schema: schema}
}

// Extract parses htmlContent, finds every element matching Schema.BaseSelector,
// and returns a slice of objects built according to Schema.Fields.
func (e *CSSExtractor) Extract(htmlContent string) ([]map[string]any, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	roots := cssSelect(doc, e.Schema.BaseSelector)
	results := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		obj := extractFields(root, e.Schema.Fields)
		results = append(results, obj)
	}
	return results, nil
}

// extractFields builds a map by applying each field's selector+type against node.
func extractFields(node *html.Node, fields []ExtractionField) map[string]any {
	obj := make(map[string]any, len(fields))
	for _, f := range fields {
		obj[f.Name] = extractField(node, f)
	}
	return obj
}

func extractField(node *html.Node, f ExtractionField) any {
	switch f.Type {
	case "list":
		var matches []*html.Node
		if f.Selector != "" {
			matches = cssSelect(node, f.Selector)
		} else {
			matches = []*html.Node{node}
		}
		items := make([]string, 0, len(matches))
		for _, m := range matches {
			items = append(items, ExtractText(m))
		}
		return items

	case "nested":
		var target *html.Node
		if f.Selector != "" {
			target = cssSelectFirst(node, f.Selector)
		} else {
			target = node
		}
		if target == nil || len(f.Fields) == 0 {
			return map[string]any{}
		}
		return extractFields(target, f.Fields)

	case "attribute":
		target := cssSelectFirst(node, f.Selector)
		if target == nil {
			return ""
		}
		return GetAttr(target, f.Attribute)

	case "html":
		target := cssSelectFirst(node, f.Selector)
		if target == nil {
			return ""
		}
		return renderHTML(target)

	default: // "text" and anything unrecognised
		target := cssSelectFirst(node, f.Selector)
		if target == nil {
			return ""
		}
		return ExtractText(target)
	}
}

// renderHTML serialises a node back to an HTML string.
func renderHTML(n *html.Node) string {
	var buf bytes.Buffer
	_ = html.Render(&buf, n)
	return buf.String()
}

// ---------------------------------------------------------------------------
// CSS selector engine
// ---------------------------------------------------------------------------

// cssSelect returns all descendants of root that match selector.
// Supports:
//   - tag names:          div
//   - class:              .name
//   - id:                 #name
//   - attribute present:  [attr]
//   - attribute value:    [attr=value]
//   - descendant:         div p
//   - direct child:       div > p
//   - comma (union):      a, b
func cssSelect(root *html.Node, selector string) []*html.Node {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil
	}

	// Handle comma-separated selector list.
	if parts := splitTopLevel(selector, ','); len(parts) > 1 {
		seen := map[*html.Node]bool{}
		var all []*html.Node
		for _, part := range parts {
			for _, n := range cssSelect(root, strings.TrimSpace(part)) {
				if !seen[n] {
					seen[n] = true
					all = append(all, n)
				}
			}
		}
		return all
	}

	chain := parseChain(selector)
	if len(chain) == 0 {
		return nil
	}

	var results []*html.Node
	walkDescendants(root, func(n *html.Node) {
		if matchChain(n, chain) {
			results = append(results, n)
		}
	})
	return results
}

// cssSelectFirst returns the first descendant of root matching selector, or nil.
func cssSelectFirst(root *html.Node, selector string) *html.Node {
	nodes := cssSelect(root, selector)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

// ---------------------------------------------------------------------------
// Chain / combinator parsing
// ---------------------------------------------------------------------------

type combinator byte

const (
	combDescendant combinator = ' '
	combChild      combinator = '>'
)

type chainStep struct {
	comb     combinator // combinator BEFORE this simple selector (ignored for first)
	simple   simpleSelector
}

type simpleSelector struct {
	tag        string   // "" = any
	id         string
	classes    []string
	attrs      []attrSel
}

type attrSel struct {
	key   string
	value string // "" = presence check only
}

// parseChain splits a selector like "div > p span" into a slice of chainSteps.
func parseChain(selector string) []chainStep {
	// Tokenise respecting [ ] brackets.
	tokens := tokeniseChain(selector)
	if len(tokens) == 0 {
		return nil
	}

	var steps []chainStep
	comb := combDescendant
	firstStep := true

	for i := 0; i < len(tokens); i++ {
		tok := strings.TrimSpace(tokens[i])
		if tok == "" {
			continue
		}
		if tok == ">" {
			comb = combChild
			continue
		}
		simple, ok := parseSimple(tok)
		if !ok {
			return nil
		}
		step := chainStep{simple: simple}
		if !firstStep {
			step.comb = comb
		}
		steps = append(steps, step)
		comb = combDescendant
		firstStep = false
	}
	return steps
}

// tokeniseChain splits on whitespace and '>' while keeping '[...]' intact.
func tokeniseChain(sel string) []string {
	var tokens []string
	var cur strings.Builder
	depth := 0
	for _, ch := range sel {
		switch {
		case ch == '[':
			depth++
			cur.WriteRune(ch)
		case ch == ']':
			depth--
			cur.WriteRune(ch)
		case depth > 0:
			cur.WriteRune(ch)
		case ch == '>':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, ">")
		case ch == ' ' || ch == '\t' || ch == '\n':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// parseSimple parses a single simple selector like "div.foo#bar[href]".
func parseSimple(s string) (simpleSelector, bool) {
	var ss simpleSelector
	i := 0
	n := len(s)

	// Leading tag name (characters that are not '.', '#', '[').
	tagEnd := i
	for tagEnd < n && s[tagEnd] != '.' && s[tagEnd] != '#' && s[tagEnd] != '[' {
		tagEnd++
	}
	ss.tag = strings.ToLower(s[i:tagEnd])
	i = tagEnd

	for i < n {
		switch s[i] {
		case '.':
			i++
			end := nextSpecial(s, i)
			if end == i {
				return ss, false
			}
			ss.classes = append(ss.classes, s[i:end])
			i = end

		case '#':
			i++
			end := nextSpecial(s, i)
			if end == i {
				return ss, false
			}
			ss.id = s[i:end]
			i = end

		case '[':
			i++
			close := strings.Index(s[i:], "]")
			if close < 0 {
				return ss, false
			}
			inner := s[i : i+close]
			i += close + 1
			if eq := strings.Index(inner, "="); eq >= 0 {
				ss.attrs = append(ss.attrs, attrSel{
					key:   strings.TrimSpace(inner[:eq]),
					value: strings.Trim(strings.TrimSpace(inner[eq+1:]), `"'`),
				})
			} else {
				ss.attrs = append(ss.attrs, attrSel{key: strings.TrimSpace(inner)})
			}

		default:
			return ss, false
		}
	}
	return ss, true
}

// nextSpecial returns the index of the next '.', '#', '[' after position i.
func nextSpecial(s string, i int) int {
	for i < len(s) && s[i] != '.' && s[i] != '#' && s[i] != '[' {
		i++
	}
	return i
}

// ---------------------------------------------------------------------------
// Matching
// ---------------------------------------------------------------------------

// matchChain checks whether node n satisfies the full selector chain.
func matchChain(n *html.Node, chain []chainStep) bool {
	if len(chain) == 0 {
		return false
	}
	// Match from the rightmost step backwards.
	cur := n
	for idx := len(chain) - 1; idx >= 0; idx-- {
		step := chain[idx]
		if !matchSimple(cur, step.simple) {
			return false
		}
		if idx == 0 {
			break // no more combinators to check
		}
		// Move cur to an ancestor according to the combinator of the CURRENT step
		// (which was set when we parsed the step after it).
		switch step.comb {
		case combChild:
			if cur.Parent == nil {
				return false
			}
			cur = cur.Parent
		default: // combDescendant
			if cur.Parent == nil {
				return false
			}
			cur = cur.Parent
			// For descendant we need the previous step to match somewhere up the tree.
			// We'll try each ancestor.
			prevStep := chain[idx-1]
			found := false
			for anc := cur; anc != nil; anc = anc.Parent {
				if matchSimple(anc, prevStep.simple) {
					// Now we need to verify the remainder of the chain above prevStep.
					// Re-enter matchChain with the prefix up to idx-1.
					if idx-1 == 0 || matchChain(anc, chain[:idx]) {
						cur = anc
						found = true
						break
					}
				}
			}
			if !found {
				return false
			}
			idx-- // skip the step we just resolved
		}
	}
	return true
}

// matchSimple checks a single simple selector against a node.
func matchSimple(n *html.Node, ss simpleSelector) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if ss.tag != "" && ss.tag != "*" && n.Data != ss.tag {
		return false
	}
	if ss.id != "" && GetAttr(n, "id") != ss.id {
		return false
	}
	if len(ss.classes) > 0 {
		cls := " " + GetAttr(n, "class") + " "
		for _, c := range ss.classes {
			if !strings.Contains(cls, " "+c+" ") {
				return false
			}
		}
	}
	for _, a := range ss.attrs {
		val := GetAttr(n, a.key)
		if a.value == "" {
			// Presence check: attribute must exist (non-empty value or explicitly set).
			found := false
			for _, attr := range n.Attr {
				if attr.Key == a.key {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else {
			if val != a.value {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Tree walk helpers
// ---------------------------------------------------------------------------

func walkDescendants(root *html.Node, fn func(*html.Node)) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			fn(c)
			walk(c)
		}
	}
	walk(root)
}

// splitTopLevel splits s on sep only when not inside brackets (e.g. '[...]').
func splitTopLevel(s string, sep rune) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + len(string(sep))
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}
