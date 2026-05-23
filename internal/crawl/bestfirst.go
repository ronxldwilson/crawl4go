package crawl

import (
	"container/heap"
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

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
	normalizedStart := content.NormalizeURL(startURL, baseU)

	pq := &priorityQueue{}
	heap.Init(pq)

	var allResults []DeepCrawlResult
	stats := CrawlStats{}

	if opts.InitialState != nil {
		for u, v := range opts.InitialState.Visited {
			visited[u] = v
		}
		for u, d := range opts.InitialState.Depths {
			depths[u] = d
		}
		for _, u := range opts.InitialState.Pending {
			score := 0.0
			if opts.InitialState.Scores != nil {
				score = opts.InitialState.Scores[u]
			}
			d := depths[u]
			heap.Push(pq, pqItem{url: u, parentURL: "", score: score, depth: d})
		}
	}

	if pq.Len() == 0 {
		visited[normalizedStart] = true
		depths[normalizedStart] = 0
		initScore := 0.0
		if opts.Scorer != nil {
			initScore = opts.Scorer.Score(startURL)
		}
		heap.Push(pq, pqItem{url: startURL, parentURL: "", score: initScore, depth: 0})
	}

	for pq.Len() > 0 && stats.PagesCrawled < opts.MaxPages {
		if ctx.Err() != nil {
			break
		}

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
