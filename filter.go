package main

import (
	"net/url"
	"path"
	"regexp"
	"strings"
)

type URLFilter interface {
	Apply(rawURL string) bool
}

type FilterChain struct {
	Filters []URLFilter
}

func (fc *FilterChain) Apply(rawURL string) bool {
	for _, f := range fc.Filters {
		if !f.Apply(rawURL) {
			return false
		}
	}
	return true
}

// URLPatternFilter accepts URLs matching any of the given glob/regex patterns.
type URLPatternFilter struct {
	patterns []*regexp.Regexp
}

func NewURLPatternFilter(patterns []string) *URLPatternFilter {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		// Convert simple glob patterns to regex
		re := globToRegex(p)
		if r, err := regexp.Compile(re); err == nil {
			compiled = append(compiled, r)
		}
	}
	return &URLPatternFilter{patterns: compiled}
}

func (f *URLPatternFilter) Apply(rawURL string) bool {
	if len(f.patterns) == 0 {
		return true
	}
	for _, p := range f.patterns {
		if p.MatchString(rawURL) {
			return true
		}
	}
	return false
}

// DomainFilter blocks or allows specific domains.
type DomainFilter struct {
	blocked map[string]bool
	allowed map[string]bool
}

func NewDomainFilter(blocked, allowed []string) *DomainFilter {
	f := &DomainFilter{
		blocked: make(map[string]bool),
		allowed: make(map[string]bool),
	}
	for _, d := range blocked {
		f.blocked[strings.ToLower(d)] = true
	}
	for _, d := range allowed {
		f.allowed[strings.ToLower(d)] = true
	}
	return f
}

func (f *DomainFilter) Apply(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())

	if len(f.blocked) > 0 {
		if f.blocked[host] {
			return false
		}
		for d := range f.blocked {
			if strings.HasSuffix(host, "."+d) {
				return false
			}
		}
	}

	if len(f.allowed) > 0 {
		if f.allowed[host] {
			return true
		}
		for d := range f.allowed {
			if strings.HasSuffix(host, "."+d) {
				return true
			}
		}
		return false
	}

	return true
}

// ContentTypeFilter filters by file extension.
type ContentTypeFilter struct {
	allowed map[string]bool
}

func NewContentTypeFilter(extensions []string) *ContentTypeFilter {
	f := &ContentTypeFilter{allowed: make(map[string]bool)}
	for _, ext := range extensions {
		if !strings.HasPrefix(ext, ".") && ext != "" {
			ext = "." + ext
		}
		f.allowed[strings.ToLower(ext)] = true
	}
	return f
}

func (f *ContentTypeFilter) Apply(rawURL string) bool {
	if len(f.allowed) == 0 {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	ext := strings.ToLower(path.Ext(u.Path))
	// No extension = likely HTML page
	if ext == "" {
		return f.allowed[""]
	}
	return f.allowed[ext]
}

func BuildFilterChain(config *FilterConfig) *FilterChain {
	var filters []URLFilter

	if len(config.URLPatterns) > 0 {
		filters = append(filters, NewURLPatternFilter(config.URLPatterns))
	}
	if len(config.BlockedDomains) > 0 || len(config.AllowedDomains) > 0 {
		filters = append(filters, NewDomainFilter(config.BlockedDomains, config.AllowedDomains))
	}
	if len(config.AllowedExtensions) > 0 {
		filters = append(filters, NewContentTypeFilter(config.AllowedExtensions))
	}

	return &FilterChain{Filters: filters}
}

func globToRegex(pattern string) string {
	var sb strings.Builder
	sb.WriteString("(?i)")
	for _, ch := range pattern {
		switch ch {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		case '.', '(', ')', '+', '|', '^', '$', '[', ']', '{', '}', '\\':
			sb.WriteRune('\\')
			sb.WriteRune(ch)
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}
