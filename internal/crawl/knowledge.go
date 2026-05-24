package crawl

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// KBEntry represents a single knowledge-base entry produced by the
// adaptive crawler.
type KBEntry struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Quality   float64   `json:"quality"`
	CrawledAt time.Time `json:"crawled_at"`
	Tags      []string  `json:"tags,omitempty"`
}

// KnowledgeBase holds a collection of knowledge-base entries with optional
// metadata.
type KnowledgeBase struct {
	Entries  []KBEntry         `json:"entries"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ExportJSONL writes entries in JSON Lines format (one JSON object per line).
func ExportJSONL(w io.Writer, entries []KBEntry) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for i := range entries {
		if err := enc.Encode(&entries[i]); err != nil {
			return err
		}
	}
	return nil
}

// ImportJSONL reads KBEntry values from a JSON Lines stream.
func ImportJSONL(r io.Reader) ([]KBEntry, error) {
	var entries []KBEntry
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry KBEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

// AddEntry appends entry to kb, deduplicating by URL. If an entry with the
// same URL already exists it is replaced with the new one.
func AddEntry(kb *KnowledgeBase, entry KBEntry) {
	for i, e := range kb.Entries {
		if e.URL == entry.URL {
			kb.Entries[i] = entry
			return
		}
	}
	kb.Entries = append(kb.Entries, entry)
}

// SearchEntries performs a simple case-insensitive keyword search across the
// Title and Summary fields. All entries whose Title or Summary contain the
// query substring are returned.
func SearchEntries(kb *KnowledgeBase, query string) []KBEntry {
	q := strings.ToLower(query)
	var results []KBEntry
	for _, e := range kb.Entries {
		if strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.Summary), q) {
			results = append(results, e)
		}
	}
	return results
}
