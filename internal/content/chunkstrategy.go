package content

import (
	"regexp"
	"strings"
)

// ChunkingStrategy is the interface for simple text chunking that returns
// plain string slices (as opposed to ChunkStrategy which returns []Chunk).
type ChunkingStrategy interface {
	Chunk(text string) []string
}

// ─── RegexChunking ──────────────────────────────────────────────────────────

// RegexChunking splits text by a configurable regex pattern.
type RegexChunking struct {
	Patterns []string // regex patterns to split on; first valid pattern is used
}

// NewRegexChunking returns a RegexChunking with the given patterns.
// If no patterns are provided, it defaults to splitting on double newlines.
func NewRegexChunking(patterns ...string) *RegexChunking {
	if len(patterns) == 0 {
		patterns = []string{`\n\n`}
	}
	return &RegexChunking{Patterns: patterns}
}

// Chunk implements ChunkingStrategy.
func (r *RegexChunking) Chunk(text string) []string {
	if text == "" {
		return nil
	}

	for _, pat := range r.Patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		parts := re.Split(text, -1)
		var result []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	// Fallback: return entire text as single chunk.
	return []string{text}
}

// ─── OverlappingWindowChunking ──────────────────────────────────────────────

// OverlappingWindowChunking splits text into fixed-size windows (measured in
// characters) that overlap by a configurable amount.
type OverlappingWindowChunking struct {
	WindowSize int
	Overlap    int
}

// NewOverlappingWindowChunking returns an OverlappingWindowChunking with the
// given window size and overlap.
func NewOverlappingWindowChunking(windowSize, overlap int) *OverlappingWindowChunking {
	return &OverlappingWindowChunking{WindowSize: windowSize, Overlap: overlap}
}

// Chunk implements ChunkingStrategy.
func (o *OverlappingWindowChunking) Chunk(text string) []string {
	if text == "" {
		return nil
	}
	if o.WindowSize <= 0 {
		o.WindowSize = 500
	}
	if o.Overlap < 0 {
		o.Overlap = 0
	}
	if o.Overlap >= o.WindowSize {
		o.Overlap = o.WindowSize - 1
	}

	runes := []rune(text)
	total := len(runes)
	step := o.WindowSize - o.Overlap
	if step <= 0 {
		step = 1
	}

	var chunks []string
	for start := 0; start < total; start += step {
		end := start + o.WindowSize
		if end > total {
			end = total
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == total {
			break
		}
	}

	return chunks
}

// ─── NlpSentenceChunking ────────────────────────────────────────────────────

// sentenceBoundaryRe matches sentence-ending punctuation followed by a space
// or newline — a simple heuristic for sentence boundaries.
var sentenceBoundaryRe = regexp.MustCompile(`(?:[.!?])\s+|\n`)

// NlpSentenceChunking splits text on sentence boundaries using a simple
// regex approach (splits on ". ", "! ", "? ", and newlines).
type NlpSentenceChunking struct{}

// NewNlpSentenceChunking returns an NlpSentenceChunking instance.
func NewNlpSentenceChunking() *NlpSentenceChunking {
	return &NlpSentenceChunking{}
}

// Chunk implements ChunkingStrategy.
func (n *NlpSentenceChunking) Chunk(text string) []string {
	if text == "" {
		return nil
	}

	// Split at boundary locations, keeping the terminator with the preceding
	// sentence (by finding match indices and slicing manually).
	indices := sentenceBoundaryRe.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		t := strings.TrimSpace(text)
		if t == "" {
			return nil
		}
		return []string{t}
	}

	var sentences []string
	prev := 0
	for _, idx := range indices {
		// Include the punctuation character (but not the trailing whitespace)
		// in the sentence.
		end := idx[0] + 1 // include . or ! or ?
		if end > len(text) {
			end = len(text)
		}
		// For newline splits, the boundary char itself is \n, keep up to idx[0].
		if text[idx[0]] == '\n' {
			end = idx[0]
		}
		s := strings.TrimSpace(text[prev:end])
		if s != "" {
			sentences = append(sentences, s)
		}
		prev = idx[1]
	}
	// Remaining text after last boundary.
	if prev < len(text) {
		s := strings.TrimSpace(text[prev:])
		if s != "" {
			sentences = append(sentences, s)
		}
	}

	return sentences
}

// ─── TopicSegmentationChunking ──────────────────────────────────────────────

// TopicSegmentationChunking splits text on topic boundaries by grouping
// adjacent paragraphs that share vocabulary. Paragraphs are split on
// double-newlines. Adjacent paragraphs are merged when their word overlap
// ratio exceeds Threshold; a new topic segment starts when it drops below.
type TopicSegmentationChunking struct {
	Threshold float64 // word overlap ratio threshold (0.0–1.0); default 0.1
}

// NewTopicSegmentationChunking returns a TopicSegmentationChunking with the
// given overlap threshold.
func NewTopicSegmentationChunking(threshold float64) *TopicSegmentationChunking {
	if threshold <= 0 {
		threshold = 0.1
	}
	return &TopicSegmentationChunking{Threshold: threshold}
}

// Chunk implements ChunkingStrategy.
func (t *TopicSegmentationChunking) Chunk(text string) []string {
	if text == "" {
		return nil
	}

	paragraphs := strings.Split(text, "\n\n")
	var cleaned []string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	if len(cleaned) == 1 {
		return cleaned
	}

	// Build word sets for each paragraph.
	wordSets := make([]map[string]struct{}, len(cleaned))
	for i, p := range cleaned {
		wordSets[i] = wordSet(p)
	}

	// Group adjacent paragraphs by overlap ratio.
	var groups []string
	current := cleaned[0]

	for i := 1; i < len(cleaned); i++ {
		ratio := overlapRatio(wordSets[i-1], wordSets[i])
		if ratio >= t.Threshold {
			current += "\n\n" + cleaned[i]
		} else {
			groups = append(groups, current)
			current = cleaned[i]
		}
	}
	groups = append(groups, current)

	return groups
}

// wordSet returns the set of lowercased words in text.
func wordSet(text string) map[string]struct{} {
	words := strings.Fields(strings.ToLower(text))
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		set[w] = struct{}{}
	}
	return set
}

// overlapRatio computes the Jaccard-like overlap ratio between two word sets:
// |intersection| / |smaller set|. Returns 0 if either set is empty.
func overlapRatio(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	smaller, larger := a, b
	if len(a) > len(b) {
		smaller, larger = b, a
	}
	overlap := 0
	for w := range smaller {
		if _, ok := larger[w]; ok {
			overlap++
		}
	}
	denom := len(smaller)
	if denom == 0 {
		return 0
	}
	return float64(overlap) / float64(denom)
}
