package content

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestPruningFilter_Filter(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		wantContent bool // whether any content survives
	}{
		{
			name: "keeps main content",
			html: `<html><body>
				<article><p>This is a substantial article about Go programming with enough text to be considered real content for extraction purposes.</p></article>
			</body></html>`,
			wantContent: true,
		},
		{
			name:        "empty body",
			html:        `<html><body></body></html>`,
			wantContent: false,
		},
		{
			name: "removes script and style",
			html: `<html><body>
				<script>var x = 1;</script>
				<style>.foo { color: red; }</style>
				<p>Actual meaningful content here with enough words to matter</p>
			</body></html>`,
			wantContent: true,
		},
		{
			name: "removes nav",
			html: `<html><body>
				<nav><a href="/">Home</a><a href="/about">About</a></nav>
				<article><p>Main content paragraph with enough text to score well for the pruning filter algorithm</p></article>
			</body></html>`,
			wantContent: true,
		},
	}

	pf := NewPruningFilter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pf.Filter(tt.html)
			if err != nil {
				t.Fatalf("Filter error: %v", err)
			}
			hasContent := len(strings.TrimSpace(result)) > 0
			if hasContent != tt.wantContent {
				t.Errorf("hasContent = %v, want %v (result = %q)", hasContent, tt.wantContent, result)
			}
		})
	}
}

func TestPruningFilter_RemovesExcludedTags(t *testing.T) {
	html := `<html><body>
		<nav>Navigation links here</nav>
		<footer>Footer content here</footer>
		<script>alert('hi')</script>
		<style>.x{}</style>
		<iframe src="ad.html"></iframe>
		<p>This is real content that should survive the pruning filter</p>
	</body></html>`

	pf := NewPruningFilter()
	result, err := pf.Filter(html)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}

	for _, tag := range []string{"<nav", "<footer", "<script", "<style", "<iframe"} {
		if strings.Contains(strings.ToLower(result), tag) {
			t.Errorf("result should not contain %s", tag)
		}
	}
}

func TestNewPruningFilter_Defaults(t *testing.T) {
	pf := NewPruningFilter()
	if pf.Threshold != 0.48 {
		t.Errorf("Threshold = %f, want 0.48", pf.Threshold)
	}
}

func TestFindBody(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantBody bool
	}{
		{"with body", `<html><body><p>hi</p></body></html>`, true},
		{"no body tag", `<p>just a paragraph</p>`, true}, // html.Parse adds body
		{"empty", ``, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseHTML(tt.html)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			body := FindBody(doc)
			if tt.wantBody && body == nil {
				t.Error("FindBody returned nil, want non-nil")
			}
		})
	}
}

func TestDefaultPruningConfig(t *testing.T) {
	cfg := DefaultPruningConfig()
	if cfg.MinTextLength != 25 {
		t.Errorf("MinTextLength = %d, want 25", cfg.MinTextLength)
	}
	if cfg.MinWordCount != 3 {
		t.Errorf("MinWordCount = %d, want 3", cfg.MinWordCount)
	}
	if cfg.MaxLinkDensity != 0.7 {
		t.Errorf("MaxLinkDensity = %f, want 0.7", cfg.MaxLinkDensity)
	}
	if len(cfg.BoilerplatePatterns) == 0 {
		t.Error("BoilerplatePatterns should not be empty")
	}
}

func TestConfigurablePruningFilter_Name(t *testing.T) {
	f := NewConfigurablePruningFilter(DefaultPruningConfig())
	if f.Name() != "configurable-pruning" {
		t.Errorf("Name() = %q, want %q", f.Name(), "configurable-pruning")
	}
}

func TestConfigurablePruningFilter_Filter(t *testing.T) {
	cfg := DefaultPruningConfig()
	f := NewConfigurablePruningFilter(cfg)
	ctx := context.Background()

	tests := []struct {
		name     string
		blocks   []string
		wantKept int // number of blocks with Kept=true
	}{
		{
			name: "good content kept",
			blocks: []string{
				"<p>This is a substantial paragraph with plenty of meaningful words that should pass all thresholds for the configurable pruning filter.</p>",
			},
			wantKept: 1,
		},
		{
			name: "short text rejected",
			blocks: []string{
				"<p>Too short</p>",
			},
			wantKept: 0,
		},
		{
			name: "boilerplate penalized",
			blocks: []string{
				"<p>Please subscribe to our newsletter and accept our privacy policy and cookie terms of service</p>",
			},
			wantKept: 0,
		},
		{
			name: "high link density rejected",
			blocks: []string{
				`<p><a href="/">All</a> <a href="/b">links</a> <a href="/c">here</a> <a href="/d">nothing</a> <a href="/e">but</a> <a href="/f">anchors</a> <a href="/g">everywhere</a> <a href="/h">more</a> <a href="/i">links</a></p>`,
			},
			wantKept: 0,
		},
		{
			name:     "empty blocks",
			blocks:   []string{},
			wantKept: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := f.Filter(ctx, tt.blocks, "")
			if err != nil {
				t.Fatalf("Filter error: %v", err)
			}
			if len(results) != len(tt.blocks) {
				t.Fatalf("results count = %d, want %d", len(results), len(tt.blocks))
			}
			kept := 0
			for _, r := range results {
				if r.Kept {
					kept++
				}
			}
			if kept != tt.wantKept {
				t.Errorf("kept = %d, want %d", kept, tt.wantKept)
			}
		})
	}
}

// parseHTML is a small test helper.
func parseHTML(s string) (*html.Node, error) {
	return html.Parse(strings.NewReader(s))
}
