package crawl

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// MonitorConfig configures the CrawlerMonitor dashboard.
type MonitorConfig struct {
	RefreshInterval time.Duration
	Output          io.Writer
}

// domainStats tracks per-domain crawl statistics.
type domainStats struct {
	Crawled  int
	Errors   int
	Bytes    int64
	Duration time.Duration
}

// crawlRecord stores details of a single crawl event.
type crawlRecord struct {
	URL      string
	Status   int
	Bytes    int64
	Duration time.Duration
	Time     time.Time
}

// errorRecord stores a crawl error.
type errorRecord struct {
	URL  string
	Err  error
	Time time.Time
}

// CrawlerMonitor tracks crawl progress and renders a terminal dashboard.
type CrawlerMonitor struct {
	config MonitorConfig

	mu         sync.Mutex
	startTime  time.Time
	crawled    []crawlRecord
	errors     []errorRecord
	pending    map[string]bool
	domains    map[string]*domainStats
	totalBytes int64
}

// NewCrawlerMonitor creates a new CrawlerMonitor with the given config.
func NewCrawlerMonitor(cfg MonitorConfig) *CrawlerMonitor {
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = time.Second
	}
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	return &CrawlerMonitor{
		config:    cfg,
		startTime: time.Now(),
		pending:   make(map[string]bool),
		domains:   make(map[string]*domainStats),
	}
}

// AddPending marks a URL as pending crawl.
func (m *CrawlerMonitor) AddPending(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending[url] = true
}

// RecordCrawl records a completed crawl.
func (m *CrawlerMonitor) RecordCrawl(url string, status int, bytes int64, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.crawled = append(m.crawled, crawlRecord{
		URL:      url,
		Status:   status,
		Bytes:    bytes,
		Duration: duration,
		Time:     time.Now(),
	})
	m.totalBytes += bytes
	delete(m.pending, url)

	domain := extractDomain(url)
	ds, ok := m.domains[domain]
	if !ok {
		ds = &domainStats{}
		m.domains[domain] = ds
	}
	ds.Crawled++
	ds.Bytes += bytes
	ds.Duration += duration
}

// RecordError records a crawl error.
func (m *CrawlerMonitor) RecordError(url string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errors = append(m.errors, errorRecord{
		URL:  url,
		Err:  err,
		Time: time.Now(),
	})
	delete(m.pending, url)

	domain := extractDomain(url)
	ds, ok := m.domains[domain]
	if !ok {
		ds = &domainStats{}
		m.domains[domain] = ds
	}
	ds.Errors++
}

// Start launches a goroutine that periodically renders the dashboard.
func (m *CrawlerMonitor) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(m.config.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.render()
			}
		}
	}()
}

// Summary returns a formatted summary of the crawl session.
func (m *CrawlerMonitor) Summary() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	elapsed := time.Since(m.startTime).Round(time.Millisecond)

	var sb strings.Builder
	sb.WriteString("=== Crawl Summary ===\n")
	fmt.Fprintf(&sb, "Elapsed:     %s\n", elapsed)
	fmt.Fprintf(&sb, "Crawled:     %d URLs\n", len(m.crawled))
	fmt.Fprintf(&sb, "Pending:     %d URLs\n", len(m.pending))
	fmt.Fprintf(&sb, "Errors:      %d\n", len(m.errors))
	fmt.Fprintf(&sb, "Transferred: %s\n", formatBytes(m.totalBytes))

	if len(m.domains) > 0 {
		sb.WriteString("\n--- Per-Domain ---\n")

		// Sort domains for deterministic output.
		domainNames := make([]string, 0, len(m.domains))
		for d := range m.domains {
			domainNames = append(domainNames, d)
		}
		sort.Strings(domainNames)

		for _, d := range domainNames {
			ds := m.domains[d]
			fmt.Fprintf(&sb, "  %-30s  crawled=%d  errors=%d  bytes=%s\n",
				d, ds.Crawled, ds.Errors, formatBytes(ds.Bytes))
		}
	}

	if len(m.errors) > 0 {
		sb.WriteString("\n--- Recent Errors ---\n")
		start := 0
		if len(m.errors) > 5 {
			start = len(m.errors) - 5
		}
		for _, e := range m.errors[start:] {
			fmt.Fprintf(&sb, "  %s: %v\n", e.URL, e.Err)
		}
	}

	return sb.String()
}

// render writes the dashboard to the configured output using ANSI codes.
func (m *CrawlerMonitor) render() {
	m.mu.Lock()
	defer m.mu.Unlock()

	elapsed := time.Since(m.startTime).Round(time.Millisecond)
	w := m.config.Output

	// ANSI: move cursor to top-left and clear screen.
	fmt.Fprint(w, "\033[H\033[2J")

	fmt.Fprintf(w, "\033[1m=== Crawler Monitor ===\033[0m\n\n")
	fmt.Fprintf(w, "  Elapsed:     %s\n", elapsed)
	fmt.Fprintf(w, "  Crawled:     \033[32m%d\033[0m URLs\n", len(m.crawled))
	fmt.Fprintf(w, "  Pending:     \033[33m%d\033[0m URLs\n", len(m.pending))
	fmt.Fprintf(w, "  Errors:      \033[31m%d\033[0m\n", len(m.errors))
	fmt.Fprintf(w, "  Transferred: %s\n", formatBytes(m.totalBytes))

	if len(m.crawled) > 0 {
		rate := float64(len(m.crawled)) / elapsed.Seconds()
		fmt.Fprintf(w, "  Rate:        %.1f URLs/s\n", rate)
	}

	// Show last 5 crawled URLs.
	if len(m.crawled) > 0 {
		fmt.Fprintf(w, "\n\033[1mRecent:\033[0m\n")
		start := 0
		if len(m.crawled) > 5 {
			start = len(m.crawled) - 5
		}
		for _, r := range m.crawled[start:] {
			statusColor := "\033[32m" // green
			if r.Status >= 400 {
				statusColor = "\033[31m" // red
			} else if r.Status >= 300 {
				statusColor = "\033[33m" // yellow
			}
			fmt.Fprintf(w, "  %s%d\033[0m  %6dB  %s  %s\n",
				statusColor, r.Status, r.Bytes, r.Duration.Round(time.Millisecond), truncateURL(r.URL, 60))
		}
	}

	// Show recent errors.
	if len(m.errors) > 0 {
		fmt.Fprintf(w, "\n\033[1;31mErrors:\033[0m\n")
		start := 0
		if len(m.errors) > 3 {
			start = len(m.errors) - 3
		}
		for _, e := range m.errors[start:] {
			fmt.Fprintf(w, "  \033[31m%s\033[0m: %v\n", truncateURL(e.URL, 50), e.Err)
		}
	}
}

// extractDomain extracts a domain from a URL string.
func extractDomain(rawURL string) string {
	// Simple extraction without importing net/url to keep dependencies minimal.
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.IndexByte(s, '/'); idx >= 0 {
		s = s[:idx]
	}
	return s
}

// truncateURL truncates a URL to maxLen characters.
func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return url[:maxLen-3] + "..."
}

// formatBytes returns a human-readable byte size.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
