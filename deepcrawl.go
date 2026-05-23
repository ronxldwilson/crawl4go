package main

import (
	"container/heap"
	"context"
	"log/slog"
	"net/url"
	"sync"
	"time"
)

type CrawlFunc func(ctx context.Context, pageURL string) (*DeepCrawlResult, error)

type CrawlStrategy interface {
	Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats)
}

type CrawlOptions struct {
	MaxDepth        int
	MaxPages        int
	IncludeExternal bool
	Filters         *FilterChain
	Scorer          URLScorer
	ScoreThreshold  float64
	Robots          *RobotsChecker
}

type DeepCrawlResult struct {
	URL          string  `json:"url"`
	Depth        int     `json:"depth"`
	ParentURL    string  `json:"parent_url,omitempty"`
	StatusCode   int     `json:"status_code"`
	Blocked      bool    `json:"blocked"`
	Content      string  `json:"content"`
	Links        LinkSet `json:"links"`
	Score        float64 `json:"score,omitempty"`
	RenderTimeMs int64   `json:"render_time_ms"`
}

type CrawlStats struct {
	PagesCrawled    int   `json:"pages_crawled"`
	PagesBlocked    int   `json:"pages_blocked"`
	MaxDepthReached int   `json:"max_depth_reached"`
	TotalTimeMs     int64 `json:"total_time_ms"`
}

// --- BFS Strategy ---

type BFSStrategy struct{}

func (s *BFSStrategy) Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats) {
	start := time.Now()
	visited := make(map[string]bool)
	depths := make(map[string]int)

	baseU, _ := url.Parse(startURL)
	normalizedStart := NormalizeURL(startURL, baseU)

	visited[normalizedStart] = true
	depths[normalizedStart] = 0

	type queueItem struct {
		url       string
		parentURL string
	}

	currentLevel := []queueItem{{url: startURL, parentURL: ""}}
	var allResults []DeepCrawlResult
	stats := CrawlStats{}

	for depth := 0; depth <= opts.MaxDepth && len(currentLevel) > 0; depth++ {
		if ctx.Err() != nil {
			break
		}

		results := make([]DeepCrawlResult, len(currentLevel))
		var wg sync.WaitGroup

		for i, item := range currentLevel {
			if stats.PagesCrawled >= opts.MaxPages {
				break
			}
			wg.Add(1)
			stats.PagesCrawled++

			go func(idx int, it queueItem) {
				defer wg.Done()
				result, err := crawlFn(ctx, it.url)
				if err != nil {
					return
				}
				result.Depth = depth
				result.ParentURL = it.parentURL
				results[idx] = *result
			}(i, item)
		}
		wg.Wait()

		var nextLevel []queueItem

		for _, result := range results {
			if result.URL == "" {
				continue
			}
			if result.Blocked {
				stats.PagesBlocked++
			}
			if depth > stats.MaxDepthReached {
				stats.MaxDepthReached = depth
			}
			allResults = append(allResults, result)

			if depth >= opts.MaxDepth {
				continue
			}

			newLinks := discoverLinks(ctx, result, visited, depths, depth+1, opts)
			for _, nl := range newLinks {
				nextLevel = append(nextLevel, queueItem{url: nl, parentURL: result.URL})
			}
		}

		if stats.PagesCrawled >= opts.MaxPages {
			break
		}
		currentLevel = nextLevel
	}

	stats.TotalTimeMs = time.Since(start).Milliseconds()
	return allResults, stats
}

// --- DFS Strategy ---

type DFSStrategy struct{}

func (s *DFSStrategy) Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats) {
	start := time.Now()
	visited := make(map[string]bool)
	depths := make(map[string]int)

	baseU, _ := url.Parse(startURL)
	normalizedStart := NormalizeURL(startURL, baseU)

	visited[normalizedStart] = true
	depths[normalizedStart] = 0

	type stackItem struct {
		url       string
		parentURL string
		depth     int
	}

	stack := []stackItem{{url: startURL, parentURL: "", depth: 0}}
	var allResults []DeepCrawlResult
	stats := CrawlStats{}

	for len(stack) > 0 && stats.PagesCrawled < opts.MaxPages {
		if ctx.Err() != nil {
			break
		}

		// Pop from stack (LIFO)
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if item.depth > opts.MaxDepth {
			continue
		}

		result, err := crawlFn(ctx, item.url)
		if err != nil {
			continue
		}
		result.Depth = item.depth
		result.ParentURL = item.parentURL
		stats.PagesCrawled++

		if result.Blocked {
			stats.PagesBlocked++
		}
		if item.depth > stats.MaxDepthReached {
			stats.MaxDepthReached = item.depth
		}
		allResults = append(allResults, *result)

		if item.depth >= opts.MaxDepth {
			continue
		}

		newLinks := discoverLinks(ctx, *result, visited, depths, item.depth+1, opts)
		// Reverse so first-discovered links are processed next
		for i := len(newLinks) - 1; i >= 0; i-- {
			stack = append(stack, stackItem{
				url:       newLinks[i],
				parentURL: result.URL,
				depth:     item.depth + 1,
			})
		}
	}

	stats.TotalTimeMs = time.Since(start).Milliseconds()
	return allResults, stats
}

