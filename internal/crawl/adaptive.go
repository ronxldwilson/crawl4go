package crawl

import (
	"container/heap"
	"context"
	"math"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

// AdaptiveStrategy implements CrawlStrategy with statistical convergence detection.
// It scores and re-ranks pending URLs after each page crawl, and stops early when
// three weighted metrics (coverage, consistency, saturation) exceed ConfidenceThreshold.
type AdaptiveStrategy struct {
	// ConfidenceThreshold is the minimum weighted score (0–1) that signals convergence.
	ConfidenceThreshold float64

	// CoverageWeight, ConsistencyWeight, SaturationWeight must sum to 1.0 (they are
	// used as-is; callers are responsible for keeping them normalised if desired).
	CoverageWeight    float64
	ConsistencyWeight float64
	SaturationWeight  float64

	// QueryTerms are the terms we want to find.  Used for coverage and link scoring.
	QueryTerms []string

	// MinPages is the minimum number of crawled pages before convergence is checked.
	MinPages int
}

// NewAdaptiveStrategy returns an AdaptiveStrategy with sensible defaults.
func NewAdaptiveStrategy(queryTerms []string) *AdaptiveStrategy {
	lower := make([]string, len(queryTerms))
	for i, t := range queryTerms {
		lower[i] = strings.ToLower(t)
	}
	return &AdaptiveStrategy{
		ConfidenceThreshold: 0.85,
		CoverageWeight:      0.4,
		ConsistencyWeight:   0.3,
		SaturationWeight:    0.3,
		QueryTerms:          lower,
		MinPages:            5,
	}
}

// knowledgeBase accumulates term-frequency statistics across crawled pages.
type knowledgeBase struct {
	termFreqs        map[string]int // total occurrences of each term across all docs
	docFreqs         map[string]int // number of documents containing each term
	docCount         int
	uniqueTermsPerDoc []int // count of unique terms seen in each document
}

func newKnowledgeBase() *knowledgeBase {
	return &knowledgeBase{
		termFreqs: make(map[string]int),
		docFreqs:  make(map[string]int),
	}
}

// update ingests the term counts from a single document.
func (kb *knowledgeBase) update(termCounts map[string]int) {
	kb.docCount++
	uniqueInDoc := 0
	for term, cnt := range termCounts {
		if _, seen := kb.termFreqs[term]; !seen {
			uniqueInDoc++ // first time we ever see this term
		}
		kb.termFreqs[term] += cnt
		kb.docFreqs[term]++
	}
	kb.uniqueTermsPerDoc = append(kb.uniqueTermsPerDoc, len(termCounts))
}

// adaptivePQItem extends pqItem with an adaptive score field so the same heap
// infrastructure from bestfirst.go can be reused without conflict.
type adaptivePQItem struct {
	url       string
	parentURL string
	score     float64
	depth     int
}

type adaptivePriorityQueue []adaptivePQItem

func (pq adaptivePriorityQueue) Len() int            { return len(pq) }
func (pq adaptivePriorityQueue) Less(i, j int) bool  { return pq[i].score > pq[j].score }
func (pq adaptivePriorityQueue) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *adaptivePriorityQueue) Push(x any)         { *pq = append(*pq, x.(adaptivePQItem)) }
func (pq *adaptivePriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

// Run executes the adaptive crawl, returning all collected results and statistics.
func (s *AdaptiveStrategy) Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats) {
	start := time.Now()

	visited := make(map[string]bool)
	depths := make(map[string]int)
	kb := newKnowledgeBase()

	baseU, _ := url.Parse(startURL)
	normalizedStart := content.NormalizeURL(startURL, baseU)
	if normalizedStart == "" {
		normalizedStart = startURL
	}

	visited[normalizedStart] = true
	depths[normalizedStart] = 0

	pq := &adaptivePriorityQueue{}
	heap.Init(pq)
	heap.Push(pq, adaptivePQItem{url: startURL, parentURL: "", score: 1.0, depth: 0})

	var allResults []DeepCrawlResult
	stats := CrawlStats{}

	// pending holds all discovered-but-not-yet-visited links so we can re-rank them.
	pending := make(map[string]adaptivePQItem)

	for (pq.Len() > 0 || len(pending) > 0) && stats.PagesCrawled < opts.MaxPages {
		if ctx.Err() != nil {
			break
		}

		// Re-rank pending links into the priority queue before picking the next URL.
		s.rerank(pq, pending, kb, opts)

		if pq.Len() == 0 {
			break
		}

		item := heap.Pop(pq).(adaptivePQItem)

		result, err := crawlFn(ctx, item.url)
		if err != nil {
			continue
		}
		result.Depth = item.depth
		result.ParentURL = item.parentURL
		result.Score = item.score
		stats.PagesCrawled++

		if result.Blocked {
			stats.PagesBlocked++
		}
		if result.Depth > stats.MaxDepthReached {
			stats.MaxDepthReached = result.Depth
		}
		allResults = append(allResults, *result)

		// Update knowledge base with the page content.
		termCounts := tokenizeContent(result.Content)
		kb.update(termCounts)

		// Check convergence after the minimum page threshold.
		if kb.docCount >= s.MinPages && s.converged(kb) {
			break
		}

		// Discover new links and add them to pending for re-ranking next iteration.
		if result.Depth < opts.MaxDepth {
			newLinks := discoverLinks(ctx, *result, visited, depths, result.Depth+1, opts)
			for _, link := range newLinks {
				if _, already := pending[link]; !already {
					pending[link] = adaptivePQItem{
						url:       link,
						parentURL: result.URL,
						score:     0, // will be set in rerank
						depth:     result.Depth + 1,
					}
				}
			}
		}
	}

	stats.TotalTimeMs = time.Since(start).Milliseconds()
	return allResults, stats
}

