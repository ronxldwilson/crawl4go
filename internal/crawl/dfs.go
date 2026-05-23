package crawl

import (
	"context"
	"net/url"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

type DFSStrategy struct{}

func (s *DFSStrategy) Run(ctx context.Context, startURL string, crawlFn CrawlFunc, opts CrawlOptions) ([]DeepCrawlResult, CrawlStats) {
	start := time.Now()
	visited := make(map[string]bool)
	depths := make(map[string]int)

	baseU, _ := url.Parse(startURL)
	normalizedStart := content.NormalizeURL(startURL, baseU)

	type stackItem struct {
		url       string
		parentURL string
		depth     int
	}

	var stack []stackItem
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
			d := depths[u]
			stack = append(stack, stackItem{url: u, parentURL: "", depth: d})
		}
	}

	if len(stack) == 0 {
		visited[normalizedStart] = true
		depths[normalizedStart] = 0
		stack = []stackItem{{url: startURL, parentURL: "", depth: 0}}
	}

	for len(stack) > 0 && stats.PagesCrawled < opts.MaxPages {
		if ctx.Err() != nil {
			break
		}

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
