package content

import (
	"testing"
)

func TestHTMLToText(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "empty string",
			html: "",
			want: "",
		},
		{
			name: "plain text passthrough",
			html: "just plain text",
			want: "just plain text",
		},
		{
			name: "simple tags stripped",
			html: "<p>Hello <b>world</b></p>",
			want: "Hello world",
		},
		{
			name: "script tag removed",
			html: `<p>Before</p><script>alert("x")</script><p>After</p>`,
			want: "Before After",
		},
		{
			name: "style tag removed",
			html: `<p>Before</p><style>body{color:red}</style><p>After</p>`,
			want: "Before After",
		},
		{
			name: "noscript tag removed",
			html: `<p>Visible</p><noscript>Hidden</noscript><p>Also Visible</p>`,
			want: "Visible Also Visible",
		},
		{
			name: "multiple whitespace collapsed",
			html: "<p>Hello    world</p>",
			want: "Hello world",
		},
		{
			name: "nested tags",
			html: "<div><p>Hello <span>beautiful <em>world</em></span></p></div>",
			want: "Hello beautiful world",
		},
		{
			name: "multiline script removed",
			html: "<div>Keep</div><script type=\"text/javascript\">\nvar x = 1;\nvar y = 2;\n</script><div>This</div>",
			want: "Keep This",
		},
		{
			name: "self-closing tags",
			html: "Line one<br/>Line two<hr/>End",
			want: "Line one Line two End",
		},
		{
			name: "attributes preserved in output text",
			html: `<a href="http://example.com">Click here</a>`,
			want: "Click here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HTMLToText(tt.html)
			if got != tt.want {
				t.Errorf("HTMLToText() = %q, want %q", got, tt.want)
			}
		})
	}
}
