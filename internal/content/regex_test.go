package content

import (
	"testing"
)

func TestRegexExtractor(t *testing.T) {
	t.Run("simple pattern", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "num", Pattern: `\d+`, Group: 0},
			},
		})
		results, err := ext.Extract("foo 42 bar 99")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("result count = %d, want 2", len(results))
		}
		if results[0]["num"] != "42" {
			t.Errorf("results[0][num] = %v, want 42", results[0]["num"])
		}
		if results[1]["num"] != "99" {
			t.Errorf("results[1][num] = %v, want 99", results[1]["num"])
		}
	})

	t.Run("capture groups", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "first", Pattern: `(\w+)@(\w+)`, Group: 1},
			},
		})
		results, err := ext.Extract("user@example")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("result count = %d, want 1", len(results))
		}
		if results[0]["first"] != "user" {
			t.Errorf("first = %v, want user", results[0]["first"])
		}
	})

	t.Run("named groups", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "match", Pattern: `(?P<user>\w+)@(?P<domain>\w+)`, Group: 0},
			},
		})
		results, err := ext.Extract("alice@wonderland")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("result count = %d, want 1", len(results))
		}
		if results[0]["user"] != "alice" {
			t.Errorf("user = %v, want alice", results[0]["user"])
		}
		if results[0]["domain"] != "wonderland" {
			t.Errorf("domain = %v, want wonderland", results[0]["domain"])
		}
	})

	t.Run("out of range group index falls back to group 0", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "val", Pattern: `(\w+)`, Group: 99},
			},
		})
		results, err := ext.Extract("hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("result count = %d, want 1", len(results))
		}
		// falls back to full match (group 0)
		if results[0]["val"] != "hello" {
			t.Errorf("val = %v, want hello", results[0]["val"])
		}
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "num", Pattern: `\d+`, Group: 0},
			},
		})
		results, err := ext.Extract("no digits here")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("result count = %d, want 0", len(results))
		}
	})

	t.Run("multiple patterns", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "num", Pattern: `\d+`, Group: 0},
				{Name: "word", Pattern: `[a-z]+`, Group: 0},
			},
		})
		results, err := ext.Extract("abc 123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// expect 1 num match + 1 word match = 2 total
		if len(results) != 2 {
			t.Fatalf("result count = %d, want 2", len(results))
		}
	})

	t.Run("invalid regex returns error", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "bad", Pattern: `[invalid`, Group: 0},
			},
		})
		_, err := ext.Extract("some text")
		if err == nil {
			t.Fatal("expected error for invalid regex, got nil")
		}
	})

	t.Run("overlapping matches via non-overlapping engine", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "pair", Pattern: `\d\d`, Group: 0},
			},
		})
		// Go regex is non-overlapping: "1234" -> "12", "34"
		results, err := ext.Extract("1234")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("result count = %d, want 2", len(results))
		}
		if results[0]["pair"] != "12" {
			t.Errorf("results[0][pair] = %v, want 12", results[0]["pair"])
		}
		if results[1]["pair"] != "34" {
			t.Errorf("results[1][pair] = %v, want 34", results[1]["pair"])
		}
	})

	t.Run("empty text returns empty slice", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "num", Pattern: `\d+`, Group: 0},
			},
		})
		results, err := ext.Extract("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("result count = %d, want 0", len(results))
		}
	})

	t.Run("group 0 explicit returns full match", func(t *testing.T) {
		ext := NewRegexExtractor(RegexExtractionSchema{
			Patterns: []RegexPattern{
				{Name: "email", Pattern: `(\w+)@(\w+\.\w+)`, Group: 0},
			},
		})
		results, err := ext.Extract("contact me at bob@example.com please")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("result count = %d, want 1", len(results))
		}
		if results[0]["email"] != "bob@example.com" {
			t.Errorf("email = %v, want bob@example.com", results[0]["email"])
		}
	})
}
