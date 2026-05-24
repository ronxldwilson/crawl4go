package content

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractTables(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		wantLen   int
		checkFunc func(t *testing.T, tables []ExtractedTable)
	}{
		{
			name: "simple table with th headers",
			html: `<html><body><table>
				<caption>Sales</caption>
				<tr><th>Name</th><th>Amount</th></tr>
				<tr><td>Alice</td><td>100</td></tr>
				<tr><td>Bob</td><td>200</td></tr>
			</table></body></html>`,
			wantLen: 1,
			checkFunc: func(t *testing.T, tables []ExtractedTable) {
				tbl := tables[0]
				if tbl.Caption != "Sales" {
					t.Errorf("caption = %q, want Sales", tbl.Caption)
				}
				if len(tbl.Headers) != 2 || tbl.Headers[0] != "Name" {
					t.Errorf("headers = %v, want [Name Amount]", tbl.Headers)
				}
				if len(tbl.Rows) != 2 {
					t.Errorf("rows = %d, want 2", len(tbl.Rows))
				}
				if !tbl.IsDataTable {
					t.Error("expected IsDataTable to be true")
				}
			},
		},
		{
			name: "table without th promotes first row",
			html: `<html><body><table>
				<tr><td>H1</td><td>H2</td></tr>
				<tr><td>A</td><td>B</td></tr>
			</table></body></html>`,
			wantLen: 1,
			checkFunc: func(t *testing.T, tables []ExtractedTable) {
				tbl := tables[0]
				if len(tbl.Headers) != 2 || tbl.Headers[0] != "H1" {
					t.Errorf("headers = %v, want [H1 H2]", tbl.Headers)
				}
				if len(tbl.Rows) != 1 {
					t.Errorf("rows = %d, want 1", len(tbl.Rows))
				}
			},
		},
		{
			name: "multiple tables",
			html: `<html><body>
				<table><tr><td>A</td></tr></table>
				<table><tr><td>B</td></tr></table>
			</body></html>`,
			wantLen: 2,
		},
		{
			name:    "no tables",
			html:    `<html><body><p>No tables here</p></body></html>`,
			wantLen: 0,
		},
		{
			name: "table with thead and tbody",
			html: `<html><body><table>
				<thead><tr><th>Col1</th><th>Col2</th></tr></thead>
				<tbody>
					<tr><td>a</td><td>b</td></tr>
					<tr><td>c</td><td>d</td></tr>
				</tbody>
			</table></body></html>`,
			wantLen: 1,
			checkFunc: func(t *testing.T, tables []ExtractedTable) {
				tbl := tables[0]
				if len(tbl.Headers) != 2 || tbl.Headers[0] != "Col1" {
					t.Errorf("headers = %v, want [Col1 Col2]", tbl.Headers)
				}
				if len(tbl.Rows) != 2 {
					t.Errorf("rows = %d, want 2", len(tbl.Rows))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := ExtractTables(tt.html)
			if len(tables) != tt.wantLen {
				t.Fatalf("expected %d tables, got %d", tt.wantLen, len(tables))
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, tables)
			}
		})
	}
}

func TestTablesToMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		tables []ExtractedTable
		want   []string // substrings that must appear
		notWant []string
	}{
		{
			name: "data table with caption",
			tables: []ExtractedTable{
				{
					Headers:     []string{"Name", "Age"},
					Rows:        [][]string{{"Alice", "30"}, {"Bob", "25"}},
					Caption:     "People",
					IsDataTable: true,
				},
			},
			want: []string{"**People**", "| Name | Age |", "| --- | --- |", "| Alice | 30 |"},
		},
		{
			name: "non-data table skipped",
			tables: []ExtractedTable{
				{
					Headers:     []string{"X"},
					Rows:        [][]string{{"Y"}},
					IsDataTable: false,
				},
			},
			want: nil,
		},
		{
			name: "pipe in cell is escaped",
			tables: []ExtractedTable{
				{
					Headers:     []string{"Val"},
					Rows:        [][]string{{"a|b"}},
					IsDataTable: true,
				},
			},
			want: []string{`a\|b`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := TablesToMarkdown(tt.tables)
			for _, s := range tt.want {
				if !strings.Contains(md, s) {
					t.Errorf("markdown missing %q:\n%s", s, md)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(md, s) {
					t.Errorf("markdown should not contain %q:\n%s", s, md)
				}
			}
		})
	}
}

func TestTablesToJSON(t *testing.T) {
	tables := []ExtractedTable{
		{Headers: []string{"A"}, Rows: [][]string{{"1"}}, Score: 0.8, IsDataTable: true},
	}
	data, err := TablesToJSON(tables)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded []ExtractedTable
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 table, got %d", len(decoded))
	}
	if decoded[0].Headers[0] != "A" {
		t.Errorf("expected header A, got %s", decoded[0].Headers[0])
	}
}

func TestTablesToJSON_nil(t *testing.T) {
	data, err := TablesToJSON(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "[]" {
		t.Errorf("expected [], got %s", string(data))
	}
}

func TestIsColumnCountConsistent(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		rows    [][]string
		want    bool
	}{
		{"all consistent", []string{"a", "b"}, [][]string{{"1", "2"}, {"3", "4"}}, true},
		{"inconsistent", []string{"a", "b"}, [][]string{{"1", "2"}, {"3"}}, false},
		{"empty rows", []string{"a"}, nil, false},
		{"no headers modal match", nil, [][]string{{"1", "2"}, {"3", "4"}, {"5", "6"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isColumnCountConsistent(tt.headers, tt.rows)
			if got != tt.want {
				t.Errorf("isColumnCountConsistent = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScoreTable_highScore(t *testing.T) {
	// A well-formed data table should score >= 0.5
	html := `<html><body><table>
		<caption>Data</caption>
		<tr><th>A</th><th>B</th></tr>
		<tr><td>1</td><td>2</td></tr>
		<tr><td>3</td><td>4</td></tr>
	</table></body></html>`

	tables := ExtractTables(html)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].Score < 0.5 {
		t.Errorf("expected score >= 0.5, got %f", tables[0].Score)
	}
	if !tables[0].IsDataTable {
		t.Error("expected IsDataTable to be true")
	}
}
