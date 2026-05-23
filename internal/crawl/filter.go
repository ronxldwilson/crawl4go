package crawl

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
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

type URLPatternFilter struct {
	patterns []*regexp.Regexp
}

func NewURLPatternFilter(patterns []string) *URLPatternFilter {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
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

// MIMETypeFilter issues a HEAD request to check the live Content-Type of a URL
// before it is fetched. It implements URLFilter so it can be appended to any
// FilterChain manually:
//
//	chain.Filters = append(chain.Filters, NewMIMETypeFilter(allowed, blocked, client))
type MIMETypeFilter struct {
	allowedTypes []string
	blockedTypes []string
	client       *http.Client
	timeout      time.Duration
}

// NewMIMETypeFilter creates a MIMETypeFilter with a 5-second HEAD-request timeout.
// Pass nil for client to use http.DefaultClient.
func NewMIMETypeFilter(allowed, blocked []string, client *http.Client) *MIMETypeFilter {
	if client == nil {
		client = http.DefaultClient
	}
	return &MIMETypeFilter{
		allowedTypes: allowed,
		blockedTypes: blocked,
		client:       client,
		timeout:      5 * time.Second,
	}
}

// Apply issues a HEAD request to rawURL and inspects the Content-Type header.
//   - Returns false if the content-type has a prefix matching any blockedTypes entry.
//   - Returns false if allowedTypes is non-empty and no entry is a prefix of the
//     content-type (i.e., it is not in the allow-list).
//   - Returns true on any network error or timeout (permissive default).
func (f *MIMETypeFilter) Apply(rawURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return true // malformed URL — let downstream decide
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return true // network error or timeout — permissive
	}
	resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	// Strip parameters (e.g. "text/html; charset=utf-8" → "text/html").
	if idx := strings.IndexByte(ct, ';'); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}

	for _, blocked := range f.blockedTypes {
		if strings.HasPrefix(ct, strings.ToLower(blocked)) {
			return false
		}
	}

	if len(f.allowedTypes) > 0 {
		for _, allowed := range f.allowedTypes {
			if strings.HasPrefix(ct, strings.ToLower(allowed)) {
				return true
			}
		}
		return false
	}

	return true
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
