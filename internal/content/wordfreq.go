package content

import (
	"sort"
	"strings"
)

type WordFrequency struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type ContentStats struct {
	WordCount     int             `json:"word_count"`
	UniqueWords   int             `json:"unique_words"`
	CharCount     int             `json:"char_count"`
	SentenceCount int             `json:"sentence_count"`
	ParagraphCount int            `json:"paragraph_count"`
	TopWords      []WordFrequency `json:"top_words"`
	TopBigrams    []WordFrequency `json:"top_bigrams"`
}

func AnalyzeContent(text string, topN int) ContentStats {
	if topN <= 0 {
		topN = 20
	}

	words := tokenizeWords(text)
	sentences := countSentences(text)
	paragraphs := countParagraphs(text)

	freq := make(map[string]int)
	for _, w := range words {
		if len(w) >= 3 && !stopWords[w] {
			freq[w]++
		}
	}

	bigramFreq := make(map[string]int)
	for i := 0; i < len(words)-1; i++ {
		if len(words[i]) >= 2 && len(words[i+1]) >= 2 && !stopWords[words[i]] && !stopWords[words[i+1]] {
			bigram := words[i] + " " + words[i+1]
			bigramFreq[bigram]++
		}
	}

	topWords := topEntries(freq, topN)
	topBigrams := topEntries(bigramFreq, topN)

	return ContentStats{
		WordCount:      len(words),
		UniqueWords:    len(freq),
		CharCount:      len(text),
		SentenceCount:  sentences,
		ParagraphCount: paragraphs,
		TopWords:       topWords,
		TopBigrams:     topBigrams,
	}
}

func tokenizeWords(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	result := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}<>-_/\\@#$%^&*+=~`")
		if w != "" {
			result = append(result, w)
		}
	}
	return result
}

func countParagraphs(text string) int {
	count := 0
	lines := strings.Split(text, "\n")
	inPara := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			if !inPara {
				count++
				inPara = true
			}
		} else {
			inPara = false
		}
	}
	return count
}

func topEntries(freq map[string]int, n int) []WordFrequency {
	entries := make([]WordFrequency, 0, len(freq))
	for word, count := range freq {
		entries = append(entries, WordFrequency{Word: word, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Count > entries[j].Count
	})
	if len(entries) > n {
		entries = entries[:n]
	}
	return entries
}
