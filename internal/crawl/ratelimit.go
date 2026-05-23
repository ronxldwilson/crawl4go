package crawl

import (
	"context"
	"math/rand"
	"net/url"
	"sync"
	"time"
)

type domainState struct {
	currentDelay time.Duration
	lastRequest  time.Time
	failCount    int
}

// RateLimiter implements a per-domain adaptive rate limiter with exponential
// backoff on failure and gradual recovery on success.
type RateLimiter struct {
	mu            sync.Mutex
	domains       map[string]*domainState
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	MaxRetries    int
	BackoffFactor float64
}

// NewRateLimiter returns a RateLimiter with sensible defaults.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		domains:       make(map[string]*domainState),
		BaseDelay:     100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		MaxRetries:    5,
		BackoffFactor: 2.0,
	}
}

// extractDomain parses rawURL and returns its host, falling back to rawURL on error.
func (rl *RateLimiter) extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

// getOrCreate returns the domainState for the given domain, creating one if absent.
// Caller must hold rl.mu.
func (rl *RateLimiter) getOrCreate(domain string) *domainState {
	ds, ok := rl.domains[domain]
	if !ok {
		ds = &domainState{
			currentDelay: rl.BaseDelay,
		}
		rl.domains[domain] = ds
	}
	return ds
}

// Wait blocks until it is safe to make a request to rawURL's domain, respecting
// ctx cancellation. It updates the domain's lastRequest timestamp after the wait.
func (rl *RateLimiter) Wait(ctx context.Context, rawURL string) error {
	domain := rl.extractDomain(rawURL)

	rl.mu.Lock()
	ds := rl.getOrCreate(domain)
	delay := ds.currentDelay
	elapsed := time.Since(ds.lastRequest)
	remaining := delay - elapsed
	rl.mu.Unlock()

	if remaining > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(remaining):
		}
	}

	rl.mu.Lock()
	ds.lastRequest = time.Now()
	rl.mu.Unlock()

	return nil
}

// RecordResult updates the domain's rate-limit state based on the HTTP status code.
// On 429 or 503, the delay is backed off. On 2xx, the delay is reduced toward BaseDelay.
func (rl *RateLimiter) RecordResult(rawURL string, statusCode int) {
	domain := rl.extractDomain(rawURL)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	ds := rl.getOrCreate(domain)

	switch {
	case statusCode == 429 || statusCode == 503:
		// Exponential backoff with ±25% jitter.
		backed := float64(ds.currentDelay) * rl.BackoffFactor
		jitter := backed * 0.25 * (rand.Float64()*2 - 1) // [-0.25*backed, +0.25*backed]
		newDelay := time.Duration(backed + jitter)
		if newDelay > rl.MaxDelay {
			newDelay = rl.MaxDelay
		}
		ds.currentDelay = newDelay
		ds.failCount++

	case statusCode >= 200 && statusCode < 300:
		// Gradual recovery.
		newDelay := time.Duration(float64(ds.currentDelay) * 0.75)
		if newDelay < rl.BaseDelay {
			newDelay = rl.BaseDelay
		}
		ds.currentDelay = newDelay
		ds.failCount = 0
	}
}

// ShouldRetry reports whether the domain associated with rawURL has not yet
// exceeded MaxRetries consecutive failures.
func (rl *RateLimiter) ShouldRetry(rawURL string) bool {
	domain := rl.extractDomain(rawURL)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	ds, ok := rl.domains[domain]
	if !ok {
		return true
	}
	return ds.failCount <= rl.MaxRetries
}

// Reset removes all stored state for the domain associated with rawURL.
func (rl *RateLimiter) Reset(rawURL string) {
	domain := rl.extractDomain(rawURL)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.domains, domain)
}
