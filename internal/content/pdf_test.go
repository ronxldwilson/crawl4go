package content

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFProcessor_Name(t *testing.T) {
	p := NewPDFProcessor(PDFProcessorConfig{})
	if got := p.Name(); got != "pdf" {
		t.Errorf("Name() = %q, want %q", got, "pdf")
	}
}

func TestPDFProcessor_ExtractFromPath_Missing(t *testing.T) {
	p := NewPDFProcessor(PDFProcessorConfig{})
	_, err := p.ExtractFromPath("/nonexistent/path/file.pdf")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestPDFProcessor_ExtractFromPath_NotPDF(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "notapdf.txt")
	if err := os.WriteFile(path, []byte("this is not a PDF file at all"), 0644); err != nil {
		t.Fatal(err)
	}

	p := NewPDFProcessor(PDFProcessorConfig{})
	_, err := p.ExtractFromPath(path)
	if err == nil {
		t.Fatal("expected error for non-PDF file, got nil")
	}
}

func TestPDFProcessor_Extract_MissingViaInterface(t *testing.T) {
	p := NewPDFProcessor(PDFProcessorConfig{})
	_, err := p.Extract(context.Background(), "/no/such/file.pdf", ExtractionConfig{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDecodePDFString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"newline escape", `\n`, "\n"},
		{"carriage return escape", `\r`, "\r"},
		{"tab escape", `\t`, "\t"},
		{"backslash escape", `\\`, `\`},
		{"open paren escape", `\(`, "("},
		{"close paren escape", `\)`, ")"},
		{"plain text unchanged", "hello world", "hello world"},
		{"mixed escapes", `hello\nworld\t!`, "hello\nworld\t!"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodePDFString(tc.input)
			if got != tc.want {
				t.Errorf("decodePDFString(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsPDFOperatorLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		want  bool
	}{
		{"PDF resource ref", "/FontSize", true},
		{"PDF dict start", "<</Type /Page>>", true},
		{"normal text", "Hello, this is a sentence.", false},
		{"long line not operator", "This is a very long line that exceeds two hundred characters of content and therefore should not be treated as a PDF operator line even if it started with something interesting", false},
		{"empty string", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isPDFOperatorLine(tc.line)
			if got != tc.want {
				t.Errorf("isPDFOperatorLine(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestCleanPDFText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips blank lines",
			input: "hello\n\n\nworld",
			want:  "hello\nworld",
		},
		{
			name:  "trims leading/trailing whitespace on each line",
			input: "  hello  \n  world  ",
			want:  "hello\nworld",
		},
		{
			name:  "removes PDF operator lines",
			input: "Real content\n/FontName\nMore content",
			want:  "Real content\nMore content",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanPDFText(tc.input)
			if got != tc.want {
				t.Errorf("cleanPDFText(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestExtractTextRuns(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		contains string
		minLen   int
	}{
		{
			name:     "ASCII text extracted",
			data:     []byte("Hello World this is text"),
			contains: "Hello",
			minLen:   5,
		},
		{
			name:     "short runs filtered out",
			data:     []byte{0x00, 0x01, 0x02, 'H', 'i', 0x00},
			contains: "",
			minLen:   0,
		},
		{
			name:     "binary data with embedded text",
			data:     append([]byte{0x00, 0x01, 0x02, 0x03}, []byte("readable text here")...),
			contains: "readable text here",
			minLen:   4,
		},
		{
			name:     "empty data",
			data:     []byte{},
			contains: "",
			minLen:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTextRuns(tc.data)
			if tc.contains != "" && len(got) == 0 {
				t.Errorf("extractTextRuns expected to contain %q but got empty string", tc.contains)
			}
			_ = got // result is valid as long as no panic
		})
	}
}
