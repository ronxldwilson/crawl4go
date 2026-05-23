package crawl

import "encoding/json"

// CrawlState captures the state of a deep crawl so it can be paused, exported,
// and later resumed by passing the state back via CrawlOptions.InitialState.
type CrawlState struct {
	Visited  map[string]bool    `json:"visited"`
	Pending  []string           `json:"pending"`
	Depths   map[string]int     `json:"depths"`
	Scores   map[string]float64 `json:"scores"`
	Strategy string             `json:"strategy"`
	StartURL string             `json:"start_url"`
}

// SaveState serialises a CrawlState to JSON bytes.
func SaveState(state CrawlState) ([]byte, error) {
	return json.Marshal(state)
}

// LoadState deserialises JSON bytes into a CrawlState.
func LoadState(data []byte) (CrawlState, error) {
	var state CrawlState
	err := json.Unmarshal(data, &state)
	return state, err
}
