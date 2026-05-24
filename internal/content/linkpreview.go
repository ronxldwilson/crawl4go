package content

import (
	"context"
	"math"
	"net/http"
	"sync"
	"time"
)

// ScoredPreview holds metadata and a BM25 relevance score for a previewed URL.
// It complements the existing LinkPreview type by adding query-based scoring.
type ScoredPreview struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ContentType string  `json:"content_type"`
	StatusCode  int     `json:"status_code"`
	Score       float64 `json:"score"`
}

// PreviewFetcher fetches link previews concurrently with optional BM25 scoring.
type PreviewFetcher struct {
	Timeout      time.Duration
	MaxBodyBytes int
	UserAgent    string
	Concurrency  int
}

// NewPreviewFetcher returns a configured PreviewFetcher.
func NewPreviewFetcher(timeout time.Duration, maxBodyBytes int, userAgent string, concurrency int) *PreviewFetcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if maxBodyBytes <= 0 {
		maxBodyBytes = 32 * 1024
	}
	if userAgent == "" {
		userAgent = "crawl4go/1.0"
	}
	if concurrency <= 0 {
		concurrency = 5
	}
	return &PreviewFetcher{
		Timeout:      timeout,
		MaxBodyBytes: maxBodyBytes,
		UserAgent:    userAgent,
		Concurrency:  concurrency,
	}
}

// FetchPreview performs a partial HTTP GET and extracts title/description
// from OG/meta tags using the existing PeekHead function.
func (pf *PreviewFetcher) FetchPreview(ctx context.Context, rawURL string) (*ScoredPreview, error) {
	client := &http.Client{Timeout: pf.Timeout}

	peek, err := PeekHead(ctx, rawURL, client)
	if err != nil {
		return nil, err
	}

	sp := &ScoredPreview{
		URL:         rawURL,
		StatusCode:  peek.StatusCode,
		ContentType: peek.ContentType,
	}

	// Prefer OG title over standard title
	if peek.OGTitle != "" {
		sp.Title = peek.OGTitle
	} else {
		sp.Title = peek.Title
	}

	// Use meta description (OG description or standard)
	if ogDesc, ok := peek.Meta["og:description"]; ok && ogDesc != "" {
		sp.Description = ogDesc
	} else {
		sp.Description = peek.Description
	}

	return sp, nil
}

// FetchPreviews fetches multiple link previews concurrently and scores
// each against the given query using a BM25-like term relevance formula.
func (pf *PreviewFetcher) FetchPreviews(ctx context.Context, urls []string, query string) ([]ScoredPreview, error) {
	type result struct {
		preview *ScoredPreview
		err     error
	}

	results := make([]result, len(urls))
	sem := make(chan struct{}, pf.Concurrency)
	var wg sync.WaitGroup

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, rawURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sp, err := pf.FetchPreview(ctx, rawURL)
			results[idx] = result{preview: sp, err: err}
		}(i, u)
	}
	wg.Wait()

	var previews []ScoredPreview
	for _, r := range results {
		if r.err != nil || r.preview == nil {
			continue
		}
		previews = append(previews, *r.preview)
	}

	if query != "" && len(previews) > 0 {
		scorePreviewsBM25(previews, query)
	}

	return previews, nil
}

// scorePreviewsBM25 applies a simplified BM25 score to each preview
// by treating the combined title+description as a document and the
// query as the search terms. It reuses the tokenize function from bm25.go.
func scorePreviewsBM25(previews []ScoredPreview, query string) {
	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return
	}

	const k1 = 2.0
	const b = 0.75

	// Build per-document token lists
	type doc struct {
		tokens []string
	}
	docs := make([]doc, len(previews))
	totalLen := 0
	for i, p := range previews {
		text := p.Title + " " + p.Description
		docs[i].tokens = tokenize(text)
		totalLen += len(docs[i].tokens)
	}

	n := float64(len(previews))
	avgDL := float64(totalLen) / n

	// Document frequency for each query term
	df := make(map[string]int)
	for _, qt := range queryTokens {
		for _, d := range docs {
			for _, t := range d.tokens {
				if t == qt {
					df[qt]++
					break
				}
			}
		}
	}

	for i, d := range docs {
		tf := make(map[string]int)
		for _, t := range d.tokens {
			tf[t]++
		}

		dl := float64(len(d.tokens))
		score := 0.0

		for _, qt := range queryTokens {
			termFreq := float64(tf[qt])
			if termFreq == 0 {
				continue
			}
			docFreq := float64(df[qt])
			idf := math.Log((n-docFreq+0.5)/(docFreq+0.5) + 1)
			tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*dl/avgDL))
			score += idf * tfNorm
		}

		previews[i].Score = score
	}
}
