package content

import (
	"fmt"
	"regexp"
	"strings"
)

// CitationRef maps a citation number to its source URL and anchor text.
type CitationRef struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title,omitempty"`
}

// AddCitations converts inline markdown links [text](url) to citation format
// [text][N] and appends a references section at the bottom.
// Returns the cited markdown and the list of references.
func AddCitations(markdown string) (string, []CitationRef) {
	linkPattern := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

	var refs []CitationRef
	seen := make(map[string]int) // URL -> citation number

	cited := linkPattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		text := parts[1]
		url := parts[2]

		// Skip image links and anchors
		if strings.HasPrefix(url, "#") || strings.HasPrefix(match, "!") {
			return match
		}

		num, ok := seen[url]
		if !ok {
			num = len(refs) + 1
			seen[url] = num
			refs = append(refs, CitationRef{
				Number: num,
				URL:    url,
				Title:  text,
			})
		}

		return fmt.Sprintf("%s [%d]", text, num)
	})

	// Append references section
	if len(refs) > 0 {
		var sb strings.Builder
		sb.WriteString(cited)
		sb.WriteString("\n\n---\n\n## References\n\n")
		for _, ref := range refs {
			sb.WriteString(fmt.Sprintf("[%d] %s", ref.Number, ref.URL))
			if ref.Title != "" && ref.Title != ref.URL {
				sb.WriteString(fmt.Sprintf(" - %s", ref.Title))
			}
			sb.WriteByte('\n')
		}
		cited = sb.String()
	}

	return cited, refs
}
