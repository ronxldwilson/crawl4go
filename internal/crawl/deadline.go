package crawl

import (
	"context"
	"time"
)

// DeadlineConfig controls how timeouts cascade through a deep crawl.
type DeadlineConfig struct {
	// Total is the overall deadline for the entire deep crawl.
	Total time.Duration
	// PerPage is the deadline for each individual page crawl.
	PerPage time.Duration
	// PerLevel is the deadline for each BFS level (0 = no limit).
	PerLevel time.Duration
}

// DefaultDeadlineConfig returns sensible defaults.
func DefaultDeadlineConfig() DeadlineConfig {
	return DeadlineConfig{
		Total:   5 * time.Minute,
		PerPage: 30 * time.Second,
	}
}

// WithTotalDeadline returns a context with the Total deadline applied.
// If the parent context already has a shorter deadline, it's preserved.
func (d DeadlineConfig) WithTotalDeadline(parent context.Context) (context.Context, context.CancelFunc) {
	if d.Total <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, d.Total)
}

// WithPageDeadline returns a child context with the PerPage deadline.
func (d DeadlineConfig) WithPageDeadline(parent context.Context) (context.Context, context.CancelFunc) {
	if d.PerPage <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, d.PerPage)
}

// WithLevelDeadline returns a child context with the PerLevel deadline.
func (d DeadlineConfig) WithLevelDeadline(parent context.Context) (context.Context, context.CancelFunc) {
	if d.PerLevel <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, d.PerLevel)
}

// WrapCrawlFunc wraps a CrawlFunc to enforce per-page deadlines.
func (d DeadlineConfig) WrapCrawlFunc(fn CrawlFunc) CrawlFunc {
	if d.PerPage <= 0 {
		return fn
	}
	return func(ctx context.Context, url string) (*DeepCrawlResult, error) {
		pageCtx, cancel := context.WithTimeout(ctx, d.PerPage)
		defer cancel()
		return fn(pageCtx, url)
	}
}

// RemainingBudget returns how much time is left on the context, or -1 if no deadline.
func RemainingBudget(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return -1
	}
	remaining := time.Until(deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}
