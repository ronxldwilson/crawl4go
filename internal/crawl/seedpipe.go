package crawl

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// SeedingConfig controls URL discovery and seeding behavior.
type SeedingConfig struct {
	Sources       []string      `json:"sources"`         // sitemap, cdx, manual
	MaxURLs       int           `json:"max_urls"`
	Domains       []string      `json:"domains"`
	RatePerDomain int           `json:"rate_per_domain"` // max URLs/sec per domain
	BufferSize    int           `json:"buffer_size"`
	Timeout       time.Duration `json:"timeout"`
}

// DefaultSeedingConfig returns sensible defaults.
func DefaultSeedingConfig() SeedingConfig {
	return SeedingConfig{
		Sources:       []string{"sitemap"},
		MaxURLs:       10000,
		RatePerDomain: 5,
		BufferSize:    1000,
		Timeout:       2 * time.Minute,
	}
}

// SeedResult holds a discovered URL with metadata.
type SeedResult struct {
	URL      string  `json:"url"`
	Domain   string  `json:"domain"`
	Source   string  `json:"source"` // sitemap, cdx, manual
	Priority float64 `json:"priority,omitempty"`
}

// SeedPipeline is a bounded producer/worker pipeline with backpressure
// for URL seeding. Producers discover URLs and push them into a buffered
// channel; consumers pull URLs at a rate-limited pace.
type SeedPipeline struct {
	cfg    SeedingConfig
	output chan SeedResult
	done   chan struct{}

	mu       sync.Mutex
	seen     map[string]bool
	produced atomic.Int64
}

// NewSeedPipeline creates a pipeline with the given config.
func NewSeedPipeline(cfg SeedingConfig) *SeedPipeline {
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = 1000
	}
	return &SeedPipeline{
		cfg:    cfg,
		output: make(chan SeedResult, bufSize),
		done:   make(chan struct{}),
		seen:   make(map[string]bool),
	}
}

// Results returns the channel consumers read from.
func (sp *SeedPipeline) Results() <-chan SeedResult {
	return sp.output
}

// FeedManual adds URLs from a manual list. Safe for concurrent use.
func (sp *SeedPipeline) FeedManual(ctx context.Context, urls []string) {
	for _, u := range urls {
		if sp.cfg.MaxURLs > 0 && int(sp.produced.Load()) >= sp.cfg.MaxURLs {
			return
		}
		sp.emit(ctx, u, "manual", 0)
	}
}

// FeedDomains starts parallel seeding across multiple domains using the
// configured sources (fan-out). Each domain is seeded in its own goroutine
// with per-domain rate limiting.
func (sp *SeedPipeline) FeedDomains(ctx context.Context, domains []string) {
	var wg sync.WaitGroup
	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			sp.seedDomain(ctx, d)
		}(domain)
	}
	go func() {
		wg.Wait()
		close(sp.output)
		close(sp.done)
	}()
}

// Wait blocks until seeding completes.
func (sp *SeedPipeline) Wait() {
	<-sp.done
}

func (sp *SeedPipeline) seedDomain(ctx context.Context, domain string) {
	// Per-domain rate limiter
	interval := time.Second
	if sp.cfg.RatePerDomain > 0 {
		interval = time.Second / time.Duration(sp.cfg.RatePerDomain)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Discover URLs from configured sources
	baseURL := "https://" + domain

	for _, source := range sp.cfg.Sources {
		if ctx.Err() != nil {
			return
		}

		switch source {
		case "sitemap":
			seeder := NewSitemapSeeder(nil, sp.cfg.MaxURLs)
			seeds, err := seeder.Discover(ctx, baseURL)
			if err != nil {
				continue
			}
			for _, s := range seeds {
				if sp.cfg.MaxURLs > 0 && int(sp.produced.Load()) >= sp.cfg.MaxURLs {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
				sp.emit(ctx, s.URL, "sitemap", s.Priority)
			}

		case "cdx":
			seeder := NewCDXSeeder(nil, sp.cfg.MaxURLs)
			records, err := seeder.Discover(ctx, domain)
			if err != nil {
				continue
			}
			for _, r := range records {
				if sp.cfg.MaxURLs > 0 && int(sp.produced.Load()) >= sp.cfg.MaxURLs {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
				sp.emit(ctx, r.URL, "cdx", 0)
			}

		case "manual":
			// manual URLs are fed via FeedManual
		}
	}
}

func (sp *SeedPipeline) emit(ctx context.Context, rawURL, source string, priority float64) {
	sp.mu.Lock()
	if sp.seen[rawURL] {
		sp.mu.Unlock()
		return
	}
	sp.seen[rawURL] = true
	sp.mu.Unlock()

	domain := ""
	if u, err := url.Parse(rawURL); err == nil {
		domain = u.Hostname()
	}

	select {
	case sp.output <- SeedResult{URL: rawURL, Domain: domain, Source: source, Priority: priority}:
		sp.produced.Add(1)
	case <-ctx.Done():
	}
}
