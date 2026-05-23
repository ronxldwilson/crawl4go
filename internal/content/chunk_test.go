package content

import (
	"strings"
	"testing"
)

func TestFixedSizeChunker(t *testing.T) {
	t.Run("empty text returns nil", func(t *testing.T) {
		c := NewFixedSizeChunker(100, 0)
		chunks := c.Chunk("")
		if chunks != nil {
			t.Errorf("expected nil, got %d chunks", len(chunks))
		}
	})

	t.Run("short text single chunk", func(t *testing.T) {
		c := NewFixedSizeChunker(100, 0)
		chunks := c.Chunk("Hello world")
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0].Text != "Hello world" {
			t.Errorf("chunk text = %q, want %q", chunks[0].Text, "Hello world")
		}
		if chunks[0].Index != 0 {
			t.Errorf("chunk index = %d, want 0", chunks[0].Index)
		}
	})

	t.Run("long text splits into multiple chunks", func(t *testing.T) {
		text := strings.Repeat("word ", 100) // 500 chars
		c := NewFixedSizeChunker(100, 0)
		chunks := c.Chunk(text)
		if len(chunks) < 2 {
			t.Errorf("expected multiple chunks, got %d", len(chunks))
		}
		// Verify indices are sequential.
		for i, ch := range chunks {
			if ch.Index != i {
				t.Errorf("chunk %d has Index %d", i, ch.Index)
			}
		}
	})

	t.Run("overlap produces more chunks", func(t *testing.T) {
		text := strings.Repeat("word ", 100)
		noOverlap := NewFixedSizeChunker(100, 0)
		withOverlap := NewFixedSizeChunker(100, 20)
		chunksNo := noOverlap.Chunk(text)
		chunksWith := withOverlap.Chunk(text)
		if len(chunksWith) <= len(chunksNo) {
			t.Errorf("overlap chunks (%d) should be more than no-overlap (%d)", len(chunksWith), len(chunksNo))
		}
	})

	t.Run("metadata is initialized", func(t *testing.T) {
		c := NewFixedSizeChunker(100, 0)
		chunks := c.Chunk("Hello world")
		if chunks[0].Metadata == nil {
			t.Error("expected non-nil Metadata")
		}
	})
}

func TestSlidingWindowChunker(t *testing.T) {
	t.Run("empty text returns nil", func(t *testing.T) {
		c := NewSlidingWindowChunker(10, 5)
		chunks := c.Chunk("")
		if chunks != nil {
			t.Errorf("expected nil, got %d chunks", len(chunks))
		}
	})

	t.Run("short text single chunk", func(t *testing.T) {
		c := NewSlidingWindowChunker(100, 50)
		chunks := c.Chunk("Hello")
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0].Text != "Hello" {
			t.Errorf("chunk text = %q, want %q", chunks[0].Text, "Hello")
		}
	})

	t.Run("produces overlapping windows", func(t *testing.T) {
		text := "abcdefghijklmnopqrstuvwxyz" // 26 chars
		c := NewSlidingWindowChunker(10, 5)
		chunks := c.Chunk(text)
		// Windows: [0:10], [5:15], [10:20], [15:25], [20:26]
		if len(chunks) < 4 {
			t.Errorf("expected at least 4 chunks, got %d", len(chunks))
		}
	})

	t.Run("sequential indices", func(t *testing.T) {
		c := NewSlidingWindowChunker(10, 5)
		chunks := c.Chunk("abcdefghijklmnopqrstuvwxyz")
		for i, ch := range chunks {
			if ch.Index != i {
				t.Errorf("chunk %d has Index %d", i, ch.Index)
			}
		}
	})
}

