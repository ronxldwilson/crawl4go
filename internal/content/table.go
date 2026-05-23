package content

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// ExtractedTable holds parsed data from a single HTML table element.
type ExtractedTable struct {
	Headers     []string   `json:"headers"`
	Rows        [][]string `json:"rows"`
	Caption     string     `json:"caption,omitempty"`
	Score       float64    `json:"score"`
	IsDataTable bool       `json:"is_data_table"`
}

// ExtractTables parses htmlContent and returns all tables found, scored for
// data-table-ness.
func ExtractTables(htmlContent string) []ExtractedTable {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}

	var tables []ExtractedTable

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			t := parseTable(n)
			tables = append(tables, t)
			// Do NOT recurse into nested tables here; parseTable handles them
			// via its own traversal which flags nested-table presence.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return tables
}

// parseTable extracts data from a single <table> node and computes its score.
func parseTable(tableNode *html.Node) ExtractedTable {
	var headers []string
	var rows [][]string
	var caption string
	hasThHeaders := false
	hasCaption := false
	hasNested := false
	headerRowIndex := -1 // index in rows[] that was promoted to headers

	// Collect all direct structural nodes: caption, thead, tbody, tfoot, tr
	// We walk the immediate children of <table> (and thead/tbody/tfoot).

	var collectRows func(n *html.Node, depth int) [][]string
	collectRows = func(n *html.Node, depth int) [][]string {
		var result [][]string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "caption":
				caption = strings.TrimSpace(ExtractText(c))
				hasCaption = caption != ""
			case "thead", "tbody", "tfoot":
				result = append(result, collectRows(c, depth)...)
			case "tr":
				row, hasTh := parseRow(c, &hasNested)
				if hasTh && !hasThHeaders {
					hasThHeaders = true
					// Use this row as headers (take the first th-bearing row).
					if len(headers) == 0 {
						headers = row
						headerRowIndex = len(result) // mark position (before appending)
						// Don't add it as a data row.
						continue
					}
				}
				result = append(result, row)
			case "table":
				hasNested = true
			}
		}
		return result
	}

	rows = collectRows(tableNode, 0)

	// If no <th>-based headers were found, promote the first row as headers.
	if len(headers) == 0 && len(rows) > 0 {
		headers = rows[0]
		rows = rows[1:]
		headerRowIndex = 0
	}
	_ = headerRowIndex

	score := scoreTable(headers, rows, hasThHeaders, hasCaption, hasNested, tableNode)
	isDataTable := score >= 0.5

	if headers == nil {
		headers = []string{}
	}
	if rows == nil {
		rows = [][]string{}
	}

	return ExtractedTable{
		Headers:     headers,
		Rows:        rows,
		Caption:     caption,
		Score:       score,
		IsDataTable: isDataTable,
	}
}

// parseRow extracts cell text from a <tr> node.
// It returns the cell texts and whether any cell was a <th>.
// hasNested is set to true if a nested <table> is found inside any cell.
func parseRow(tr *html.Node, hasNested *bool) ([]string, bool) {
	var cells []string
	hasTh := false
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.Data {
		case "th":
			hasTh = true
			cells = append(cells, strings.TrimSpace(ExtractText(c)))
			if containsTable(c) {
				*hasNested = true
			}
		case "td":
			cells = append(cells, strings.TrimSpace(ExtractText(c)))
			if containsTable(c) {
				*hasNested = true
			}
		}
	}
	return cells, hasTh
}

// containsTable reports whether any descendant of n is a <table> element.
func containsTable(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			if c.Data == "table" {
				return true
			}
			if containsTable(c) {
				return true
			}
		}
	}
	return false
}

// scoreTable computes a heuristic data-table score in [0, 1.1].
func scoreTable(headers []string, rows [][]string, hasThHeaders, hasCaption, hasNested bool, tableNode *html.Node) float64 {
	var score float64

	// +0.3 — has <th> headers
	if hasThHeaders {
		score += 0.3
	}

	// +0.2 — has <caption>
	if hasCaption {
		score += 0.2
	}

	// +0.3 — consistent column count across rows
	if isColumnCountConsistent(headers, rows) {
		score += 0.3
	}

	// +0.1 — more than 1 row of data
	if len(rows) > 1 {
		score += 0.1
	}

	// +0.1 — no nested tables
	if !hasNested {
		score += 0.1
	}

	// +0.1 — cells contain mostly text (avg text density > 0.7)
	if avgTextDensity(tableNode) > 0.7 {
		score += 0.1
	}

	return score
}

