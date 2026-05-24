package content

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// SchemaField describes how to extract a single named value from an element.
type SchemaField struct {
	Name      string              // output key
	Selector  string              // CSS selector, XPath, or regex pattern
	Type      string              // "css", "xpath", or "regex"
	Attribute string              // attribute to extract (for css/xpath); empty means text content
	Transform func(string) string // optional post-processing
	IsList    bool                // if true, collect all matches as []string
	Children  []SchemaField       // nested fields (extracted from each matched child)
}

// EngineSchema ties a base selector to a set of SchemaFields.
// BaseSelector is a CSS selector used to find row elements.
type EngineSchema struct {
	BaseSelector string
	Fields       []SchemaField
}

// SchemaEngine extracts structured data from HTML using a flexible schema
// that supports CSS, XPath, and regex field types.
type SchemaEngine struct{}

// Extract applies the schema to htmlContent: finds all elements matching
// BaseSelector, then extracts each field from every matched element.
func (se *SchemaEngine) Extract(htmlContent string, schema EngineSchema) ([]map[string]any, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("schemaengine: parse html: %w", err)
	}

	roots := cssSelect(doc, schema.BaseSelector)
	if len(roots) == 0 {
		return nil, nil
	}

	results := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		obj, err := se.extractSchemaFields(root, schema.Fields)
		if err != nil {
			return nil, err
		}
		results = append(results, obj)
	}
	return results, nil
}

func (se *SchemaEngine) extractSchemaFields(node *html.Node, fields []SchemaField) (map[string]any, error) {
	obj := make(map[string]any, len(fields))
	for _, f := range fields {
		val, err := se.extractSchemaField(node, f)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}
		obj[f.Name] = val
	}
	return obj, nil
}

func (se *SchemaEngine) extractSchemaField(node *html.Node, f SchemaField) (any, error) {
	switch f.Type {
	case "css":
		return se.extractCSS(node, f)
	case "xpath":
		return se.extractXPath(node, f)
	case "regex":
		return se.extractRegex(node, f)
	default:
		return nil, fmt.Errorf("unknown field type %q", f.Type)
	}
}

func (se *SchemaEngine) extractCSS(node *html.Node, f SchemaField) (any, error) {
	matches := cssSelect(node, f.Selector)

	// Handle children (nested extraction).
	if len(f.Children) > 0 {
		var nested []map[string]any
		for _, m := range matches {
			child, err := se.extractSchemaFields(m, f.Children)
			if err != nil {
				return nil, err
			}
			nested = append(nested, child)
		}
		if f.IsList {
			return nested, nil
		}
		if len(nested) > 0 {
			return nested[0], nil
		}
		return map[string]any{}, nil
	}

	if f.IsList {
		items := make([]string, 0, len(matches))
		for _, m := range matches {
			items = append(items, se.nodeValue(m, f))
		}
		return items, nil
	}

	if len(matches) == 0 {
		return "", nil
	}
	return se.nodeValue(matches[0], f), nil
}

func (se *SchemaEngine) extractXPath(node *html.Node, f SchemaField) (any, error) {
	matches := xpathSelect(node, f.Selector)

	if len(f.Children) > 0 {
		var nested []map[string]any
		for _, m := range matches {
			child, err := se.extractSchemaFields(m, f.Children)
			if err != nil {
				return nil, err
			}
			nested = append(nested, child)
		}
		if f.IsList {
			return nested, nil
		}
		if len(nested) > 0 {
			return nested[0], nil
		}
		return map[string]any{}, nil
	}

	if f.IsList {
		items := make([]string, 0, len(matches))
		for _, m := range matches {
			items = append(items, se.nodeValue(m, f))
		}
		return items, nil
	}

	if len(matches) == 0 {
		return "", nil
	}
	return se.nodeValue(matches[0], f), nil
}

func (se *SchemaEngine) extractRegex(node *html.Node, f SchemaField) (any, error) {
	text := ExtractText(node)

	re, err := regexp.Compile(f.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid regex %q: %w", f.Selector, err)
	}

	if f.IsList {
		allMatches := re.FindAllString(text, -1)
		items := make([]string, 0, len(allMatches))
		for _, m := range allMatches {
			if f.Transform != nil {
				m = f.Transform(m)
			}
			items = append(items, m)
		}
		return items, nil
	}

	match := re.FindString(text)
	if f.Transform != nil {
		match = f.Transform(match)
	}
	return match, nil
}

// nodeValue extracts a string value from a node, optionally reading an
// attribute and applying a transform.
func (se *SchemaEngine) nodeValue(n *html.Node, f SchemaField) string {
	var val string
	if f.Attribute != "" {
		val = GetAttr(n, f.Attribute)
	} else {
		val = ExtractText(n)
	}
	if f.Transform != nil {
		val = f.Transform(val)
	}
	return val
}
