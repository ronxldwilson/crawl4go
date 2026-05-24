package content

import (
	"strings"
	"testing"
)

func TestAnalyzeStructure_EmptyHTML(t *testing.T) {
	ps := AnalyzeStructure("")
	if ps.Title != "" {
		t.Errorf("Title = %q, want empty", ps.Title)
	}
	if len(ps.H1) != 0 || len(ps.H2) != 0 || len(ps.H3) != 0 {
		t.Errorf("expected no headings, got h1=%v h2=%v h3=%v", ps.H1, ps.H2, ps.H3)
	}
	if ps.MetaTags == nil {
		t.Error("MetaTags should be non-nil for empty HTML")
	}
}

func TestAnalyzeStructure_TitleExtraction(t *testing.T) {
	html := `<html><head><title>Hello World</title></head><body></body></html>`
	ps := AnalyzeStructure(html)
	if ps.Title != "Hello World" {
		t.Errorf("Title = %q, want %q", ps.Title, "Hello World")
	}
}

func TestAnalyzeStructure_MultipleTitleTags(t *testing.T) {
	// Only the first title should be used.
	html := `<html><head><title>First</title></head><body><title>Second</title></body></html>`
	ps := AnalyzeStructure(html)
	if ps.Title != "First" {
		t.Errorf("Title = %q, want %q", ps.Title, "First")
	}
}

func TestAnalyzeStructure_HeadingSlices(t *testing.T) {
	html := `<html><body>
		<h1>H1 One</h1>
		<h1>H1 Two</h1>
		<h2>H2 Alpha</h2>
		<h2>H2 Beta</h2>
		<h3>H3 Deep</h3>
	</body></html>`
	ps := AnalyzeStructure(html)

	if len(ps.H1) != 2 {
		t.Fatalf("H1 count = %d, want 2", len(ps.H1))
	}
	if ps.H1[0] != "H1 One" || ps.H1[1] != "H1 Two" {
		t.Errorf("H1 = %v, want [H1 One, H1 Two]", ps.H1)
	}
	if len(ps.H2) != 2 {
		t.Fatalf("H2 count = %d, want 2", len(ps.H2))
	}
	if ps.H2[0] != "H2 Alpha" || ps.H2[1] != "H2 Beta" {
		t.Errorf("H2 = %v", ps.H2)
	}
	if len(ps.H3) != 1 || ps.H3[0] != "H3 Deep" {
		t.Errorf("H3 = %v, want [H3 Deep]", ps.H3)
	}
}

func TestAnalyzeStructure_Counters(t *testing.T) {
	html := `<html><body>
		<form></form><form></form>
		<a href="#">link1</a><a href="#">link2</a><a href="#">link3</a>
		<img src="a.png"><img src="b.png">
		<script></script>
		<style></style><style></style>
		<iframe></iframe>
		<nav></nav><nav></nav>
	</body></html>`
	ps := AnalyzeStructure(html)

	if ps.FormCount != 2 {
		t.Errorf("FormCount = %d, want 2", ps.FormCount)
	}
	if ps.LinkCount != 3 {
		t.Errorf("LinkCount = %d, want 3", ps.LinkCount)
	}
	if ps.ImageCount != 2 {
		t.Errorf("ImageCount = %d, want 2", ps.ImageCount)
	}
	if ps.ScriptCount != 1 {
		t.Errorf("ScriptCount = %d, want 1", ps.ScriptCount)
	}
	if ps.StyleCount != 2 {
		t.Errorf("StyleCount = %d, want 2", ps.StyleCount)
	}
	if ps.IframeCount != 1 {
		t.Errorf("IframeCount = %d, want 1", ps.IframeCount)
	}
	if ps.NavCount != 2 {
		t.Errorf("NavCount = %d, want 2", ps.NavCount)
	}
}

