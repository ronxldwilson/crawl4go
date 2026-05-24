package content

import "context"

// ExtractionStrategy defines the interface for content extraction from HTML.
type ExtractionStrategy interface {
	// Extract takes HTML content and returns structured extraction results.
	Extract(ctx context.Context, html string, config ExtractionConfig) ([]ExtractionResult, error)
	// Name returns the strategy identifier.
	Name() string
}

// ExtractionConfig holds parameters for extraction.
type ExtractionConfig struct {
	// CSS selector (used by CSSExtractionStrategy).
	Selector string `json:"selector,omitempty"`
	// XPath expression (used by XPathExtractionStrategy).
	XPath string `json:"xpath,omitempty"`
	// Whether to extract nested content.
	Nested bool `json:"nested"`
	// Fields to extract (for structured CSS extraction).
	Fields []FieldConfig `json:"fields,omitempty"`
	// XPathFields to extract (for structured XPath extraction).
	XPathFields []XPathField `json:"xpath_fields,omitempty"`
	// Regex patterns (used by RegexExtractionStrategy).
	Patterns []RegexPattern `json:"patterns,omitempty"`
}

// FieldConfig defines a single field to extract via CSS selector.
type FieldConfig struct {
	Name     string `json:"name"`
	Selector string `json:"selector"`
	Type     string `json:"type"`           // text, html, attribute, list, nested
	Attr     string `json:"attr,omitempty"` // attribute name when Type is "attribute"
}

// ExtractionResult holds a single extraction output.
type ExtractionResult struct {
	Content string         `json:"content"`
	Fields  map[string]any `json:"fields,omitempty"`
	Index   int            `json:"index"`
}

// ---------------------------------------------------------------------------
// CSS strategy
// ---------------------------------------------------------------------------

// CSSExtractionStrategy extracts content using CSS selectors.
// It wraps the existing CSSExtractor.
type CSSExtractionStrategy struct{}

func (s *CSSExtractionStrategy) Name() string { return "css" }

func (s *CSSExtractionStrategy) Extract(_ context.Context, html string, config ExtractionConfig) ([]ExtractionResult, error) {
	fields := make([]ExtractionField, len(config.Fields))
	for i, f := range config.Fields {
		fields[i] = ExtractionField{
			Name:      f.Name,
			Selector:  f.Selector,
			Type:      f.Type,
			Attribute: f.Attr,
		}
	}

	extractor := NewCSSExtractor(ExtractionSchema{
		BaseSelector: config.Selector,
		Fields:       fields,
	})

	matches, err := extractor.Extract(html)
	if err != nil {
		return nil, err
	}

	out := make([]ExtractionResult, len(matches))
	for i, m := range matches {
		out[i] = ExtractionResult{Fields: m, Index: i}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// XPath strategy
// ---------------------------------------------------------------------------

// XPathExtractionStrategy extracts content using XPath expressions.
// It wraps the existing XPathExtractor.
type XPathExtractionStrategy struct{}

func (s *XPathExtractionStrategy) Name() string { return "xpath" }

func (s *XPathExtractionStrategy) Extract(_ context.Context, html string, config ExtractionConfig) ([]ExtractionResult, error) {
	extractor := NewXPathExtractor(XPathExtractionSchema{
		BaseXPath: config.XPath,
		Fields:    config.XPathFields,
	})

	matches, err := extractor.Extract(html)
	if err != nil {
		return nil, err
	}

	out := make([]ExtractionResult, len(matches))
	for i, m := range matches {
		out[i] = ExtractionResult{Fields: m, Index: i}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Regex strategy
// ---------------------------------------------------------------------------

// RegexExtractionStrategy extracts content using regular expressions.
// It wraps the existing RegexExtractor.
type RegexExtractionStrategy struct{}

func (s *RegexExtractionStrategy) Name() string { return "regex" }

func (s *RegexExtractionStrategy) Extract(_ context.Context, html string, config ExtractionConfig) ([]ExtractionResult, error) {
	extractor := NewRegexExtractor(RegexExtractionSchema{
		Patterns: config.Patterns,
	})

	matches, err := extractor.Extract(html)
	if err != nil {
		return nil, err
	}

	out := make([]ExtractionResult, len(matches))
	for i, m := range matches {
		out[i] = ExtractionResult{Fields: m, Index: i}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Registry helper
// ---------------------------------------------------------------------------

// StrategyRegistry maps strategy names to their implementations.
var StrategyRegistry = map[string]ExtractionStrategy{
	"css":   &CSSExtractionStrategy{},
	"xpath": &XPathExtractionStrategy{},
	"regex": &RegexExtractionStrategy{},
}

// GetStrategy returns the ExtractionStrategy registered under the given name,
// or nil if no such strategy exists.
func GetStrategy(name string) ExtractionStrategy {
	return StrategyRegistry[name]
}