func TestSemanticChunker(t *testing.T) {
	t.Run("empty text returns nil", func(t *testing.T) {
		c := NewSemanticChunker(100)
		chunks := c.Chunk("")
		if chunks != nil {
			t.Errorf("expected nil, got %d chunks", len(chunks))
		}
	})

	t.Run("single paragraph within limit", func(t *testing.T) {
		c := NewSemanticChunker(200)
		chunks := c.Chunk("This is a single paragraph.")
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
	})

	t.Run("merges small paragraphs", func(t *testing.T) {
		text := "Para one.\n\nPara two.\n\nPara three."
		c := NewSemanticChunker(500)
		chunks := c.Chunk(text)
		if len(chunks) != 1 {
			t.Errorf("expected 1 merged chunk, got %d", len(chunks))
		}
	})

	t.Run("splits at paragraph boundary when over limit", func(t *testing.T) {
		para1 := strings.Repeat("a", 60)
		para2 := strings.Repeat("b", 60)
		text := para1 + "\n\n" + para2
		c := NewSemanticChunker(80)
		chunks := c.Chunk(text)
		if len(chunks) != 2 {
			t.Fatalf("expected 2 chunks, got %d", len(chunks))
		}
	})

	t.Run("heading detected in metadata", func(t *testing.T) {
		text := "# My Heading\n\nSome content under the heading."
		c := NewSemanticChunker(500)
		chunks := c.Chunk(text)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0].Metadata["heading"] != "My Heading" {
			t.Errorf("heading = %q, want %q", chunks[0].Metadata["heading"], "My Heading")
		}
	})
}

func TestMarkdownChunker(t *testing.T) {
	t.Run("empty text returns nil", func(t *testing.T) {
		c := NewMarkdownChunker(100)
		chunks := c.Chunk("")
		if chunks != nil {
			t.Errorf("expected nil, got %d chunks", len(chunks))
		}
	})

	t.Run("splits at header boundaries", func(t *testing.T) {
		text := "# Section 1\n" + strings.Repeat("Content one. ", 10) + "\n\n# Section 2\n" + strings.Repeat("Content two. ", 10)
		c := NewMarkdownChunker(50)
		chunks := c.Chunk(text)
		if len(chunks) < 2 {
			t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
		}
	})

	t.Run("preserves heading in metadata", func(t *testing.T) {
		text := "# Introduction\nSome intro text."
		c := NewMarkdownChunker(500)
		chunks := c.Chunk(text)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0].Metadata["heading"] != "Introduction" {
			t.Errorf("heading = %q, want %q", chunks[0].Metadata["heading"], "Introduction")
		}
	})

	t.Run("code blocks kept intact", func(t *testing.T) {
		text := "# Code\n\n```go\nfunc main() {\n}\n```\n\nAfter code."
		c := NewMarkdownChunker(500)
		chunks := c.Chunk(text)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if !strings.Contains(chunks[0].Text, "func main()") {
			t.Error("code block content missing from chunk")
		}
	})

	t.Run("large section hard-split", func(t *testing.T) {
		text := "# Big Section\n" + strings.Repeat("word ", 200)
		c := NewMarkdownChunker(100)
		chunks := c.Chunk(text)
		if len(chunks) < 2 {
			t.Errorf("expected hard-split into multiple chunks, got %d", len(chunks))
		}
	})
}

func TestDetectHeading(t *testing.T) {
	tests := []struct {
		name string
		para string
		want string
	}{
		{
			name: "markdown h1",
			para: "# Hello World",
			want: "Hello World",
		},
		{
			name: "markdown h2",
			para: "## Sub Heading",
			want: "Sub Heading",
		},
		{
			name: "short line followed by long",
			para: "Title\nThis is a much longer second line that serves as the body.",
			want: "Title",
		},
		{
			name: "no heading detected",
			para: "This is just a regular paragraph with enough text.",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectHeading(tt.para)
			if got != tt.want {
				t.Errorf("detectHeading(%q) = %q, want %q", tt.para, got, tt.want)
			}
		})
	}
}

func TestIsMarkdownHeader(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"# Heading", true},
		{"## Sub", true},
		{"###### Deep", true},
		{"#NoSpace", false},
		{"Not a heading", false},
		{"", false},
		{"  # Indented heading", true},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := isMarkdownHeader(tt.line)
			if got != tt.want {
				t.Errorf("isMarkdownHeader(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
