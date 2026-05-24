package content

import (
	"testing"
)

func TestAnalyzeStructure(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		checkFunc func(t *testing.T, ps PageStructure)
	}{
		{
			name: "title extraction",
			html: `<html><head><title>My Page</title></head><body></body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if ps.Title != "My Page" {
					t.Errorf("Title = %q, want My Page", ps.Title)
				}
			},
		},
		{
			name: "heading extraction",
			html: `<html><body>
				<h1>First H1</h1>
				<h1>Second H1</h1>
				<h2>An H2</h2>
				<h3>An H3</h3>
			</body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if len(ps.H1) != 2 {
					t.Errorf("H1 count = %d, want 2", len(ps.H1))
				}
				if len(ps.H2) != 1 || ps.H2[0] != "An H2" {
					t.Errorf("H2 = %v, want [An H2]", ps.H2)
				}
				if len(ps.H3) != 1 || ps.H3[0] != "An H3" {
					t.Errorf("H3 = %v, want [An H3]", ps.H3)
				}
			},
		},
		{
			name: "element counting",
			html: `<html><body>
				<form></form>
				<form></form>
				<a href="#">Link1</a>
				<a href="#">Link2</a>
				<a href="#">Link3</a>
				<img src="a.png">
				<script></script>
				<style></style>
				<iframe></iframe>
				<nav></nav>
			</body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if ps.FormCount != 2 {
					t.Errorf("FormCount = %d, want 2", ps.FormCount)
				}
				if ps.LinkCount != 3 {
					t.Errorf("LinkCount = %d, want 3", ps.LinkCount)
				}
				if ps.ImageCount != 1 {
					t.Errorf("ImageCount = %d, want 1", ps.ImageCount)
				}
				if ps.ScriptCount != 1 {
					t.Errorf("ScriptCount = %d, want 1", ps.ScriptCount)
				}
				if ps.StyleCount != 1 {
					t.Errorf("StyleCount = %d, want 1", ps.StyleCount)
				}
				if ps.IframeCount != 1 {
					t.Errorf("IframeCount = %d, want 1", ps.IframeCount)
				}
				if ps.NavCount != 1 {
					t.Errorf("NavCount = %d, want 1", ps.NavCount)
				}
			},
		},
		{
			name: "semantic tags",
			html: `<html><body>
				<header>H</header>
				<main>M</main>
				<article>A</article>
				<aside>S</aside>
				<footer>F</footer>
			</body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if !ps.HasHeader {
					t.Error("expected HasHeader = true")
				}
				if !ps.HasMainTag {
					t.Error("expected HasMainTag = true")
				}
				if !ps.HasArticle {
					t.Error("expected HasArticle = true")
				}
				if !ps.HasAside {
					t.Error("expected HasAside = true")
				}
				if !ps.HasFooter {
					t.Error("expected HasFooter = true")
				}
			},
		},
		{
			name: "no semantic tags",
			html: `<html><body><div>Just a div</div></body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if ps.HasHeader || ps.HasMainTag || ps.HasArticle || ps.HasAside || ps.HasFooter {
					t.Error("expected all semantic flags to be false")
				}
			},
		},
		{
			name: "meta tags extraction",
			html: `<html><head>
				<meta name="description" content="A test page">
				<meta property="og:title" content="OG Title">
				<meta name="keywords" content="go,test">
			</head><body></body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if ps.MetaTags["description"] != "A test page" {
					t.Errorf("meta description = %q", ps.MetaTags["description"])
				}
				if ps.MetaTags["og:title"] != "OG Title" {
					t.Errorf("meta og:title = %q", ps.MetaTags["og:title"])
				}
				if ps.MetaTags["keywords"] != "go,test" {
					t.Errorf("meta keywords = %q", ps.MetaTags["keywords"])
				}
			},
		},
		{
			name: "empty html",
			html: "",
			checkFunc: func(t *testing.T, ps PageStructure) {
				if ps.Title != "" {
					t.Errorf("expected empty title, got %q", ps.Title)
				}
				if ps.MetaTags == nil {
					t.Error("MetaTags should be initialized, not nil")
				}
			},
		},
		{
			name: "meta without content attribute ignored",
			html: `<html><head><meta name="viewport"></head><body></body></html>`,
			checkFunc: func(t *testing.T, ps PageStructure) {
				if _, ok := ps.MetaTags["viewport"]; ok {
					t.Error("meta without content should not be stored")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := AnalyzeStructure(tt.html)
			tt.checkFunc(t, ps)
		})
	}
}
