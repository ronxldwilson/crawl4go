package crawl

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

type BFSStrategy struct{}

func (s *BFSStrategy) Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats) {
	start := time.Now()
	visited := make(map[string]bool)
	depths := make(map[string]int)

	baseU, _ := url.Parse(startURL)
	normalizedStart := content.NormalizeURL(startURL, baseU)

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