// isColumnCountConsistent returns true when at least 80 % of data rows have
// the same column count as the header row (or the modal count when there are
// no headers).
func isColumnCountConsistent(headers []string, rows [][]string) bool {
	if len(rows) == 0 {
		return false
	}

	// Build a frequency map of column counts.
	freq := make(map[int]int)
	for _, r := range rows {
		freq[len(r)]++
	}

	// Find the modal count.
	modal := 0
	modalCount := 0
	for cnt, n := range freq {
		if n > modalCount {
			modal = cnt
			modalCount = n
		}
	}

	// If headers exist, prefer their count as the reference.
	ref := modal
	if len(headers) > 0 {
		ref = len(headers)
	}

	consistent := 0
	for _, r := range rows {
		if len(r) == ref {
			consistent++
		}
	}

	return float64(consistent)/float64(len(rows)) >= 0.8
}

// avgTextDensity estimates the ratio of text characters to total rendered HTML
// characters for all leaf cells in the table.  A high ratio means cells
// contain plain text rather than complex markup.
func avgTextDensity(tableNode *html.Node) float64 {
	var totalHTML, totalText int

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "td" || n.Data == "th") {
			// Approximate HTML size as the length of rendered text plus tag overhead.
			// We count inner HTML by rendering child nodes.
			innerText := ExtractText(n)
			innerHTML := renderInnerHTML(n)
			totalText += len(innerText)
			totalHTML += len(innerHTML)
			return // don't recurse into cells again
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(tableNode)

	if totalHTML == 0 {
		return 1.0 // no markup at all — treat as pure text
	}
	return float64(totalText) / float64(totalHTML)
}

// renderInnerHTML produces a rough text representation of a node's inner
// content (text + tag names) so we can estimate markup density cheaply without
// importing html/template or a renderer.
func renderInnerHTML(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		switch cur.Type {
		case html.TextNode:
			sb.WriteString(cur.Data)
		case html.ElementNode:
			sb.WriteByte('<')
			sb.WriteString(cur.Data)
			for _, a := range cur.Attr {
				sb.WriteByte(' ')
				sb.WriteString(a.Key)
				sb.WriteString(`="`)
				sb.WriteString(a.Val)
				sb.WriteByte('"')
			}
			sb.WriteByte('>')
			for c := cur.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			sb.WriteString("</")
			sb.WriteString(cur.Data)
			sb.WriteByte('>')
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return sb.String()
}

// TablesToMarkdown converts all data tables (IsDataTable == true) to GitHub-
// Flavoured Markdown table syntax.  Non-data tables are skipped.
func TablesToMarkdown(tables []ExtractedTable) string {
	var sb strings.Builder

	for _, t := range tables {
		if !t.IsDataTable {
			continue
		}

		if t.Caption != "" {
			sb.WriteString("**")
			sb.WriteString(t.Caption)
			sb.WriteString("**\n\n")
		}

		// Determine column count.
		cols := len(t.Headers)
		if cols == 0 && len(t.Rows) > 0 {
			cols = len(t.Rows[0])
		}
		if cols == 0 {
			continue
		}

		// Header row.
		headers := t.Headers
		if len(headers) == 0 {
			// Synthesise empty headers so the separator row is well-formed.
			headers = make([]string, cols)
		}
		sb.WriteString("|")
		for _, h := range headers {
			sb.WriteString(" ")
			sb.WriteString(markdownEscape(h))
			sb.WriteString(" |")
		}
		sb.WriteByte('\n')

		// Separator row.
		sb.WriteString("|")
		for i := 0; i < cols; i++ {
			sb.WriteString(" --- |")
		}
		sb.WriteByte('\n')

		// Data rows.
		for _, row := range t.Rows {
			sb.WriteString("|")
			for ci := 0; ci < cols; ci++ {
				cell := ""
				if ci < len(row) {
					cell = row[ci]
				}
				sb.WriteString(" ")
				sb.WriteString(markdownEscape(cell))
				sb.WriteString(" |")
			}
			sb.WriteByte('\n')
		}

		sb.WriteByte('\n')
	}

	return strings.TrimRight(sb.String(), "\n")
}

// markdownEscape escapes pipe characters inside cell text so they don't break
// the table structure.
func markdownEscape(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// TablesToJSON serialises the full slice of ExtractedTable values to a JSON
// array.  All tables (not just data tables) are included so the caller can
// decide what to do with non-data tables.
func TablesToJSON(tables []ExtractedTable) ([]byte, error) {
	if tables == nil {
		tables = []ExtractedTable{}
	}
	return json.Marshal(tables)
}
