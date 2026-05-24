package crawl

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

// RetryConfig controls retry behavior for crawl operations.
type RetryConfig struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	BackoffFactor  float64       `json:"backoff_factor"`
	JitterFraction float64       `json:"jitter_fraction"` // 0-1, fraction of backoff to add as jitter
	RotateProxy    bool          `json:"rotate_proxy"`
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		BackoffFactor:  2.0,
		JitterFraction: 0.25,
	}
}

// RetryResult wraps a crawl result with retry metadata.
type RetryResult struct {
	Result   *DeepCrawlResult
	Attempts int
	LastErr  error
}

// CrawlWithRetry wraps a CrawlFunc with exponential-backoff retry logic.
// On retryable failures it backs off exponentially with jitter.
// If rotateProxy is true and proxyFn is provided, it calls proxyFn before
// each retry to get a fresh proxy URL (the caller is responsible for wiring
// the proxy into the CrawlFunc).
func CrawlWithRetry(ctx context.Context, url string, crawlFn CrawlFunc, cfg RetryConfig, onRetry func(attempt int, err error)) *RetryResult {
	var lastErr error
	backoff := cfg.InitialBackoff

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// Apply jitter
			jitter := time.Duration(float64(backoff) * cfg.JitterFraction * rand.Float64())
			wait := backoff + jitter

			slog.Debug("retry crawl", "url", url, "attempt", attempt, "backoff", wait)

			if onRetry != nil {
				onRetry(attempt, lastErr)
			}

			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return &RetryResult{Attempts: attempt, LastErr: ctx.Err()}
			}

			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * cfg.BackoffFactor)
			if backoff > cfg.MaxBackoff {
				backoff = cfg.MaxBackoff
			}
		}

		result, err := crawlFn(ctx, url)
		if err == nil && result != nil && !result.Blocked {
			return &RetryResult{Result: result, Attempts: attempt + 1}
		}

		if err != nil {
			lastErr = err
			if !IsRetryable(err) {
				return &RetryResult{Attempts: attempt + 1, LastErr: err}
			}
		} else if result != nil && result.Blocked {
			lastErr = NewBlockedError(url, "anti-bot detection triggered")
		}
	}

	return &RetryResult{Attempts: cfg.MaxRetries + 1, LastErr: lastErr}
}

// BackoffDuration calculates the backoff duration for a given attempt.
func BackoffDuration(attempt int, initial, max time.Duration, factor, jitterFraction float64) time.Duration {
	backoff := float64(initial) * math.Pow(factor, float64(attempt))
	if time.Duration(backoff) > max {
		backoff = float64(max)
	}
	jitter := backoff * jitterFraction * rand.Float64()
	return time.Duration(backoff + jitter)
}