func TestAnalyzeStructure_SemanticBooleans(t *testing.T) {
	html := `<html><body>
		<main></main>
		<article></article>
		<aside></aside>
		<header></header>
		<footer></footer>
	</body></html>`
	ps := AnalyzeStructure(html)

	if !ps.HasMainTag {
		t.Error("HasMainTag should be true")
	}
	if !ps.HasArticle {
		t.Error("HasArticle should be true")
	}
	if !ps.HasAside {
		t.Error("HasAside should be true")
	}
	if !ps.HasHeader {
		t.Error("HasHeader should be true")
	}
	if !ps.HasFooter {
		t.Error("HasFooter should be true")
	}
}

func TestAnalyzeStructure_SemanticBooleansAbsent(t *testing.T) {
	html := `<html><body><p>No semantic elements</p></body></html>`
	ps := AnalyzeStructure(html)

	if ps.HasMainTag {
		t.Error("HasMainTag should be false")
	}
	if ps.HasArticle {
		t.Error("HasArticle should be false")
	}
	if ps.HasAside {
		t.Error("HasAside should be false")
	}
	if ps.HasHeader {
		t.Error("HasHeader should be false")
	}
	if ps.HasFooter {
		t.Error("HasFooter should be false")
	}
}

func TestAnalyzeStructure_MetaTagsByName(t *testing.T) {
	html := `<html><head>
		<meta name="description" content="Page description">
		<meta name="keywords" content="go, test">
	</head><body></body></html>`
	ps := AnalyzeStructure(html)

	if ps.MetaTags["description"] != "Page description" {
		t.Errorf("MetaTags[description] = %q, want %q", ps.MetaTags["description"], "Page description")
	}
	if ps.MetaTags["keywords"] != "go, test" {
		t.Errorf("MetaTags[keywords] = %q, want %q", ps.MetaTags["keywords"], "go, test")
	}
}

func TestAnalyzeStructure_MetaTagsOpenGraph(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="OG Title">
		<meta property="og:description" content="OG Desc">
		<meta property="og:image" content="https://example.com/img.jpg">
	</head><body></body></html>`
	ps := AnalyzeStructure(html)

	if ps.MetaTags["og:title"] != "OG Title" {
		t.Errorf("MetaTags[og:title] = %q, want %q", ps.MetaTags["og:title"], "OG Title")
	}
	if ps.MetaTags["og:description"] != "OG Desc" {
		t.Errorf("MetaTags[og:description] = %q, want %q", ps.MetaTags["og:description"], "OG Desc")
	}
	if ps.MetaTags["og:image"] != "https://example.com/img.jpg" {
		t.Errorf("MetaTags[og:image] = %q", ps.MetaTags["og:image"])
	}
}

func TestAnalyzeStructure_NestedElements(t *testing.T) {
	// Links and images inside headings or nav should still be counted.
	html := `<html><body>
		<nav>
			<a href="#">nav link 1</a>
			<a href="#">nav link 2</a>
		</nav>
		<h1>Title with <a href="#">link</a></h1>
		<img src="banner.png">
	</body></html>`
	ps := AnalyzeStructure(html)

	if ps.NavCount != 1 {
		t.Errorf("NavCount = %d, want 1", ps.NavCount)
	}
	// 3 <a> tags total (2 inside nav + 1 inside h1)
	if ps.LinkCount != 3 {
		t.Errorf("LinkCount = %d, want 3", ps.LinkCount)
	}
	if ps.ImageCount != 1 {
		t.Errorf("ImageCount = %d, want 1", ps.ImageCount)
	}
	if len(ps.H1) != 1 {
		t.Fatalf("H1 count = %d, want 1", len(ps.H1))
	}
	if !strings.Contains(ps.H1[0], "Title with") {
		t.Errorf("H1[0] = %q, expected to contain 'Title with'", ps.H1[0])
	}
}

func TestAnalyzeStructure_MalformedHTML(t *testing.T) {
	// The Go HTML parser is lenient, so this should not panic or return zero struct.
	html := `<html><body><h1>Unclosed<p>Para<h2>Also unclosed`
	ps := AnalyzeStructure(html)
	// Should still parse some headings
	if len(ps.H1) == 0 {
		t.Error("expected at least one H1 in malformed HTML")
	}
	// MetaTags must be initialised
	if ps.MetaTags == nil {
		t.Error("MetaTags should not be nil for malformed HTML")
	}
}
