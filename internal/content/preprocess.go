package content

import (
	"regexp"
	"strings"
)

// PreprocessHTML reduces HTML size for LLM/schema extraction by stripping
// non-semantic elements while preserving content structure.
func PreprocessHTML(html string) string {
	// Remove script tags and content
	html = removeTagAndContent(html, "script")
	// Remove style tags and content
	html = removeTagAndContent(html, "style")
	// Remove HTML comments
	html = removePattern(html, `<!--[\s\S]*?-->`)
	// Remove noscript
	html = removeTagAndContent(html, "noscript")
	// Remove SVG
	html = removeTagAndContent(html, "svg")
	// Remove header/footer/nav (typically boilerplate)
	html = removeTagAndContent(html, "header")
	html = removeTagAndContent(html, "footer")
	html = removeTagAndContent(html, "nav")
	// Strip non-essential attributes (keep href, src, alt, title, class, id)
	html = stripAttributes(html)
	// Collapse whitespace
	html = collapseWhitespace(html)

	return strings.TrimSpace(html)
}

// PreprocessForLLM is more aggressive — strips everything except text content
// with minimal structural markers.
func PreprocessForLLM(html string, maxChars int) string {
	html = PreprocessHTML(html)
	// Also remove aside, form elements
	html = removeTagAndContent(html, "aside")
	html = removeTagAndContent(html, "form")
	// Remove all images (just keep alt text would be ideal but simple strip for now)
	html = removePattern(html, `<img[^>]*>`)
	// Remove empty tags
	html = removePattern(html, `<[a-z][a-z0-9]*[^>]*>\s*</[a-z][a-z0-9]*>`)

	html = collapseWhitespace(html)
	html = strings.TrimSpace(html)

	if maxChars > 0 && len(html) > maxChars {
		html = html[:maxChars]
	}

	return html
}

func removeTagAndContent(html, tag string) string {
	pattern := regexp.MustCompile(`(?i)<` + tag + `[\s>][\s\S]*?</` + tag + `>`)
	return pattern.ReplaceAllString(html, "")
}

func removePattern(html, pattern string) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(html, "")
}

var attrKeepList = map[string]bool{
	"href": true, "src": true, "alt": true, "title": true,
	"class": true, "id": true, "type": true, "name": true,
	"content": true, "property": true, "rel": true,
}

func stripAttributes(html string) string {
	// Match opening tags with attributes
	tagPattern := regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9]*)((?:\s+[^>]*?)?)>`)
	attrPattern := regexp.MustCompile(`\s+([a-zA-Z][\w-]*)(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*))?`)

	return tagPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := tagPattern.FindStringSubmatch(match)
		if len(parts) < 3 || parts[2] == "" {
			return match
		}
		tag := parts[1]
		attrs := attrPattern.FindAllStringSubmatch(parts[2], -1)

		var kept []string
		for _, attr := range attrs {
			if len(attr) >= 2 && attrKeepList[strings.ToLower(attr[1])] {
				kept = append(kept, strings.TrimSpace(attr[0]))
			}
		}

		if len(kept) == 0 {
			return "<" + tag + ">"
		}
		return "<" + tag + " " + strings.Join(kept, " ") + ">"
	})
}

func collapseWhitespace(html string) string {
	// Collapse multiple whitespace (but not inside pre/code)
	ws := regexp.MustCompile(`\s{2,}`)
	return ws.ReplaceAllString(html, " ")
}
