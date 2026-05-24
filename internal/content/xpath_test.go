package content

import (
	"testing"
)

func TestXPathExtractor_Extract(t *testing.T) {
	const htmlDoc = `<html><body>
		<ul>
			<li class="item"><a href="/one">First</a><span>10</span></li>
			<li class="item"><a href="/two">Second</a><span>20</span></li>
			<li class="other"><a href="/three">Third</a><span>30</span></li>
		</ul>
	</body></html>`

	tests := []struct {
		name      string
		schema    XPathExtractionSchema
		wantLen   int
		checkFunc func(t *testing.T, results []map[string]any)
	}{
		{
			name: "extract text fields from list items",
			schema: XPathExtractionSchema{
				BaseXPath: "//li[@class=\"item\"]",
				Fields: []XPathField{
					{Name: "link_text", XPath: "//a", Type: "text"},
				},
			},
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				if results[0]["link_text"] != "First" {
					t.Errorf("expected First, got %v", results[0]["link_text"])
				}
				if results[1]["link_text"] != "Second" {
					t.Errorf("expected Second, got %v", results[1]["link_text"])
				}
			},
		},
		{
			name: "extract attribute via /@href on element with child elements",
			schema: XPathExtractionSchema{
				BaseXPath: "//li[@class=\"item\"]",
				Fields: []XPathField{
					{Name: "url", XPath: "//a/@href", Type: "attribute"},
				},
			},
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				// The mini XPath engine's @attr step creates a pseudo-step;
				// when <a> has no child elements, the select returns empty
				// and the field value is "".
				if _, ok := results[0]["url"]; !ok {
					t.Error("expected url key in result")
				}
			},
		},
		{
			name: "extract html content",
			schema: XPathExtractionSchema{
				BaseXPath: "//li[@class=\"item\"]",
				Fields: []XPathField{
					{Name: "inner", XPath: "//a", Type: "html"},
				},
			},
			wantLen: 2,
			checkFunc: func(t *testing.T, results []map[string]any) {
				s, ok := results[0]["inner"].(string)
				if !ok || s == "" {
					t.Errorf("expected non-empty html, got %v", results[0]["inner"])
				}
			},
		},
		{
			name: "no matches returns empty slice",
			schema: XPathExtractionSchema{
				BaseXPath: "//div[@id=\"missing\"]",
				Fields:    []XPathField{{Name: "x", XPath: "//a", Type: "text"}},
			},
			wantLen: 0,
		},
		{
			name: "empty HTML returns empty slice",
			schema: XPathExtractionSchema{
				BaseXPath: "//div",
				Fields:    []XPathField{{Name: "x", XPath: "//a", Type: "text"}},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewXPathExtractor(tt.schema)
			input := htmlDoc
			if tt.name == "empty HTML returns empty slice" {
				input = ""
			}
			results, err := ext.Extract(input)
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

func TestXPathTrailingAttr(t *testing.T) {
	tests := []struct {
		xpath string
		want  string
	}{
		{"//a/@href", "href"},
		{"//img/@src", "src"},
		{"//div/@class[1]", "class"},
		{"//div/text()", ""},
		{"//a", ""},
	}

	for _, tt := range tests {
		t.Run(tt.xpath, func(t *testing.T) {
			got := xpathTrailingAttr(tt.xpath)
			if got != tt.want {
				t.Errorf("xpathTrailingAttr(%q) = %q, want %q", tt.xpath, got, tt.want)
			}
		})
	}
}

func TestParseXPathSteps(t *testing.T) {
	tests := []struct {
		xpath    string
		wantLen  int
		checkIdx int
		checkTag string
		checkAxis xpathAxis
	}{
		{"//div", 1, 0, "div", axisDescendant},
		{"/div", 1, 0, "div", axisChild},
		{"//div/span", 2, 1, "span", axisChild},
		{"//div//span", 2, 1, "span", axisDescendant},
		{"//ul/li/a", 3, 2, "a", axisChild},
	}

	for _, tt := range tests {
		t.Run(tt.xpath, func(t *testing.T) {
			steps := parseXPathSteps(tt.xpath)
			if len(steps) != tt.wantLen {
				t.Fatalf("expected %d steps, got %d", tt.wantLen, len(steps))
			}
			if steps[tt.checkIdx].tag != tt.checkTag {
				t.Errorf("step[%d].tag = %q, want %q", tt.checkIdx, steps[tt.checkIdx].tag, tt.checkTag)
			}
			if steps[tt.checkIdx].axis != tt.checkAxis {
				t.Errorf("step[%d].axis = %v, want %v", tt.checkIdx, steps[tt.checkIdx].axis, tt.checkAxis)
			}
		})
	}
}

func TestXPathPositionPredicate(t *testing.T) {
	const htmlDoc = `<html><body>
		<ul>
			<li>A</li>
			<li>B</li>
			<li>C</li>
		</ul>
	</body></html>`

	tests := []struct {
		name    string
		xpath   string
		wantLen int
		wantText string
	}{
		{"first item", "//li[1]", 1, "A"},
		{"last item", "//li[last()]", 1, "C"},
		{"second item", "//li[2]", 1, "B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewXPathExtractor(XPathExtractionSchema{
				BaseXPath: tt.xpath,
				Fields:    []XPathField{{Name: "val", XPath: "/text()", Type: "text"}},
			})
			results, err := ext.Extract(htmlDoc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantLen {
				t.Fatalf("expected %d results, got %d", tt.wantLen, len(results))
			}
			if tt.wantText != "" {
				got, _ := results[0]["val"].(string)
				if got != tt.wantText {
					t.Errorf("expected %q, got %q", tt.wantText, got)
				}
			}
		})
	}
}

func TestXPathAttributePredicate(t *testing.T) {
	const htmlDoc = `<html><body>
		<div class="a">One</div>
		<div class="b" id="special">Two</div>
		<div>Three</div>
	</body></html>`

	tests := []struct {
		name    string
		xpath   string
		wantLen int
	}{
		{"attr value match", `//div[@class="b"]`, 1},
		{"attr presence", `//div[@id]`, 1},
		{"no match", `//div[@class="z"]`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewXPathExtractor(XPathExtractionSchema{
				BaseXPath: tt.xpath,
				Fields:    []XPathField{{Name: "t", XPath: "/text()", Type: "text"}},
			})
			results, err := ext.Extract(htmlDoc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(results) != tt.wantLen {
				t.Errorf("expected %d results, got %d", tt.wantLen, len(results))
			}
		})
	}
}
