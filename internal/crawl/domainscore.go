package crawl

import (
	"math"
	"sort"
	"sync"
	"time"
)

// DomainAuthority tracks crawl quality statistics for a domain.
type DomainAuthority struct {
	Domain     string    `json:"domain"`
	Score      float64   `json:"score"`
	CrawlCount int      `json:"crawl_count"`
	AvgQuality float64   `json:"avg_quality"`
	LastSeen   time.Time `json:"last_seen"`
}

// DomainAuthorityScorer maintains running authority scores for domains
// based on observed crawl quality.
type DomainAuthorityScorer struct {
	mu          sync.RWMutex
	authorities map[string]*DomainAuthority
}

// NewDomainAuthorityScorer creates a new scorer with an empty authority map.
func NewDomainAuthorityScorer() *DomainAuthorityScorer {
	return &DomainAuthorityScorer{
		authorities: make(map[string]*DomainAuthority),
	}
}

// RecordCrawl updates the running average quality and score for a domain.
// Quality values should typically be in [0, 1].
func (das *DomainAuthorityScorer) RecordCrawl(domain string, quality float64) {
	das.mu.Lock()
	defer das.mu.Unlock()

	auth, ok := das.authorities[domain]
	if !ok {
		auth = &DomainAuthority{Domain: domain}
		das.authorities[domain] = auth
	}

	// Running average: new_avg = old_avg + (quality - old_avg) / new_count
	auth.CrawlCount++
	auth.AvgQuality += (quality - auth.AvgQuality) / float64(auth.CrawlCount)
	auth.LastSeen = time.Now()

	// Score blends average quality with a log-based crawl count bonus
	// so frequently-crawled high-quality domains rank higher.
	auth.Score = auth.AvgQuality * (1 + 0.1*math.Log2(float64(auth.CrawlCount)+1))
}

// GetAuthority returns the current authority score for a domain.
// Returns 0 if the domain has not been seen.
func (das *DomainAuthorityScorer) GetAuthority(domain string) float64 {
	das.mu.RLock()
	defer das.mu.RUnlock()

	if auth, ok := das.authorities[domain]; ok {
		return auth.Score
	}
	return 0
}

// TopDomains returns the top n domains sorted by score descending.
func (das *DomainAuthorityScorer) TopDomains(n int) []DomainAuthority {
	das.mu.RLock()
	defer das.mu.RUnlock()

	all := make([]DomainAuthority, 0, len(das.authorities))
	for _, auth := range das.authorities {
		all = append(all, *auth)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Score > all[j].Score
	})

	if n > len(all) {
		n = len(all)
	}
	return all[:n]
}
