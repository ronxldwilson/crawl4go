package content

import (
	"testing"
)

func TestCSSExtractor(t *testing.T) {
	tests := []struct {
		name       string
		html       string
		schema     ExtractionSchema
		wantCount  int
		checkFirst func(t *testing.T, obj map[string]any)
	}{
		{
			name: "simple tag selector",
			html: `<html><body><p>Hello</p><p>World</p></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: "p",
				Fields: []ExtractionField{
					{Name: "items", Selector: "", Type: "list"},
				},
			},
			wantCount: 2,
			checkFirst: func(t *testing.T, obj map[string]any) {
				items := obj["items"].([]string)
				if len(items) != 1 || items[0] != "Hello" {
					t.Errorf("items = %v, want [Hello]", items)
				}
			},
		},
		{
			name: "class selector",
			html: `<html><body><div class="item"><span>A</span></div><div><span>B</span></div><div class="item"><span>C</span></div></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: ".item",
				Fields: []ExtractionField{
					{Name: "content", Selector: "span", Type: "text"},
				},
			},
			wantCount: 2,
			checkFirst: func(t *testing.T, obj map[string]any) {
				if obj["content"] != "A" {
					t.Errorf("content = %v, want A", obj["content"])
				}
			},
		},
		{
			name: "id selector",
			html: `<html><body><div id="main"><span>Main Content</span></div><div>Other</div></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: "#main",
				Fields: []ExtractionField{
					{Name: "text", Selector: "span", Type: "text"},
				},
			},
			wantCount: 1,
			checkFirst: func(t *testing.T, obj map[string]any) {
				if obj["text"] != "Main Content" {
					t.Errorf("text = %v, want Main Content", obj["text"])
				}
			},
		},
		{
			name: "descendant combinator",
			html: `<html><body><div class="container"><span><em>Inside</em></span></div><span>Outside</span></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: ".container span",
				Fields: []ExtractionField{
					{Name: "text", Selector: "em", Type: "text"},
				},
			},
			wantCount: 1,
			checkFirst: func(t *testing.T, obj map[string]any) {
				if obj["text"] != "Inside" {
					t.Errorf("text = %v, want Inside", obj["text"])
				}
			},
		},
		{
			name: "attribute selector",
			html: `<html><body><a href="https://example.com"><span>Link</span></a><a><span>No href</span></a></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: "a[href]",
				Fields: []ExtractionField{
					{Name: "text", Selector: "span", Type: "text"},
				},
			},
			wantCount: 1,
			checkFirst: func(t *testing.T, obj map[string]any) {
				if obj["text"] != "Link" {
					t.Errorf("text = %v, want Link", obj["text"])
				}
			},
		},
		{
			name: "text and attribute extraction types",
			html: `<html><body><div class="link"><a href="https://example.com">Link Text</a></div></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: ".link",
				Fields: []ExtractionField{
					{Name: "url", Selector: "a", Type: "attribute", Attribute: "href"},
					{Name: "label", Selector: "a", Type: "text"},
				},
			},
			wantCount: 1,
			checkFirst: func(t *testing.T, obj map[string]any) {
				if obj["url"] != "https://example.com" {
					t.Errorf("url = %v, want https://example.com", obj["url"])
				}
				if obj["label"] != "Link Text" {
					t.Errorf("label = %v, want Link Text", obj["label"])
				}
			},
		},
		{
			name: "nested fields",
			html: `<html><body><div class="card"><h2>Title</h2><p>Description</p></div></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: ".card",
				Fields: []ExtractionField{
					{
						Name:     "details",
						Selector: "",
						Type:     "nested",
						Fields: []ExtractionField{
							{Name: "title", Selector: "h2", Type: "text"},
							{Name: "desc", Selector: "p", Type: "text"},
						},
					},
				},
			},
			wantCount: 1,
			checkFirst: func(t *testing.T, obj map[string]any) {
				nested, ok := obj["details"].(map[string]any)
				if !ok {
					t.Fatalf("details is not map[string]any: %T", obj["details"])
				}
				if nested["title"] != "Title" {
					t.Errorf("nested title = %v, want Title", nested["title"])
				}
				if nested["desc"] != "Description" {
					t.Errorf("nested desc = %v, want Description", nested["desc"])
				}
			},
		},
		{
			name: "list type",
			html: `<html><body><ul><li>One</li><li>Two</li><li>Three</li></ul></body></html>`,
			schema: ExtractionSchema{
				BaseSelector: "ul",
				Fields: []ExtractionField{
					{Name: "items", Selector: "li", Type: "list"},
				},
			},
			wantCount: 1,
			checkFirst: func(t *testing.T, obj map[string]any) {
				items, ok := obj["items"].([]string)
				if !ok {
					t.Fatalf("items is not []string: %T", obj["items"])
				}
				if len(items) != 3 {
					t.Fatalf("items count = %d, want 3", len(items))
				}
				if items[0] != "One" || items[1] != "Two" || items[2] != "Three" {
					t.Errorf("items = %v, want [One Two Three]", items)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewCSSExtractor(tt.schema)
			results, err := ext.Extract(tt.html)
			if err != nil {
				t.Fatalf("Extract error: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Fatalf("result count = %d, want %d", len(results), tt.wantCount)
			}
			if tt.checkFirst != nil && len(results) > 0 {
				tt.checkFirst(t, results[0])
			}
		})
	}
}
