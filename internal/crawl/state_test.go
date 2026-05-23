package crawl

import (
	"reflect"
	"testing"
)

func TestSaveLoadState(t *testing.T) {
	tests := []struct {
		name  string
		state CrawlState
	}{
		{
			name: "round-trip with data",
			state: CrawlState{
				Visited:  map[string]bool{"https://a.com": true, "https://b.com": false},
				Pending:  []string{"https://c.com", "https://d.com"},
				Depths:   map[string]int{"https://a.com": 0, "https://c.com": 2},
				Scores:   map[string]float64{"https://a.com": 0.95},
				Strategy: "bfs",
				StartURL: "https://a.com",
			},
		},
		{
			name: "empty state",
			state: CrawlState{
				Visited: nil,
				Pending: nil,
				Depths:  nil,
				Scores:  nil,
			},
		},
		{
			name: "state with empty maps",
			state: CrawlState{
				Visited:  map[string]bool{},
				Pending:  []string{},
				Depths:   map[string]int{},
				Scores:   map[string]float64{},
				Strategy: "dfs",
				StartURL: "https://example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := SaveState(tt.state)
			if err != nil {
				t.Fatalf("SaveState error: %v", err)
			}

			loaded, err := LoadState(data)
			if err != nil {
				t.Fatalf("LoadState error: %v", err)
			}

			if !reflect.DeepEqual(normaliseState(tt.state), normaliseState(loaded)) {
				t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", loaded, tt.state)
			}
		})
	}
}

// normaliseState converts nil slices/maps to empty ones so DeepEqual works
// consistently after JSON round-tripping (which turns nil → null → nil but
// empty → [] → empty).
func normaliseState(s CrawlState) CrawlState {
	if s.Visited == nil {
		s.Visited = map[string]bool{}
	}
	if s.Pending == nil {
		s.Pending = []string{}
	}
	if s.Depths == nil {
		s.Depths = map[string]int{}
	}
	if s.Scores == nil {
		s.Scores = map[string]float64{}
	}
	return s
}

func TestLoadStateInvalidJSON(t *testing.T) {
	_, err := LoadState([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
