package crawl

import "context"

// FallbackFunc is a last-resort fetch function called when the primary crawl
// fails after all retries. It receives the URL and last error, and returns
// raw content or an error.
type FallbackFunc func(ctx context.Context, url string, lastErr error) (string, error)

// CrawlWithFallback tries the primary crawl with retries, then falls back
// to fallbackFn if all attempts fail.
func CrawlWithFallback(ctx context.Context, url string, crawlFn CrawlFunc, cfg RetryConfig, fallbackFn FallbackFunc) *RetryResult {
	result := CrawlWithRetry(ctx, url, crawlFn, cfg, nil)
	if result.Result != nil && !result.Result.Blocked {
		return result
	}

	if fallbackFn == nil {
		return result
	}

	content, err := fallbackFn(ctx, url, result.LastErr)
	if err != nil {
		return result // original result is still better context
	}

	return &RetryResult{
		Result: &DeepCrawlResult{
			URL:     url,
			Content: content,
		},
		Attempts: result.Attempts + 1,
	}
}
