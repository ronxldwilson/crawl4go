package content

import (
	"testing"
)

func TestDiffText(t *testing.T) {
	tests := []struct {
		name       string
		oldText    string
		newText    string
		wantSim    float64
		wantAdded  int
		wantRemoved int
	}{
		{
			name:       "identical texts",
			oldText:    "hello\nworld",
			newText:    "hello\nworld",
			wantSim:    1.0,
			wantAdded:  0,
			wantRemoved: 0,
		},
		{
			name:       "completely different texts",
			oldText:    "alpha\nbeta",
			newText:    "gamma\ndelta",
			wantSim:    0.0,
			wantAdded:  2,
			wantRemoved: 2,
		},
		{
			name:       "partial overlap",
			oldText:    "hello\nworld\nfoo",
			newText:    "hello\nworld\nbar",
			wantSim:    2.0 * 2 / (3 + 3), // 2 unchanged out of 3+3 unique lines
			wantAdded:  1,
			wantRemoved: 1,
		},
		{
			name:       "both empty",
			oldText:    "",
			newText:    "",
			wantSim:    0.0,
			wantAdded:  0,
			wantRemoved: 0,
		},
		{
			name:       "old empty",
			oldText:    "",
			newText:    "hello\nworld",
			wantSim:    0.0,
			wantAdded:  2,
			wantRemoved: 0,
		},
		{
			name:       "new empty",
			oldText:    "hello\nworld",
			newText:    "",
			wantSim:    0.0,
			wantAdded:  0,
			wantRemoved: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiffText(tt.oldText, tt.newText)

			if result.Similarity != tt.wantSim {
				t.Errorf("Similarity = %v, want %v", result.Similarity, tt.wantSim)
			}
			if len(result.Added) != tt.wantAdded {
				t.Errorf("Added count = %d, want %d", len(result.Added), tt.wantAdded)
			}
			if len(result.Removed) != tt.wantRemoved {
				t.Errorf("Removed count = %d, want %d", len(result.Removed), tt.wantRemoved)
			}
		})
	}
}

func TestContentHash(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "non-empty string", input: "hello world"},
		{name: "empty string", input: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h1 := ContentHash(tt.input)
			h2 := ContentHash(tt.input)

			if h1 != h2 {
				t.Errorf("ContentHash not deterministic: %q != %q", h1, h2)
			}
			if len(h1) != 64 {
				t.Errorf("ContentHash length = %d, want 64 hex chars", len(h1))
			}
		})
	}

	// Different inputs produce different hashes.
	if ContentHash("a") == ContentHash("b") {
		t.Error("ContentHash('a') should differ from ContentHash('b')")
	}
}
