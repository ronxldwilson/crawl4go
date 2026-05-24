package content

import (
	"regexp"
	"sync"
)

// PatternEntry describes a named regex pattern with optional validation.
type PatternEntry struct {
	Name        string
	Pattern     string
	Description string
	Validate    func(string) bool
}

var (
	builtinOnce     sync.Once
	builtinPatterns map[string]PatternEntry
)

// BuiltinPatterns returns a catalog of common regex patterns.
func BuiltinPatterns() map[string]PatternEntry {
	builtinOnce.Do(func() {
		builtinPatterns = map[string]PatternEntry{
			"email": {
				Name:        "email",
				Pattern:     `[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`,
				Description: "Email addresses",
				Validate: func(s string) bool {
					return len(s) >= 5 && len(s) <= 254
				},
			},
			"phone": {
				Name:        "phone",
				Pattern:     `(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}|\+\d{1,3}[-.\s]?\d{1,14}`,
				Description: "Phone numbers (US format and international)",
				Validate:    func(s string) bool { return len(s) >= 7 },
			},
			"url": {
				Name:        "url",
				Pattern:     `https?://[^\s<>"` + "`" + `\{\}\|\\\^\[\]]+`,
				Description: "HTTP/HTTPS URLs",
				Validate:    func(s string) bool { return len(s) >= 10 },
			},
			"ipv4": {
				Name:        "ipv4",
				Pattern:     `\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\b`,
				Description: "IPv4 addresses",
				Validate: func(s string) bool {
					return len(s) >= 7 && len(s) <= 15
				},
			},
			"ipv6": {
				Name:        "ipv6",
				Pattern:     `(?i)[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){7}|(?:[0-9a-f]{1,4}:){1,7}:|::(?:[0-9a-f]{1,4}:){0,5}[0-9a-f]{1,4}`,
				Description: "IPv6 addresses (simplified)",
				Validate:    func(s string) bool { return len(s) >= 2 },
			},
			"credit_card": {
				Name:        "credit_card",
				Pattern:     `\b(?:4\d{3}|5[1-5]\d{2}|3[47]\d{2})[-\s]?\d{4,6}[-\s]?\d{4,5}(?:[-\s]?\d{4})?\b`,
				Description: "Credit card numbers (Visa, MC, Amex patterns)",
				Validate: func(s string) bool {
					return len(s) >= 13 && len(s) <= 22
				},
			},
			"date_iso": {
				Name:        "date_iso",
				Pattern:     `\b\d{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])(?:T\d{2}:\d{2}(?::\d{2})?(?:Z|[+\-]\d{2}:?\d{2})?)?\b`,
				Description: "ISO 8601 dates",
				Validate:    func(s string) bool { return len(s) >= 10 },
			},
			"price": {
				Name:        "price",
				Pattern:     `[$€£]\s?\d{1,3}(?:[,.]?\d{3})*(?:[.,]\d{1,2})?|\d{1,3}(?:[,.]?\d{3})*(?:[.,]\d{1,2})?\s?[$€£]`,
				Description: "Currency amounts ($, €, £)",
				Validate:    func(s string) bool { return len(s) >= 2 },
			},
		}
	})
	return builtinPatterns
}

// ExtractAll extracts all matches for the requested patterns from html.
// If no patternNames are provided, all builtin patterns are used.
func ExtractAll(html string, patternNames ...string) map[string][]string {
	catalog := BuiltinPatterns()

	var entries []PatternEntry
	if len(patternNames) == 0 {
		for _, e := range catalog {
			entries = append(entries, e)
		}
	} else {
		for _, name := range patternNames {
			if e, ok := catalog[name]; ok {
				entries = append(entries, e)
			}
		}
	}

	results := make(map[string][]string, len(entries))
	for _, entry := range entries {
		re, err := regexp.Compile(entry.Pattern)
		if err != nil {
			continue
		}
		matches := re.FindAllString(html, -1)
		if entry.Validate != nil {
			filtered := matches[:0]
			for _, m := range matches {
				if entry.Validate(m) {
					filtered = append(filtered, m)
				}
			}
			matches = filtered
		}
		if len(matches) > 0 {
			results[entry.Name] = matches
		}
	}
	return results
}
