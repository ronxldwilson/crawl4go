package content

import (
	"strings"
	"unicode/utf8"
)

// Chunk represents a piece of text with its position and optional metadata.
type Chunk struct {
	Text     string
	Index    int
	Metadata map[string]string
}

// ChunkStrategy is the interface implemented by all chunking strategies.
type ChunkStrategy interface {
	Chunk(text string) []Chunk
}

// ─── FixedSizeChunker ────────────────────────────────────────────────────────

// FixedSizeChunker splits text into fixed-size chunks with optional overlap.
// It tries to break at sentence, paragraph, or word boundaries.
type FixedSizeChunker struct {
	MaxChars int
	Overlap  int
}

// NewFixedSizeChunker returns a FixedSizeChunker with the given parameters.
func NewFixedSizeChunker(maxChars, overlap int) *FixedSizeChunker {
	return &FixedSizeChunker{MaxChars: maxChars, Overlap: overlap}
}

// Chunk implements ChunkStrategy for FixedSizeChunker.
func (c *FixedSizeChunker) Chunk(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}

	var chunks []Chunk
	runes := []rune(text)
	total := len(runes)
	start := 0
	index := 0

	for start < total {
		end := start + c.MaxChars
		if end >= total {
			// Last chunk – take everything remaining.
			chunk := string(runes[start:total])
			chunks = append(chunks, Chunk{
				Text:     chunk,
				Index:    index,
				Metadata: map[string]string{},
			})
			break
		}

		// Try to find the best break point within the window.
		breakAt := findBreakPoint(runes, start, end)

		chunk := strings.TrimSpace(string(runes[start:breakAt]))
		if chunk != "" {
			chunks = append(chunks, Chunk{
				Text:     chunk,
				Index:    index,
				Metadata: map[string]string{},
			})
			index++
		}

		// Advance start, accounting for overlap.
		next := breakAt
		if c.Overlap > 0 && next > start+c.Overlap {
			next = next - c.Overlap
		}
		// Safety: always advance at least one rune to prevent infinite loop.
		if next <= start {
			next = start + 1
		}
		start = next
	}

	return chunks
}

// findBreakPoint searches backwards from end for sentence → paragraph → word
// boundaries within runes[start:end].
func findBreakPoint(runes []rune, start, end int) int {
	// Try paragraph boundary first (\n\n).
	for i := end; i > start; i-- {
		if i+1 < len(runes) && runes[i] == '\n' && runes[i-1] == '\n' {
			return i + 1
		}
	}
	// Try sentence boundary (., !, ?).
	for i := end; i > start; i-- {
		r := runes[i]
		if r == '.' || r == '!' || r == '?' {
			return i + 1
		}
	}
	// Try word boundary (space).
	for i := end; i > start; i-- {
		if runes[i] == ' ' {
			return i + 1
		}
	}
	// Hard cut.
	return end
}

// ─── SlidingWindowChunker ────────────────────────────────────────────────────

// SlidingWindowChunker produces overlapping chunks of WindowSize characters
// advancing StepSize characters each iteration.
type SlidingWindowChunker struct {
	WindowSize int
	StepSize   int
}

// NewSlidingWindowChunker returns a SlidingWindowChunker with the given sizes.
func NewSlidingWindowChunker(windowSize, stepSize int) *SlidingWindowChunker {
	return &SlidingWindowChunker{WindowSize: windowSize, StepSize: stepSize}
}

// Chunk implements ChunkStrategy for SlidingWindowChunker.
func (c *SlidingWindowChunker) Chunk(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}
	if c.StepSize <= 0 {
		c.StepSize = 1
	}

	runes := []rune(text)
	total := len(runes)
	var chunks []Chunk
	index := 0

	for start := 0; start < total; start += c.StepSize {
		end := start + c.WindowSize
		if end > total {
			end = total
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, Chunk{
				Text:     chunk,
				Index:    index,
				Metadata: map[string]string{},
			})
			index++
		}
		if end == total {
			break
		}
	}

	return chunks
}

// ─── SemanticChunker ─────────────────────────────────────────────────────────

// SemanticChunker splits on paragraph and heading boundaries, merging small
// paragraphs together up to MaxChars. Heading context is preserved in metadata.
type SemanticChunker struct {
	MaxChars int
}

// NewSemanticChunker returns a SemanticChunker with the given size limit.
func NewSemanticChunker(maxChars int) *SemanticChunker {
	return &SemanticChunker{MaxChars: maxChars}
}

// Chunk implements ChunkStrategy for SemanticChunker.
func (c *SemanticChunker) Chunk(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}

	// Split on double newlines (paragraph boundaries).
	paragraphs := splitParagraphs(text)

	type block struct {
		text    string
		heading string
	}

	// Tag each paragraph with its detected heading.
	var blocks []block
	currentHeading := ""
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if h := detectHeading(p); h != "" {
			currentHeading = h
		}
		blocks = append(blocks, block{text: p, heading: currentHeading})
	}

	// Merge blocks up to MaxChars.
	var chunks []Chunk
	index := 0
	acc := ""
	accHeading := ""

	flush := func() {
		t := strings.TrimSpace(acc)
		if t == "" {
			return
		}
		chunks = append(chunks, Chunk{
			Text:  t,
			Index: index,
			Metadata: map[string]string{
				"heading": accHeading,
			},
		})
		index++
		acc = ""
	}

	for _, b := range blocks {
		candidate := acc
		if candidate != "" {
			candidate += "\n\n"
		}
		candidate += b.text

		if utf8.RuneCountInString(candidate) > c.MaxChars && acc != "" {
			flush()
			acc = b.text
			accHeading = b.heading
		} else {
			acc = candidate
			if b.heading != "" {
				accHeading = b.heading
			}
		}
	}
	flush()

	return chunks
}

