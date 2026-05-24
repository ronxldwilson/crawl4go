package content

import (
	"testing"
)

func TestScoreReadability(t *testing.T) {
	t.Run("empty text returns zero value", func(t *testing.T) {
		score := ScoreReadability("")
		if score.WordCount != 0 {
			t.Errorf("WordCount = %d, want 0", score.WordCount)
		}
		if score.SentenceCount != 0 {
			t.Errorf("SentenceCount = %d, want 0", score.SentenceCount)
		}
		if score.FleschReading != 0 {
			t.Errorf("FleschReading = %f, want 0", score.FleschReading)
		}
	})

	t.Run("word and sentence count for simple sentence", func(t *testing.T) {
		// "The cat sat." -> 3 words, 1 sentence
		score := ScoreReadability("The cat sat.")
		if score.WordCount != 3 {
			t.Errorf("WordCount = %d, want 3", score.WordCount)
		}
		if score.SentenceCount != 1 {
			t.Errorf("SentenceCount = %d, want 1", score.SentenceCount)
		}
	})

	t.Run("single sentence flesch reading score", func(t *testing.T) {
		// "The cat sat." -> 3 words, 1 sentence, 3 syllables
		// fleschReading = 206.835 - (1.015 * 3) - (84.6 * 1.0) = 119.19
		score := ScoreReadability("The cat sat.")
		if score.FleschReading != 119.19 {
			t.Errorf("FleschReading = %f, want 119.19", score.FleschReading)
		}
	})

	t.Run("easy text has higher flesch reading score than difficult text", func(t *testing.T) {
		easy := ScoreReadability("The cat sat. The dog ran. It is fun.")
		// A text with many polysyllabic words should score lower
		difficult := ScoreReadability("Photosynthesis is the biological process by which organisms synthesize carbohydrates.")
		if easy.FleschReading <= difficult.FleschReading {
			t.Errorf("easy FleschReading (%f) should be > difficult FleschReading (%f)",
				easy.FleschReading, difficult.FleschReading)
		}
	})

	t.Run("single sentence avg words per sentence", func(t *testing.T) {
		score := ScoreReadability("Hello world.")
		// 2 words, 1 sentence
		if score.AvgWordsPerSent != 2.0 {
			t.Errorf("AvgWordsPerSent = %f, want 2.0", score.AvgWordsPerSent)
		}
	})

	t.Run("rounding to two decimal places", func(t *testing.T) {
		score := ScoreReadability("The quick brown fox jumps over the lazy dog.")
		// Verify scores are rounded to two decimal places by checking they equal their rounded selves
		rounded := func(f float64) float64 {
			// multiply by 100, round, divide - use integer math
			v := int(f*100+0.5) // simple rounding check
			_ = v
			// Just check it has at most 2 decimal digits by checking score matches itself rounded
			return float64(int(f*100)) / 100
		}
		if score.FleschReading != rounded(score.FleschReading) && score.FleschReading != float64(int(score.FleschReading*100+0.5))/100 {
			t.Errorf("FleschReading %f is not rounded to 2 decimal places", score.FleschReading)
		}
	})
}

func TestCountSentences(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  int
	}{
		{"empty string", "", 0},
		{"single period", "Hello.", 1},
		{"question mark", "How are you?", 1},
		{"exclamation", "Wow!", 1},
		{"multiple sentences", "Hello. How are you? Fine!", 3},
		{"no punctuation", "hello world", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSentences(tt.text)
			if got != tt.want {
				t.Errorf("countSentences(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestCountSyllables(t *testing.T) {
	tests := []struct {
		word string
		want int
	}{
		{"running", 2},
		{"cake", 1},
		{"walked", 1},
		{"wanted", 2},
		{"a", 1},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := countSyllables(tt.word)
			if got != tt.want {
				t.Errorf("countSyllables(%q) = %d, want %d", tt.word, got, tt.want)
			}
		})
	}
}

func TestGradeToLevel(t *testing.T) {
	tests := []struct {
		name  string
		grade float64
		want  string
	}{
		{"below 1 is kindergarten", 0.5, "kindergarten"},
		{"exactly 0 is kindergarten", 0, "kindergarten"},
		{"negative is kindergarten", -5, "kindergarten"},
		{"grade 1 is elementary", 1.0, "elementary"},
		{"grade 5.9 is elementary", 5.9, "elementary"},
		{"grade 6 is middle school", 6.0, "middle_school"},
		{"grade 8.9 is middle school", 8.9, "middle_school"},
		{"grade 9 is high school", 9.0, "high_school"},
		{"grade 12.9 is high school", 12.9, "high_school"},
		{"grade 13 is college", 13.0, "college"},
		{"grade 16.9 is college", 16.9, "college"},
		{"grade 17 is graduate", 17.0, "graduate"},
		{"grade 20 is graduate", 20.0, "graduate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gradeToLevel(tt.grade)
			if got != tt.want {
				t.Errorf("gradeToLevel(%v) = %q, want %q", tt.grade, got, tt.want)
			}
		})
	}
}
