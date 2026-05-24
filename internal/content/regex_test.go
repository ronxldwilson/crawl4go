package content

import (
	"testing"
)

func TestRegexExtractor_Extract(t *testing.T) {
	tests := []struct {
		name      string
		schema    RegexExtractionSchema
		text      string
		wantLen   int
		wantErr   bool
		checkFunc func(t *testing.T, results []map[string]any)
	}{
		{
			name: "simple full match",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "email", Pattern: `[\w.+-]+@[\w-]+\.[\w.]+`, Group: 0},
				},
			},
			text:    "Contact us at alice@example.com or bob@test.org for info.",
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				if results[0]["email"] != "alice@example.com" {
					t.Errorf("first email = %v", results[0]["email"])
				}
				if results[1]["email"] != "bob@test.org" {
					t.Errorf("second email = %v", results[1]["email"])
				}
			},
		},
		{
			name: "capture group extraction",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "price", Pattern: `\$(\d+\.\d{2})`, Group: 1},
				},
			},
			text:    "Item costs $19.99 and $5.50.",
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				if results[0]["price"] != "19.99" {
					t.Errorf("first price = %v", results[0]["price"])
				}
				if results[1]["price"] != "5.50" {
					t.Errorf("second price = %v", results[1]["price"])
				}
			},
		},
		{
			name: "named capture groups auto-populated",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "date", Pattern: `(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})`, Group: 0},
				},
			},
			text:    "Published on 2024-01-15.",
			wantLen: 1,
			checkFunc: func(t *testing.T, results []map[string]any) {
				if results[0]["year"] != "2024" {
					t.Errorf("year = %v", results[0]["year"])
				}
				if results[0]["month"] != "01" {
					t.Errorf("month = %v", results[0]["month"])
				}
				if results[0]["day"] != "15" {
					t.Errorf("day = %v", results[0]["day"])
				}
				if results[0]["date"] != "2024-01-15" {
					t.Errorf("date = %v", results[0]["date"])
				}
			},
		},
		{
			name: "no matches returns empty",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "zip", Pattern: `\d{5}`, Group: 0},
				},
			},
			text:    "no numbers here",
			wantLen: 0,
		},
		{
			name: "invalid regex returns error",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "bad", Pattern: `[invalid`, Group: 0},
				},
			},
			text:    "test",
			wantErr: true,
		},
		{
			name: "group out of range falls back to 0",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "word", Pattern: `\b(\w+)\b`, Group: 99},
				},
			},
			text:    "hello world",
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				// Group 99 doesn't exist, so falls back to group 0 (full match)
				if results[0]["word"] != "hello" {
					t.Errorf("expected hello, got %v", results[0]["word"])
				}
			},
		},
		{
			name: "multiple patterns",
			schema: RegexExtractionSchema{
				Patterns: []RegexPattern{
					{Name: "email", Pattern: `[\w.+-]+@[\w-]+\.[\w.]+`, Group: 0},
					{Name: "phone", Pattern: `\d{3}-\d{4}`, Group: 0},
				},
			},
			text:    "Contact alice@test.com or 555-1234.",
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				if results[0]["email"] != "alice@test.com" {
					t.Errorf("email = %v", results[0]["email"])
				}
				if results[1]["phone"] != "555-1234" {
					t.Errorf("phone = %v", results[1]["phone"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewRegexExtractor(tt.schema)
			results, err := ext.Extract(tt.text)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantLen {
				t.Fatalf("expected %d results, got %d", tt.wantLen, len(results))
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, results)
			}
		})
	}
}

func TestNewRegexExtractor(t *testing.T) {
	schema := RegexExtractionSchema{
		Patterns: []RegexPattern{{Name: "test", Pattern: `\d+`, Group: 0}},
	}
	ext := NewRegexExtractor(schema)
	if ext == nil {
		t.Fatal("expected non-nil extractor")
	}
	if len(ext.Schema.Patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(ext.Schema.Patterns))
	}
}
