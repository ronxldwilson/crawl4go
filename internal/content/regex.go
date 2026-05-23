package content

import (
	"fmt"
	"regexp"
)

// ---------------------------------------------------------------------------
// Schema types
// ---------------------------------------------------------------------------

// RegexPattern describes a single named regex pattern and which capture group
// to extract from each match.
type RegexPattern struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Group   int    `json:"group"` // 0 = full match, >0 = numbered capture group
}

// RegexExtractionSchema holds a collection of patterns to apply.
type RegexExtractionSchema struct {
	Patterns []RegexPattern `json:"patterns"`
}

// RegexExtractor performs regex-based extraction over raw text (or HTML
// source).
type RegexExtractor struct {
	Schema RegexExtractionSchema
}

// NewRegexExtractor creates a RegexExtractor for the given schema.
func NewRegexExtractor(schema RegexExtractionSchema) *RegexExtractor {
	return &RegexExtractor{Schema: schema}
}

// Extract runs every pattern in the schema against text and returns all
// matches.  Each match becomes a map with at least the pattern's Name mapped
// to the selected capture group.  When named capture groups ((?P<name>...))
// are present, their names and values are added to the map automatically.
func (e *RegexExtractor) Extract(text string) ([]map[string]any, error) {
	var results []map[string]any

	for _, pat := range e.Schema.Patterns {
		re, err := regexp.Compile(pat.Pattern)
		if err != nil {
			return nil, fmt.Errorf("regex pattern %q: %w", pat.Name, err)
		}

		matches := re.FindAllStringSubmatch(text, -1)
		subNames := re.SubexpNames() // index 0 is always ""

		for _, match := range matches {
			obj := make(map[string]any)

			// Primary value: the requested capture group (or full match).
			group := pat.Group
			if group < 0 || group >= len(match) {
				group = 0
			}
			obj[pat.Name] = match[group]

			// Auto-populate named capture groups.
			for i, name := range subNames {
				if name == "" || i >= len(match) {
					continue
				}
				obj[name] = match[i]
			}

			results = append(results, obj)
		}
	}

	return results, nil
}
