package content

import (
	"math"
	"strings"
	"unicode"
)

type ReadabilityScore struct {
	FleschKincaid   float64 `json:"flesch_kincaid"`
	FleschReading   float64 `json:"flesch_reading_ease"`
	GunningFog      float64 `json:"gunning_fog"`
	WordCount       int     `json:"word_count"`
	SentenceCount   int     `json:"sentence_count"`
	SyllableCount   int     `json:"syllable_count"`
	AvgWordsPerSent float64 `json:"avg_words_per_sentence"`
	AvgSyllPerWord  float64 `json:"avg_syllables_per_word"`
	ReadingLevel    string  `json:"reading_level"`
}

func ScoreReadability(text string) ReadabilityScore {
	words := strings.Fields(text)
	wordCount := len(words)
	if wordCount == 0 {
		return ReadabilityScore{}
	}

	sentenceCount := countSentences(text)
	if sentenceCount == 0 {
		sentenceCount = 1
	}

	syllableCount := 0
	complexWords := 0
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		s := countSyllables(w)
		syllableCount += s
		if s >= 3 {
			complexWords++
		}
	}

	avgWPS := float64(wordCount) / float64(sentenceCount)
	avgSPW := float64(syllableCount) / float64(wordCount)

	fleschReading := 206.835 - (1.015 * avgWPS) - (84.6 * avgSPW)
	fleschKincaid := (0.39 * avgWPS) + (11.8 * avgSPW) - 15.59
	gunningFog := 0.4 * (avgWPS + 100*(float64(complexWords)/float64(wordCount)))

	fleschReading = math.Round(fleschReading*100) / 100
	fleschKincaid = math.Round(fleschKincaid*100) / 100
	gunningFog = math.Round(gunningFog*100) / 100

	level := gradeToLevel(fleschKincaid)

	return ReadabilityScore{
		FleschKincaid:   fleschKincaid,
		FleschReading:   fleschReading,
		GunningFog:      gunningFog,
		WordCount:       wordCount,
		SentenceCount:   sentenceCount,
		SyllableCount:   syllableCount,
		AvgWordsPerSent: math.Round(avgWPS*100) / 100,
		AvgSyllPerWord:  math.Round(avgSPW*100) / 100,
		ReadingLevel:    level,
	}
}

func countSentences(text string) int {
	count := 0
	for _, r := range text {
		if r == '.' || r == '!' || r == '?' {
			count++
		}
	}
	return count
}

func countSyllables(word string) int {
	word = strings.ToLower(word)
	if len(word) <= 2 {
		return 1
	}

	vowels := "aeiouy"
	count := 0
	prevVowel := false

	for i, r := range word {
		isVowel := strings.ContainsRune(vowels, r)
		if isVowel && !prevVowel {
			count++
		}
		prevVowel = isVowel
		_ = i
	}

	if strings.HasSuffix(word, "e") && !strings.HasSuffix(word, "le") {
		count--
	}
	if strings.HasSuffix(word, "ed") && len(word) > 3 {
		prev := rune(word[len(word)-3])
		if !unicode.IsLetter(prev) || (prev != 't' && prev != 'd') {
			count--
		}
	}

	if count <= 0 {
		count = 1
	}
	return count
}

func gradeToLevel(grade float64) string {
	switch {
	case grade < 1:
		return "kindergarten"
	case grade < 6:
		return "elementary"
	case grade < 9:
		return "middle_school"
	case grade < 13:
		return "high_school"
	case grade < 17:
		return "college"
	default:
		return "graduate"
	}
}