// splitParagraphs splits text on double newline sequences.
func splitParagraphs(text string) []string {
	return strings.Split(text, "\n\n")
}

// detectHeading returns the heading text if the paragraph looks like a heading,
// otherwise returns an empty string.
// Detects markdown headings (lines starting with #) and short lines followed
// by a longer continuation line.
func detectHeading(para string) string {
	lines := strings.SplitN(para, "\n", 3)
	first := strings.TrimSpace(lines[0])

	// Markdown heading.
	if strings.HasPrefix(first, "#") {
		return strings.TrimSpace(strings.TrimLeft(first, "#"))
	}

	// Short line (≤ 80 chars) followed by a longer line – treat as heading.
	if len(lines) >= 2 {
		second := strings.TrimSpace(lines[1])
		if utf8.RuneCountInString(first) <= 80 && utf8.RuneCountInString(second) > utf8.RuneCountInString(first) {
			return first
		}
	}

	return ""
}

// ─── MarkdownChunker ─────────────────────────────────────────────────────────

// MarkdownChunker understands markdown structure. It splits at header
// boundaries and keeps code blocks intact. The last heading seen is preserved
// in Metadata["heading"].
type MarkdownChunker struct {
	MaxChars int
}

// NewMarkdownChunker returns a MarkdownChunker with the given size limit.
func NewMarkdownChunker(maxChars int) *MarkdownChunker {
	return &MarkdownChunker{MaxChars: maxChars}
}

// Chunk implements ChunkStrategy for MarkdownChunker.
func (c *MarkdownChunker) Chunk(text string) []Chunk {
	if len(text) == 0 {
		return nil
	}

	sections := splitMarkdownSections(text)

	var chunks []Chunk
	index := 0
	acc := ""
	accHeading := ""

	flush := func() {
		t := strings.TrimSpace(acc)
		if t == "" {
			return
		}
		// If acc exceeds MaxChars, hard-split it.
		if utf8.RuneCountInString(t) > c.MaxChars {
			sub := hardSplit(t, c.MaxChars)
			for _, s := range sub {
				chunks = append(chunks, Chunk{
					Text:  s,
					Index: index,
					Metadata: map[string]string{
						"heading": accHeading,
					},
				})
				index++
			}
		} else {
			chunks = append(chunks, Chunk{
				Text:  t,
				Index: index,
				Metadata: map[string]string{
					"heading": accHeading,
				},
			})
			index++
		}
		acc = ""
	}

	for _, sec := range sections {
		sec.content = strings.TrimSpace(sec.content)
		if sec.content == "" {
			if sec.heading != "" {
				accHeading = sec.heading
			}
			continue
		}

		candidate := acc
		if candidate != "" {
			candidate += "\n\n"
		}
		candidate += sec.content

		candidateLen := utf8.RuneCountInString(candidate)

		if candidateLen > c.MaxChars && acc != "" {
			flush()
			acc = sec.content
			if sec.heading != "" {
				accHeading = sec.heading
			}
		} else {
			acc = candidate
			if sec.heading != "" {
				accHeading = sec.heading
			}
		}
	}
	flush()

	return chunks
}

type mdSection struct {
	heading string
	content string
}

// splitMarkdownSections splits markdown text at header lines (# ## ### etc.)
// and preserves code blocks intact.
func splitMarkdownSections(text string) []mdSection {
	lines := strings.Split(text, "\n")
	var sections []mdSection
	var currentHeading string
	var buf strings.Builder
	inCodeBlock := false

	save := func() {
		c := strings.TrimSpace(buf.String())
		if c != "" || currentHeading != "" {
			sections = append(sections, mdSection{
				heading: currentHeading,
				content: c,
			})
		}
		buf.Reset()
	}

	for _, line := range lines {
		// Track code block boundaries.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}

		if !inCodeBlock && isMarkdownHeader(line) {
			save()
			currentHeading = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
			// Include the header line itself in the new section's content.
			buf.WriteString(line)
			buf.WriteByte('\n')
			continue
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	save()

	return sections
}

// isMarkdownHeader returns true if the line is a markdown ATX header (# to ######).
func isMarkdownHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	// Must be followed by a space or be only hashes.
	rest := strings.TrimLeft(trimmed, "#")
	return rest == "" || strings.HasPrefix(rest, " ")
}

// hardSplit splits text into chunks of at most maxChars runes, breaking at
// word boundaries where possible.
func hardSplit(text string, maxChars int) []string {
	runes := []rune(text)
	total := len(runes)
	var result []string
	start := 0

	for start < total {
		end := start + maxChars
		if end >= total {
			result = append(result, strings.TrimSpace(string(runes[start:total])))
			break
		}
		bp := findBreakPoint(runes, start, end)
		result = append(result, strings.TrimSpace(string(runes[start:bp])))
		if bp <= start {
			bp = start + 1
		}
		start = bp
	}

	return result
}
