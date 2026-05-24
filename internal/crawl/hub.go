package crawl

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Crawler is the interface that all pluggable crawlers must implement.
type Crawler interface {
	Crawl(ctx context.Context, url string) (*CrawlResult, error)
}

// CrawlerFactory is a constructor function that creates a Crawler from a
// configuration map. Each registered factory is invoked on demand by
// CrawlerHub.GetCrawler.
type CrawlerFactory func(config map[string]any) (Crawler, error)

// CrawlerHub is a plugin-style dynamic crawler registry. It is safe for
// concurrent use.
type CrawlerHub struct {
	mu        sync.RWMutex
	factories map[string]CrawlerFactory
}

// DefaultHub returns a new CrawlerHub with no pre-registered crawlers.
func DefaultHub() *CrawlerHub {
	return &CrawlerHub{
		factories: make(map[string]CrawlerFactory),
	}
}

// RegisterCrawler registers a named crawler factory. If a factory with the
// same name already exists it will be overwritten.
func (h *CrawlerHub) RegisterCrawler(name string, factory CrawlerFactory) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.factories[name] = factory
}

// GetCrawler instantiates a crawler by name using the registered factory.
// A nil config is replaced with an empty map before being passed to the
// factory. Returns an error if the name is not registered.
func (h *CrawlerHub) GetCrawler(name string, config map[string]any) (Crawler, error) {
	h.mu.RLock()
	factory, ok := h.factories[name]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("crawl: unknown crawler %q", name)
	}
	if config == nil {
		config = make(map[string]any)
	}
	return factory(config)
}

// ListCrawlers returns the names of all registered crawlers in sorted order.
func (h *CrawlerHub) ListCrawlers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	names := make([]string, 0, len(h.factories))
	for name := range h.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
