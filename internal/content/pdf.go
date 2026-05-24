package content

import (
	"bytes"
	"compress/flate"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// PDFProcessorConfig holds configuration for PDF text extraction.
type PDFProcessorConfig struct {
	ExtractImages bool
	MaxPages      int
	OCREnabled    bool
}

// PDFProcessor implements ExtractionStrategy for PDF files.
// It uses a lightweight approach: reads the PDF binary, extracts text between
// stream/endstream markers using FlateDecode decompression where possible,
// and falls back to extracting readable ASCII text runs.
type PDFProcessor struct {
	Config PDFProcessorConfig
}

// NewPDFProcessor creates a PDFProcessor with the given config.
func NewPDFProcessor(cfg PDFProcessorConfig) *PDFProcessor {
	return &PDFProcessor{Config: cfg}
}

func (p *PDFProcessor) Name() string { return "pdf" }

// Extract reads a PDF file from the given path and extracts text content.
// The input parameter is a file path.
func (p *PDFProcessor) Extract(_ context.Context, input string, _ ExtractionConfig) ([]ExtractionResult, error) {
	text, err := p.ExtractFromPath(input)
	if err != nil {
		return nil, err
	}
	return []ExtractionResult{{Content: text, Index: 0}}, nil
}

// ExtractFromPath reads a PDF file and returns extracted text.
func (p *PDFProcessor) ExtractFromPath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("pdf: read file: %w", err)
	}

	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		return "", fmt.Errorf("pdf: not a valid PDF file")
	}

	var textParts []string

	// Extract text from streams (FlateDecode decompression).
	streamTexts := extractStreams(data, p.Config.MaxPages)
	textParts = append(textParts, streamTexts...)

	// If streams yielded little text, fall back to ASCII text runs.
	combined := strings.Join(textParts, "\n")
	if len(strings.TrimSpace(combined)) < 50 {
		fallback := extractTextRuns(data)
		if len(fallback) > len(combined) {
			combined = fallback
		}
	}

	// Clean up the result.
	combined = cleanPDFText(combined)
	return combined, nil
}

var (
	streamStartRe = regexp.MustCompile(`stream\r?\n`)
	endstreamRe   = regexp.MustCompile(`\r?\nendstream`)
	flatDecodeRe  = regexp.MustCompile(`/FlateDecode`)
)

// extractStreams finds stream/endstream pairs in the PDF data and attempts
// to decompress FlateDecode streams, extracting text content.
func extractStreams(data []byte, maxPages int) []string {
	var texts []string
	pageCount := 0

	startMatches := streamStartRe.FindAllIndex(data, -1)
	for _, startIdx := range startMatches {
		if maxPages > 0 && pageCount >= maxPages {
			break
		}

		streamBegin := startIdx[1]
		remaining := data[streamBegin:]
		endIdx := endstreamRe.FindIndex(remaining)
		if endIdx == nil {
			continue
		}

		streamData := remaining[:endIdx[0]]

		// Try FlateDecode decompression.
		// Check if this stream's object dictionary mentions FlateDecode.
		prelude := data[max(0, startIdx[0]-512):startIdx[0]]
		if flatDecodeRe.Match(prelude) {
			decoded, err := flateDecompress(streamData)
			if err == nil {
				text := extractTextFromPDFStream(decoded)
				if text != "" {
					texts = append(texts, text)
					pageCount++
					continue
				}
			}
		}

		// Try raw text extraction from uncompressed streams.
		text := extractTextFromPDFStream(streamData)
		if text != "" {
			texts = append(texts, text)
			pageCount++
		}
	}

	return texts
}

// flateDecompress decompresses zlib/flate-compressed data.
func flateDecompress(data []byte) ([]byte, error) {
	reader := flate.NewReader(bytes.NewReader(data))
	defer reader.Close()

	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var (
	// Matches PDF text operators: Tj, TJ, ' and "
	pdfTextOpRe = regexp.MustCompile(`\(([^)]*)\)\s*Tj|\[([^\]]*)\]\s*TJ`)
	// Matches parenthesized strings inside TJ arrays
	pdfTJStringRe = regexp.MustCompile(`\(([^)]*)\)`)
)

// extractTextFromPDFStream extracts readable text from a decoded PDF content
// stream by finding text-showing operators (Tj, TJ).
func extractTextFromPDFStream(data []byte) string {
	s := string(data)
	var parts []string

	matches := pdfTextOpRe.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		if m[1] != "" {
			// Tj operator: single string.
			parts = append(parts, decodePDFString(m[1]))
		} else if m[2] != "" {
			// TJ operator: array of strings and positioning values.
			subMatches := pdfTJStringRe.FindAllStringSubmatch(m[2], -1)
			for _, sm := range subMatches {
				parts = append(parts, decodePDFString(sm[1]))
			}
		}
	}

	return strings.Join(parts, "")
}

// decodePDFString handles basic PDF string escape sequences.
func decodePDFString(s string) string {
	replacer := strings.NewReplacer(
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
		`\\`, `\`,
		`\(`, "(",
		`\)`, ")",
	)
	return replacer.Replace(s)
}

// extractTextRuns finds sequences of printable ASCII characters in raw data.
func extractTextRuns(data []byte) string {
	var runs []string
	var current []byte

	for _, b := range data {
		if b >= 32 && b <= 126 {
			current = append(current, b)
		} else if b == '\n' || b == '\r' || b == '\t' {
			current = append(current, ' ')
		} else {
			if len(current) >= 4 {
				runs = append(runs, string(current))
			}
			current = current[:0]
		}
	}
	if len(current) >= 4 {
		runs = append(runs, string(current))
	}

	return strings.Join(runs, "\n")
}

// cleanPDFText normalizes whitespace and removes non-meaningful lines.
func cleanPDFText(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip lines that are purely PDF operators/metadata.
		if isPDFOperatorLine(line) {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

var pdfOpLineRe = regexp.MustCompile(`^[0-9.\s]*[A-Z][a-z]?\s*$|^<<.*>>$|^/[A-Z]`)

// isPDFOperatorLine detects lines that are PDF internal syntax rather than content.
func isPDFOperatorLine(line string) bool {
	if len(line) > 200 {
		return false
	}
	return pdfOpLineRe.MatchString(line)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