// rerank drains pending into the priority queue, computing a fresh score for each URL.
func (s *AdaptiveStrategy) rerank(pq *adaptivePriorityQueue, pending map[string]adaptivePQItem, kb *knowledgeBase, opts CrawlOptions) {
	// Drain items already in the queue back into pending so we can re-score them.
	for pq.Len() > 0 {
		item := heap.Pop(pq).(adaptivePQItem)
		pending[item.url] = item
	}

	for rawURL, item := range pending {
		item.score = s.scoreURL(rawURL, item.depth, kb, opts)
		heap.Push(pq, item)
		delete(pending, rawURL)
	}
	heap.Init(pq)
}

// scoreURL computes the adaptive ranking score for a candidate URL.
//
// Score = keywordOverlap + novelty - depthPenalty, normalised to [0, 1].
func (s *AdaptiveStrategy) scoreURL(rawURL string, depth int, kb *knowledgeBase, opts CrawlOptions) float64 {
	u, err := url.Parse(rawURL)
	pathTokens := make(map[string]bool)
	if err == nil {
		for _, tok := range tokenizeContent(u.Path) {
			// tokenizeContent returns map[string]int; we just need presence
			_ = tok
		}
		for term := range tokenizeContent(u.Path) {
			pathTokens[term] = true
		}
		for term := range tokenizeContent(u.RawQuery) {
			pathTokens[term] = true
		}
	}

	// Keyword overlap: fraction of QueryTerms present in the URL path/query.
	keywordOverlap := 0.0
	if len(s.QueryTerms) > 0 {
		matches := 0
		for _, qt := range s.QueryTerms {
			if pathTokens[qt] {
				matches++
			}
		}
		keywordOverlap = float64(matches) / float64(len(s.QueryTerms))
	}

	// Novelty: fraction of path tokens that have never appeared in the knowledge base.
	novelty := 0.0
	if len(pathTokens) > 0 {
		newTerms := 0
		for term := range pathTokens {
			if kb.termFreqs[term] == 0 {
				newTerms++
			}
		}
		novelty = float64(newTerms) / float64(len(pathTokens))
	}

	// Also use the opts scorer if provided, as a secondary signal.
	optsScore := 0.0
	if opts.Scorer != nil {
		optsScore = opts.Scorer.Score(rawURL)
	}

	// Depth penalty: deeper pages cost more (linear decay capped at depth 10).
	depthPenalty := math.Min(float64(depth)*0.05, 0.5)

	// Combine: keyword overlap is the primary driver, novelty and optsScore are secondary.
	combined := 0.5*keywordOverlap + 0.3*novelty + 0.2*optsScore - depthPenalty
	if combined < 0 {
		combined = 0
	}
	return combined
}

