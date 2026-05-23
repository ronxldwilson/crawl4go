package main

import (
	"fmt"
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

var inlineLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// HTMLToMarkdown converts HTML content to Markdown with optional citation-style links.
func HTMLToMarkdown(htmlContent string, baseURL string) string {
	md, err := htmltomarkdown.ConvertString(htmlContent)
	if err != nil {
		return HTMLToText(htmlContent)
	}

	md = convertToCitations(md)
	return strings.TrimSpace(md)
}

func convertToCitations(markdown string) string {
	urls := make(map[string]int)
	var refs []string
	counter := 0

	result := inlineLinkRe.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := inlineLinkRe.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		text := parts[1]
		href := parts[2]

		num, exists := urls[href]
		if !exists {
			counter++
			num = counter
			urls[href] = num
			refs = append(refs, fmt.Sprintf("[%d]: %s", num, href))
		}

		return fmt.Sprintf("%s [%d]", text, num)
	})

	if len(refs) > 0 {
		result += "\n\n## References\n\n" + strings.Join(refs, "\n")
	}

	return result
}
