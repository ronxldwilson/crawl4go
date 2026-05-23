package crawl

import (
	"context"
	"log/slog"
	"net/url"

	"github.com/ronxldwilson/crawl4go/internal/content"
)

func discoverLinks(ctx context.Context, result DeepCrawlResult, visited map[string]bool, depths map[string]int, newDepth int, opts CrawlOptions) []string {
	var discovered []string

	links := result.Links.Internal
	if opts.IncludeExternal {
		links = append(links, result.Links.External...)
	}

	baseU, _ := url.Parse(result.URL)

	for _, link := range links {
		normalized := content.NormalizeURL(link.Href, baseU)
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
