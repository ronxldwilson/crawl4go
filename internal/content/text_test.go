package content

import (
	"testing"
)

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "tag stripping",
			input: "<p>Hello</p>",
			want:  "Hello",
		},
		{
			name:  "script removal",
			input: "<p>Before</p><script>alert('xss')</script><p>After</p>",
			want:  "Before After",
		},
		{
			name:  "style removal",
			input: "<style>body { color: red; }</style><p>Text</p>",
			want:  "Text",
		},
		{
			name:  "noscript removal",
			input: "<noscript>Enable JS</noscript><p>Content</p>",
			want:  "Content",
		},
		{
			name:  "nested tags",
			input: "<div><p><strong>Bold</strong> text</p></div>",
			want:  "Bold text",
		},
		{
			name:  "whitespace collapse",
			input: "<p>Hello   world</p>",
			want:  "Hello world",
		},
		{
			name:  "leading and trailing trim",
			input: "   <p>Hello</p>   ",
			want:  "Hello",
		},
		{
			name:  "mixed content",
			input: "<h1>Title</h1><p>Paragraph</p>",
			want:  "Title Paragraph",
		},
		{
			name:  "HTML entities are preserved as-is",
			input: "<p>Tom &amp; Jerry</p>",
			want:  "Tom &amp; Jerry",
		},
		{
			name:  "multiline script removed",
			input: "<p>Start</p><script type=\"text/javascript\">\nvar x = 1;\nvar y = 2;\n</script><p>End</p>",
			want:  "Start End",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTMLToText(tt.input)
			if got != tt.want {
				t.Errorf("HTMLToText(%q)\n  got  %q\n  want %q", tt.input, got, tt.want)
			}
		})
	}
}
