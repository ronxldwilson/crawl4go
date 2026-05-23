package content

import (
	"strings"
	"testing"
)

func TestHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		contains []string
	}{
		{
			name:     "basic paragraph",
			html:     `<p>Hello world</p>`,
			contains: []string{"Hello world"},
		},
		{
			name: "link converted to citation",
			html: `<p>Visit <a href="https://example.com">Example</a></p>`,
			contains: []string{
				"Example [1]",
				"[1]: https://example.com",
			},
		},
		{
			name: "headers converted",
			html: `<h1>Title</h1><h2>Subtitle</h2>`,
			contains: []string{
				"# Title",
				"## Subtitle",
			},
		},
		{
			name:     "empty input",
			html:     "",
			contains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTMLToMarkdown(tt.html, "")
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("result does not contain %q\ngot: %s", want, result)
				}
			}
		})
	}
}