// --- Best-First Strategy ---

type BestFirstStrategy struct{}

const bestFirstBatchSize = 10

type pqItem struct {
	url       string
	parentURL string
	score     float64
	depth     int
}

type priorityQueue []pqItem

func (pq priorityQueue) Len() int            { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool  { return pq[i].score > pq[j].score }
func (pq priorityQueue) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *priorityQueue) Push(x any)         { *pq = append(*pq, x.(pqItem)) }
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}

func (s *BestFirstStrategy) Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats) {
	start := time.Now()
	visited := make(map[string]bool)
	depths := make(map[string]int)

	baseU, _ := url.Parse(startURL)
	normalizedStart := NormalizeURL(startURL, baseU)

	visited[normalizedStart] = true
	depths[normalizedStart] = 0

	pq := &priorityQueue{}
	heap.Init(pq)

	initScore := 0.0
	if opts.Scorer != nil {
		initScore = opts.Scorer.Score(startURL)
	}
	heap.Push(pq, pqItem{url: startURL, parentURL: "", score: initScore, depth: 0})

	var allResults []DeepCrawlResult
	stats := CrawlStats{}

	for pq.Len() > 0 && stats.PagesCrawled < opts.MaxPages {
		if ctx.Err() != nil {
			break
		}

		// Dequeue batch
		var batch []pqItem
		for i := 0; i < bestFirstBatchSize && pq.Len() > 0; i++ {
			batch = append(batch, heap.Pop(pq).(pqItem))
		}

		results := make([]DeepCrawlResult, len(batch))
		var wg sync.WaitGroup

		for i, item := range batch {
			if stats.PagesCrawled >= opts.MaxPages {
				break
			}
			wg.Add(1)
			stats.PagesCrawled++

			go func(idx int, it pqItem) {
				defer wg.Done()
				result, err := crawlFn(ctx, it.url)
				if err != nil {
					return
				}
				result.Depth = it.depth
				result.ParentURL = it.parentURL
				result.Score = it.score
				results[idx] = *result
			}(i, item)
		}
		wg.Wait()

		for _, result := range results {
			if result.URL == "" {
				continue
			}
			if result.Blocked {
				stats.PagesBlocked++
			}
			if result.Depth > stats.MaxDepthReached {
				stats.MaxDepthReached = result.Depth
			}
			allResults = append(allResults, result)

			if result.Depth >= opts.MaxDepth {
				continue
			}

			newLinks := discoverLinks(ctx, result, visited, depths, result.Depth+1, opts)
			for _, link := range newLinks {
				linkScore := 0.0
				if opts.Scorer != nil {
					linkScore = opts.Scorer.Score(link)
				}
				if linkScore >= opts.ScoreThreshold || opts.Scorer == nil {
					heap.Push(pq, pqItem{
						url:       link,
						parentURL: result.URL,
						score:     linkScore,
						depth:     result.Depth + 1,
					})
				}
			}
		}
	}

	stats.TotalTimeMs = time.Since(start).Milliseconds()
	return allResults, stats
}

// --- Shared helpers ---

func discoverLinks(ctx context.Context, result DeepCrawlResult, visited map[string]bool, depths map[string]int, newDepth int, opts CrawlOptions) []string {
	var discovered []string

	links := result.Links.Internal
	if opts.IncludeExternal {
		links = append(links, result.Links.External...)
	}

	baseU, _ := url.Parse(result.URL)

	for _, link := range links {
		normalized := NormalizeURL(link.Href, baseU)
		if normalized == "" || visited[normalized] {
			continue
		}

		if opts.Filters != nil && !opts.Filters.Apply(normalized) {
			continue
		}

		if opts.Robots != nil && !opts.Robots.CanFetch(ctx, "crawl4go", normalized) {
			continue
		}

		if opts.Scorer != nil {
			score := opts.Scorer.Score(normalized)
			if score < opts.ScoreThreshold {
				continue
			}
		}

		visited[normalized] = true
		depths[normalized] = newDepth
		discovered = append(discovered, normalized)
	}

	slog.Debug("links discovered", "page", result.URL, "found", len(discovered))
	return discovered
}
