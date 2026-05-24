package crawl

import (
	"context"
	"sync"
	"sync/atomic"
)

// CancelFunc is an external callback that returns true if the crawl should stop.
// It's polled between page crawls.
type CancelFunc func() bool

// CrawlController manages cancellation and progress for a crawl operation.
// It wraps a context.Context with additional crawl-specific cancellation signals.
type CrawlController struct {
	ctx       context.Context
	cancel    context.CancelFunc
	reason    atomic.Value // stores string
	cancelled atomic.Bool

	mu           sync.Mutex
	shouldCancel []CancelFunc
	onCancel     []func(reason string)
}

// NewCrawlController creates a controller from a parent context.
func NewCrawlController(parent context.Context) *CrawlController {
	ctx, cancel := context.WithCancel(parent)
	cc := &CrawlController{
		ctx:    ctx,
		cancel: cancel,
	}
	return cc
}

// Context returns the underlying context for passing to crawl functions.
func (cc *CrawlController) Context() context.Context {
	return cc.ctx
}

// Cancel stops the crawl with a reason.
func (cc *CrawlController) Cancel(reason string) {
	if cc.cancelled.CompareAndSwap(false, true) {
		cc.reason.Store(reason)
		cc.cancel()
		cc.mu.Lock()
		callbacks := make([]func(string), len(cc.onCancel))
		copy(callbacks, cc.onCancel)
		cc.mu.Unlock()
		for _, fn := range callbacks {
			fn(reason)
		}
	}
}

// IsCancelled returns whether the crawl has been cancelled.
func (cc *CrawlController) IsCancelled() bool {
	return cc.cancelled.Load()
}

// Reason returns the cancellation reason, or empty string if not cancelled.
func (cc *CrawlController) Reason() string {
	if v := cc.reason.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// OnShouldCancel registers an external callback that's polled to check if
// the crawl should stop. Use this to wire up external signals (UI buttons,
// resource limits, etc.).
func (cc *CrawlController) OnShouldCancel(fn CancelFunc) {
	cc.mu.Lock()
	cc.shouldCancel = append(cc.shouldCancel, fn)
	cc.mu.Unlock()
}

// OnCancel registers a callback that fires when the crawl is cancelled.
func (cc *CrawlController) OnCancel(fn func(reason string)) {
	cc.mu.Lock()
	cc.onCancel = append(cc.onCancel, fn)
	cc.mu.Unlock()
}

// CheckShouldCancel polls all registered should_cancel callbacks.
// Call this between page crawls. Returns true if any callback wants to stop.
func (cc *CrawlController) CheckShouldCancel() bool {
	if cc.IsCancelled() {
		return true
	}
	cc.mu.Lock()
	checks := make([]CancelFunc, len(cc.shouldCancel))
	copy(checks, cc.shouldCancel)
	cc.mu.Unlock()
	for _, fn := range checks {
		if fn() {
			cc.Cancel("external cancellation")
			return true
		}
	}
	return false
}