// converged returns true when the weighted convergence score exceeds ConfidenceThreshold.
func (s *AdaptiveStrategy) converged(kb *knowledgeBase) bool {
	coverage := s.computeCoverage(kb)
	consistency := s.computeConsistency(kb)
	saturation := s.computeSaturation(kb)

	weighted := s.CoverageWeight*coverage + s.ConsistencyWeight*consistency + s.SaturationWeight*saturation
	return weighted > s.ConfidenceThreshold
}

// computeCoverage returns the fraction of QueryTerms that appear at least once in the corpus.
func (s *AdaptiveStrategy) computeCoverage(kb *knowledgeBase) float64 {
	if len(s.QueryTerms) == 0 {
		return 1.0
	}
	found := 0
	for _, term := range s.QueryTerms {
		if kb.termFreqs[term] > 0 {
			found++
		}
	}
	return float64(found) / float64(len(s.QueryTerms))
}

// computeConsistency measures how stable term frequencies are across documents.
// It returns 1.0 - normalised stddev of per-term average frequencies, clamped to [0, 1].
func (s *AdaptiveStrategy) computeConsistency(kb *knowledgeBase) float64 {
	if kb.docCount == 0 || len(kb.termFreqs) == 0 {
		return 0
	}

	// Build slice of normalised term frequencies (avg occurrences per doc).
	freqs := make([]float64, 0, len(kb.termFreqs))
	for _, cnt := range kb.termFreqs {
		freqs = append(freqs, float64(cnt)/float64(kb.docCount))
	}

	mean := 0.0
	for _, f := range freqs {
		mean += f
	}
	mean /= float64(len(freqs))

	variance := 0.0
	for _, f := range freqs {
		d := f - mean
		variance += d * d
	}
	variance /= float64(len(freqs))
	stddev := math.Sqrt(variance)

	// Normalise stddev by mean to get coefficient of variation; invert for consistency.
	if mean == 0 {
		return 1.0
	}
	cv := stddev / mean
	// CV of 0 → perfect consistency (1.0); we use a soft cap so it reaches 1.0 quickly.
	consistency := 1.0 / (1.0 + cv)
	return consistency
}

// computeSaturation measures how quickly the rate of new unique terms is decreasing.
// A high saturation score means recent pages contribute fewer new terms (corpus is saturated).
func (s *AdaptiveStrategy) computeSaturation(kb *knowledgeBase) float64 {
	n := len(kb.uniqueTermsPerDoc)
	if n == 0 {
		return 0
	}

	// Average unique terms per doc across all docs.
	sum := 0
	for _, u := range kb.uniqueTermsPerDoc {
		sum += u
	}
	avgAll := float64(sum) / float64(n)

	// Average for the most recent half (or last doc if only one).
	recentStart := n / 2
	if recentStart == n {
		recentStart = n - 1
	}
	recentSum := 0
	for _, u := range kb.uniqueTermsPerDoc[recentStart:] {
		recentSum += u
	}
	recentCount := n - recentStart
	avgRecent := float64(recentSum) / float64(recentCount)

	if avgAll == 0 {
		return 1.0
	}

	// Saturation = how much the recent rate has dropped below the overall average.
	// If avgRecent <= 0 the corpus is fully saturated (1.0).
	// If avgRecent >= avgAll, saturation is 0.
	ratio := avgRecent / avgAll
	saturation := 1.0 - ratio
	if saturation < 0 {
		saturation = 0
	}
	return saturation
}

// tokenizeContent lowercases text, splits on whitespace and punctuation, and returns
// a map of token → occurrence count.  Single-character tokens are skipped.
func tokenizeContent(text string) map[string]int {
	counts := make(map[string]int)
	lower := strings.ToLower(text)

	// Split on any character that is not a letter or digit.
	var sb strings.Builder
	flush := func() {
		tok := sb.String()
		sb.Reset()
		if len(tok) > 1 {
			counts[tok]++
		}
	}

	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()

	return counts
}
