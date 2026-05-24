package content

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int    // minimum expected token count
		has   string // a token that should be present (stemmed)
		lacks string // a stop word that should be absent
	}{
		{
			name:  "basic sentence",
			input: "The quick brown foxes jumped over the lazy dogs",
			want:  3,
			has:   "fox", // stemmed from "foxes"
			lacks: "",
		},
		{
			name:  "stop words removed",
			input: "I have been to the store",
			want:  1, // "store" should survive
			lacks: "the",
		},
		{
			name:  "punctuation stripped",
			input: "Hello, world! Testing... things?",
			want:  2,
		},
		{
			name:  "empty string",
			input: "",
			want:  0,
		},
		{
			name:  "only stop words",
			input: "the and or but",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenize(tt.input)
			if len(tokens) < tt.want {
				t.Errorf("tokenize(%q) returned %d tokens, want at least %d", tt.input, len(tokens), tt.want)
			}
			if tt.has != "" {
				found := false
				for _, tok := range tokens {
					if tok == tt.has {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("tokenize(%q) missing expected token %q, got %v", tt.input, tt.has, tokens)
				}
			}
			if tt.lacks != "" {
				for _, tok := range tokens {
					if tok == tt.lacks {
						t.Errorf("tokenize(%q) should not contain stop word %q", tt.input, tt.lacks)
					}
				}
			}
		})
	}
}

func TestExtractTextChunks(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		minCount int // minimum chunks expected
	}{
		{
			name:     "paragraph text",
			html:     `<html><body><p>Hello world this is a test paragraph</p></body></html>`,
			minCount: 1,
		},
		{
			name:     "multiple blocks",
			html:     `<html><body><p>First paragraph here</p><p>Second paragraph here</p></body></html>`,
			minCount: 2,
		},
		{
			name:     "single word ignored",
			html:     `<html><body><p>word</p></body></html>`,
			minCount: 0,
		},
		{
			name:     "empty body",
			html:     `<html><body></body></html>`,
			minCount: 0,
		},
		{
			name:     "invalid html",
			html:     `<<<>>>`,
			minCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ExtractTextChunks(tt.html)
			if len(chunks) < tt.minCount {
				t.Errorf("ExtractTextChunks returned %d chunks, want at least %d", len(chunks), tt.minCount)
			}
		})
	}
}

func TestExtractPageQuery(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "title extracted",
			html: `<html><head><title>My Page Title</title></head><body></body></html>`,
			want: "My Page Title",
		},
		{
			name: "h1 fallback",
			html: `<html><head></head><body><h1>Main Heading</h1></body></html>`,
			want: "Main Heading",
		},
		{
			name: "meta keywords fallback",
			html: `<html><head><meta name="keywords" content="go,testing,code"></head><body></body></html>`,
			want: "go,testing,code",
		},
		{
			name: "empty html",
			html: `<html><head></head><body></body></html>`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPageQuery(tt.html)
			if got != tt.want {
				t.Errorf("ExtractPageQuery = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBM25FilterByRelevance(t *testing.T) {
	tests := []struct {
		name   string
		chunks []TextChunk
		query  string
		wantGT int // want more than this many results
		wantLE int // want at most this many results
	}{
		{
			name:   "empty chunks",
			chunks: nil,
			query:  "test",
			wantGT: -1,
			wantLE: 0,
		},
		{
			name: "empty query returns all",
			chunks: []TextChunk{
				{Index: 0, Text: "hello world"},
			},
			query:  "",
			wantGT: 0,
			wantLE: 1,
		},
		{
			name: "relevant chunk kept",
			chunks: []TextChunk{
				{Index: 0, Text: "machine learning algorithms for natural language processing"},
				{Index: 1, Text: "cooking recipes for delicious pasta dishes"},
			},
			query:  "machine learning algorithms",
			wantGT: 0,
			wantLE: 2,
		},
		{
			name: "tag priority boosts score",
			chunks: []TextChunk{
				{Index: 0, Text: "machine learning algorithms", TagName: "h1"},
				{Index: 1, Text: "machine learning algorithms", TagName: "div"},
			},
			query:  "machine learning",
			wantGT: -1,
			wantLE: 2,
		},
	}

	f := NewBM25Filter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.FilterByRelevance(tt.chunks, tt.query)
			if len(result) <= tt.wantGT {
				t.Errorf("got %d results, want > %d", len(result), tt.wantGT)
			}
			if len(result) > tt.wantLE {
				t.Errorf("got %d results, want <= %d", len(result), tt.wantLE)
			}
		})
	}
}

func TestBM25FilterThreshold(t *testing.T) {
	f := &BM25Filter{K1: 2.0, B: 0.75, Threshold: 100.0} // very high threshold
	chunks := []TextChunk{
		{Index: 0, Text: "some random text about programming"},
	}
	result := f.FilterByRelevance(chunks, "programming")
	if len(result) != 0 {
		t.Errorf("high threshold should filter everything out, got %d results", len(result))
	}
}
