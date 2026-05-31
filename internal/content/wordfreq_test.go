package content

import (
	"testing"
)

func TestAnalyzeContent_BasicStats(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		topN           int
		wantWordCount  int
		wantCharCount  int
		wantSentences  int
		wantParagraphs int
	}{
		{
			name:           "empty string",
			text:           "",
			topN:           5,
			wantWordCount:  0,
			wantCharCount:  0,
			wantSentences:  0,
			wantParagraphs: 0,
		},
		{
			name:           "single sentence",
			text:           "The quick brown fox jumps over the lazy dog.",
			topN:           5,
			wantWordCount:  9,
			wantCharCount:  44,
			wantSentences:  1,
			wantParagraphs: 1,
		},
		{
			name:           "multiple sentences",
			text:           "Hello world. This is a test! Is it working?",
			topN:           5,
			wantWordCount:  9,
			wantCharCount:  43,
			wantSentences:  3,
			wantParagraphs: 1,
		},
		{
			name:           "multiple paragraphs",
			text:           "First paragraph here.\n\nSecond paragraph here.\n\nThird paragraph here.",
			topN:           5,
			wantWordCount:  9,
			wantCharCount:  68,
			wantSentences:  3,
			wantParagraphs: 3,
		},
		{
			name:           "topN defaults to 20 when zero",
			text:           "word word word",
			topN:           0,
			wantWordCount:  3,
			wantCharCount:  14,
			wantSentences:  0,
			wantParagraphs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := AnalyzeContent(tt.text, tt.topN)
			if stats.WordCount != tt.wantWordCount {
				t.Errorf("WordCount = %d, want %d", stats.WordCount, tt.wantWordCount)
			}
			if stats.CharCount != tt.wantCharCount {
				t.Errorf("CharCount = %d, want %d", stats.CharCount, tt.wantCharCount)
			}
			if stats.SentenceCount != tt.wantSentences {
				t.Errorf("SentenceCount = %d, want %d", stats.SentenceCount, tt.wantSentences)
			}
			if stats.ParagraphCount != tt.wantParagraphs {
				t.Errorf("ParagraphCount = %d, want %d", stats.ParagraphCount, tt.wantParagraphs)
			}
		})
	}
}

func TestAnalyzeContent_WordFrequencies(t *testing.T) {
	// "hello" appears 3 times, "world" 2 times. Both are >= 3 chars and not stop words.
	text := "hello world hello world hello"
	stats := AnalyzeContent(text, 10)

	if len(stats.TopWords) == 0 {
		t.Fatal("expected TopWords to be non-empty")
	}
	if stats.TopWords[0].Word != "hello" || stats.TopWords[0].Count != 3 {
		t.Errorf("TopWords[0] = %+v, want {hello, 3}", stats.TopWords[0])
	}
	if len(stats.TopWords) < 2 || stats.TopWords[1].Word != "world" || stats.TopWords[1].Count != 2 {
		t.Errorf("TopWords[1] = %+v, want {world, 2}", stats.TopWords[1])
	}
}

func TestAnalyzeContent_StopWordsFiltered(t *testing.T) {
	// "the" is a stop word and should not appear in TopWords.
	text := "the the the hello"
	stats := AnalyzeContent(text, 10)

	for _, wf := range stats.TopWords {
		if wf.Word == "the" {
			t.Error("stop word 'the' should not appear in TopWords")
		}
	}
}

func TestAnalyzeContent_ShortWordsFiltered(t *testing.T) {
	// Words shorter than 3 chars should not appear in TopWords.
	text := "go is ok but hello world rocks"
	stats := AnalyzeContent(text, 10)

	for _, wf := range stats.TopWords {
		if len(wf.Word) < 3 {
			t.Errorf("word %q (len %d) should be filtered from TopWords", wf.Word, len(wf.Word))
		}
	}
}

func TestAnalyzeContent_Bigrams(t *testing.T) {
	// Non-stop adjacent words >= 2 chars should form bigrams.
	text := "machine learning deep learning machine learning"
	stats := AnalyzeContent(text, 10)

	if len(stats.TopBigrams) == 0 {
		t.Fatal("expected TopBigrams to be non-empty")
	}

	found := false
	for _, bg := range stats.TopBigrams {
		if bg.Word == "machine learning" {
			found = true
			if bg.Count != 2 {
				t.Errorf("bigram 'machine learning' count = %d, want 2", bg.Count)
			}
		}
	}
	if !found {
		t.Error("expected to find bigram 'machine learning'")
	}
}

func TestAnalyzeContent_TopNLimit(t *testing.T) {
	// Create text with many distinct words, then request topN=2.
	text := "alpha alpha alpha bravo bravo bravo charlie charlie charlie delta delta delta echo echo echo"
	stats := AnalyzeContent(text, 2)

	if len(stats.TopWords) != 2 {
		t.Errorf("len(TopWords) = %d, want 2", len(stats.TopWords))
	}
}

func TestAnalyzeContent_UniqueWords(t *testing.T) {
	// UniqueWords counts distinct words >= 3 chars that are not stop words.
	text := "hello world hello universe"
	stats := AnalyzeContent(text, 10)

	if stats.UniqueWords != 3 {
		t.Errorf("UniqueWords = %d, want 3", stats.UniqueWords)
	}
}

func TestCountParagraphs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"single line", "hello", 1},
		{"blank lines between", "para1\n\npara2\n\npara3", 3},
		{"multiple consecutive blank lines", "para1\n\n\n\npara2", 2},
		{"trailing newline", "para1\n", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countParagraphs(tt.text)
			if got != tt.want {
				t.Errorf("countParagraphs() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTokenizeWords(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"empty", "", nil},
		{"simple", "Hello World", []string{"hello", "world"}},
		{"punctuation stripped", "Hello, world!", []string{"hello", "world"}},
		{"mixed punctuation", `"quoted" (parens) <angle>`, []string{"quoted", "parens", "angle"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeWords(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("tokenizeWords() len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenizeWords()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
