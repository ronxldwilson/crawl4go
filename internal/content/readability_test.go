package content

import (
	"testing"
)

func TestScoreReadability(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		wantWordCount int
		wantSentCount int
		checkFunc     func(t *testing.T, s ReadabilityScore)
	}{
		{
			name:          "simple sentences",
			text:          "The cat sat on the mat. The dog ran fast.",
			wantWordCount: 10,
			wantSentCount: 2,
			checkFunc: func(t *testing.T, s ReadabilityScore) {
				if s.AvgWordsPerSent != 5.0 {
					t.Errorf("AvgWordsPerSent = %f, want 5.0", s.AvgWordsPerSent)
				}
				if s.ReadingLevel != "elementary" && s.ReadingLevel != "kindergarten" {
					t.Errorf("unexpected reading level %q for simple text", s.ReadingLevel)
				}
			},
		},
		{
			name:          "empty text",
			text:          "",
			wantWordCount: 0,
			wantSentCount: 0,
			checkFunc: func(t *testing.T, s ReadabilityScore) {
				if s.FleschKincaid != 0 {
					t.Errorf("expected zero FleschKincaid for empty, got %f", s.FleschKincaid)
				}
				if s.ReadingLevel != "" {
					t.Errorf("expected empty ReadingLevel, got %q", s.ReadingLevel)
				}
			},
		},
		{
			name:          "complex text",
			text:          "The implementation of sophisticated algorithms necessitates comprehensive understanding of computational complexity. Establishing appropriate methodologies requires considerable deliberation.",
			wantWordCount: 17,
			wantSentCount: 2,
			checkFunc: func(t *testing.T, s ReadabilityScore) {
				// Complex text should have higher grade level
				if s.FleschKincaid < 10 {
					t.Errorf("expected FleschKincaid >= 10 for complex text, got %f", s.FleschKincaid)
				}
				if s.GunningFog < 10 {
					t.Errorf("expected GunningFog >= 10 for complex text, got %f", s.GunningFog)
				}
			},
		},
		{
			name:          "no sentence terminators treated as one sentence",
			text:          "hello world this has no punctuation",
			wantWordCount: 6,
			wantSentCount: 1,
			checkFunc: func(t *testing.T, s ReadabilityScore) {
				if s.AvgWordsPerSent != 6.0 {
					t.Errorf("AvgWordsPerSent = %f, want 6.0", s.AvgWordsPerSent)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ScoreReadability(tt.text)
			if s.WordCount != tt.wantWordCount {
				t.Errorf("WordCount = %d, want %d", s.WordCount, tt.wantWordCount)
			}
			if s.SentenceCount != tt.wantSentCount {
				t.Errorf("SentenceCount = %d, want %d", s.SentenceCount, tt.wantSentCount)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, s)
			}
		})
	}
}

func TestCountSentences(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"Hello. World.", 2},
		{"What? Really! Yes.", 3},
		{"No punctuation here", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
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
		{"the", 1},
		{"it", 1},
		{"hello", 2},
		{"beautiful", 3},
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
		grade float64
		want  string
	}{
		{0.5, "kindergarten"},
		{3.0, "elementary"},
		{7.0, "middle_school"},
		{10.0, "high_school"},
		{14.0, "college"},
		{18.0, "graduate"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := gradeToLevel(tt.grade)
			if got != tt.want {
				t.Errorf("gradeToLevel(%f) = %q, want %q", tt.grade, got, tt.want)
			}
		})
	}
}

func TestReadabilityScoreFields(t *testing.T) {
	text := "This is a simple test. It has two sentences."
	s := ScoreReadability(text)

	if s.SyllableCount <= 0 {
		t.Error("expected positive SyllableCount")
	}
	if s.AvgSyllPerWord <= 0 {
		t.Error("expected positive AvgSyllPerWord")
	}
	if s.FleschReading == 0 && s.WordCount > 0 {
		t.Error("expected non-zero FleschReading for non-empty text")
	}
}
