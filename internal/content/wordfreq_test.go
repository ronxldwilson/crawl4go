package content

import (
	"sync"
	"testing"
)

func TestAnalyzeContent_EmptyText(t *testing.T) {
	cs := AnalyzeContent("", 10)
	if cs.WordCount != 0 {
		t.Errorf("WordCount = %d, want 0", cs.WordCount)
	}
	if cs.CharCount != 0 {
		t.Errorf("CharCount = %d, want 0", cs.CharCount)
	}
	if cs.UniqueWords != 0 {
		t.Errorf("UniqueWords = %d, want 0", cs.UniqueWords)
	}
	if len(cs.TopWords) != 0 {
		t.Errorf("TopWords should be empty for empty text, got %v", cs.TopWords)
	}
	if len(cs.TopBigrams) != 0 {
		t.Errorf("TopBigrams should be empty for empty text, got %v", cs.TopBigrams)
	}
}

func TestAnalyzeContent_TopNZeroDefaultsTwenty(t *testing.T) {
	// Build a text with 25+ distinct long words.
	words := []string{
		"alpha", "bravo", "charlie", "delta", "echo",
		"foxtrot", "golf", "hotel", "india", "juliet",
		"kilo", "lima", "mike", "november", "oscar",
		"papa", "quebec", "romeo", "sierra", "tango",
		"uniform", "victor", "whiskey", "yankee", "zulu",
	}
	text := ""
	for _, w := range words {
		text += w + " "
	}
	// Repeat so every word appears at least once
	text += text

	cs0 := AnalyzeContent(text, 0)
	// topN defaults to 20
	if len(cs0.TopWords) > 20 {
		t.Errorf("TopWords len = %d, want at most 20 (default topN=20)", len(cs0.TopWords))
	}
}

func TestAnalyzeContent_StopWordsExcluded(t *testing.T) {
	// "the", "and", "is" are stop words; only "gopher" should appear in TopWords.
	cs := AnalyzeContent("the gopher and the gopher is great", 10)
	for _, wf := range cs.TopWords {
		switch wf.Word {
		case "the", "and", "is":
			t.Errorf("stop word %q found in TopWords", wf.Word)
		}
	}
	found := false
	for _, wf := range cs.TopWords {
		if wf.Word == "gopher" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'gopher' in TopWords")
	}
}

func TestAnalyzeContent_ShortWordsExcluded(t *testing.T) {
	// Words shorter than 3 chars should not appear in TopWords.
	cs := AnalyzeContent("go do up it in on at be to of or hi no", 10)
	for _, wf := range cs.TopWords {
		if len(wf.Word) < 3 {
			t.Errorf("short word %q (len=%d) found in TopWords", wf.Word, len(wf.Word))
		}
	}
}

func TestAnalyzeContent_PunctuationStripping(t *testing.T) {
	cs := AnalyzeContent("hello, world! hello. world? hello", 10)
	found := map[string]int{}
	for _, wf := range cs.TopWords {
		found[wf.Word] = wf.Count
	}
	if found["hello"] != 3 {
		t.Errorf("hello count = %d, want 3", found["hello"])
	}
	if found["world"] != 2 {
		t.Errorf("world count = %d, want 2", found["world"])
	}
}

func TestAnalyzeContent_ParagraphCounting(t *testing.T) {
	text := "First paragraph line one.\nFirst paragraph line two.\n\nSecond paragraph.\n\nThird paragraph."
	cs := AnalyzeContent(text, 10)
	if cs.ParagraphCount != 3 {
		t.Errorf("ParagraphCount = %d, want 3", cs.ParagraphCount)
	}
}

func TestCountParagraphs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single line", "hello world", 1},
		{"two paragraphs", "para one\n\npara two", 2},
		{"trailing newlines", "para one\n\npara two\n\n", 2},
		{"three paragraphs", "a\n\nb\n\nc", 3},
		{"only whitespace lines", "   \n\t\n  ", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countParagraphs(tt.input)
			if got != tt.want {
				t.Errorf("countParagraphs(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestTokenizeWords(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"Hello, World!", []string{"hello", "world"}},
		{"one two three", []string{"one", "two", "three"}},
		// tokenizeWords trims only leading/trailing punctuation; embedded
		// punctuation that isn't stripped leaves a single token.
		{"strip.punctuation!here", []string{"strip.punctuation!here"}},
		{"", []string{}},
	}
	for _, tt := range tests {
		got := tokenizeWords(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("tokenizeWords(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("tokenizeWords(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestTopEntries_SortedDescending(t *testing.T) {
	freq := map[string]int{
		"apple":  5,
		"banana": 3,
		"cherry": 8,
		"date":   1,
	}
	entries := topEntries(freq, 10)
	if len(entries) != 4 {
		t.Fatalf("len = %d, want 4", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Count > entries[i-1].Count {
			t.Errorf("entries not sorted descending at index %d: %d > %d", i, entries[i].Count, entries[i-1].Count)
		}
	}
	if entries[0].Word != "cherry" || entries[0].Count != 8 {
		t.Errorf("top entry = %+v, want cherry:8", entries[0])
	}
}

func TestTopEntries_LimitN(t *testing.T) {
	freq := map[string]int{
		"alpha": 10, "beta": 9, "gamma": 8, "delta": 7, "epsilon": 6,
	}
	entries := topEntries(freq, 3)
	if len(entries) != 3 {
		t.Errorf("len = %d, want 3", len(entries))
	}
}

func TestAnalyzeContent_UniqueWordsCount(t *testing.T) {
	// "gopher" x3, "language" x2, "great" x1 => 3 unique words (all >=3 chars, not stop words)
	cs := AnalyzeContent("gopher gopher gopher language language great", 10)
	if cs.UniqueWords != 3 {
		t.Errorf("UniqueWords = %d, want 3", cs.UniqueWords)
	}
}

func TestAnalyzeContent_CharCount(t *testing.T) {
	text := "hello"
	cs := AnalyzeContent(text, 5)
	if cs.CharCount != 5 {
		t.Errorf("CharCount = %d, want 5", cs.CharCount)
	}
}

func TestAnalyzeContent_SentenceCount(t *testing.T) {
	text := "Hello world. How are you? Fine!"
	cs := AnalyzeContent(text, 10)
	if cs.SentenceCount != 3 {
		t.Errorf("SentenceCount = %d, want 3", cs.SentenceCount)
	}
}

func TestAnalyzeContent_BigramComputation(t *testing.T) {
	// "gopher language" should appear twice
	cs := AnalyzeContent("gopher language gopher language", 10)
	found := false
	for _, bg := range cs.TopBigrams {
		if bg.Word == "gopher language" && bg.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bigram 'gopher language' with count 2 in %v", cs.TopBigrams)
	}
}

func TestAnalyzeContent_ConcurrentCallsIdentical(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog testing concurrent safety"
	const goroutines = 10
	results := make([]ContentStats, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = AnalyzeContent(text, 5)
		}(i)
	}
	wg.Wait()

	ref := results[0]
	for i := 1; i < goroutines; i++ {
		if results[i].WordCount != ref.WordCount {
			t.Errorf("goroutine %d: WordCount = %d, want %d", i, results[i].WordCount, ref.WordCount)
		}
		if results[i].UniqueWords != ref.UniqueWords {
			t.Errorf("goroutine %d: UniqueWords = %d, want %d", i, results[i].UniqueWords, ref.UniqueWords)
		}
		if results[i].CharCount != ref.CharCount {
			t.Errorf("goroutine %d: CharCount = %d, want %d", i, results[i].CharCount, ref.CharCount)
		}
	}
}
