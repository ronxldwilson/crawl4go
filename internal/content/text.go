package content

import (
	"regexp"
	"strings"
)

var (
	HtmlTagRe     = regexp.MustCompile(`<[^>]*>`)
	WhitespaceRe  = regexp.MustCompile(`\s+`)
	ScriptStyleRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</\1>`)
)

func HTMLToText(htmlContent string) string {
	text := ScriptStyleRe.ReplaceAllString(htmlContent, " ")
	text = HtmlTagRe.ReplaceAllString(text, " ")
	text = WhitespaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}
