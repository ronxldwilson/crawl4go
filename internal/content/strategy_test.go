package content

import (
	"testing"
)

func TestGetStrategy(t *testing.T) {
	tests := []struct {
		name     string
		wantName string
	}{
		{"css", "css"},
		{"xpath", "xpath"},
		{"regex", "regex"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := GetStrategy(tc.name)
			if s == nil {
				t.Fatalf("GetStrategy(%q) returned nil", tc.name)
			}
			if got := s.Name(); got != tc.wantName {
				t.Errorf("strategy.Name() = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func TestGetStrategy_Unknown(t *testing.T) {
	s := GetStrategy("nonexistent")
	if s != nil {
		t.Errorf("GetStrategy(\"nonexistent\") = %v, want nil", s)
	}
}

func TestStrategyRegistry(t *testing.T) {
	expected := []string{"css", "xpath", "regex"}

	for _, name := range expected {
		if _, ok := StrategyRegistry[name]; !ok {
			t.Errorf("StrategyRegistry missing %q", name)
		}
	}

	if len(StrategyRegistry) != len(expected) {
		t.Errorf("StrategyRegistry has %d entries, want %d", len(StrategyRegistry), len(expected))
	}
}

func TestCSSExtractionStrategy_Name(t *testing.T) {
	s := &CSSExtractionStrategy{}
	if got := s.Name(); got != "css" {
		t.Errorf("Name() = %q, want %q", got, "css")
	}
}

func TestXPathExtractionStrategy_Name(t *testing.T) {
	s := &XPathExtractionStrategy{}
	if got := s.Name(); got != "xpath" {
		t.Errorf("Name() = %q, want %q", got, "xpath")
	}
}

func TestRegexExtractionStrategy_Name(t *testing.T) {
	s := &RegexExtractionStrategy{}
	if got := s.Name(); got != "regex" {
		t.Errorf("Name() = %q, want %q", got, "regex")
	}
}
