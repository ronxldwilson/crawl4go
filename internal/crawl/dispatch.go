package crawl

import (
	"sync"
	"time"
)

// CrawlStatus represents the lifecycle state of a crawl task.
type CrawlStatus int

const (
	StatusPending   CrawlStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusCancelled
	StatusRetrying
)

func (s CrawlStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	case StatusRetrying:
		return "retrying"
	default:
		return "unknown"
	}
}

// DomainState tracks crawl state for a single domain across multiple URLs.
type DomainState struct {
	Domain           string      `json:"domain"`
	Status           CrawlStatus `json:"status"`
	URLsTotal        int         `json:"urls_total"`
	URLsCrawled      int         `json:"urls_crawled"`
	URLsFailed       int         `json:"urls_failed"`
	URLsBlocked      int         `json:"urls_blocked"`
	AvgLatencyMs     int64       `json:"avg_latency_ms"`
	LastCrawled      time.Time   `json:"last_crawled"`
	ConsecutiveFails int         `json:"consecutive_fails"`
}

// CrawlerTask represents a single URL crawl task in the dispatch queue.
type CrawlerTask struct {
	URL       string      `json:"url"`
	Domain    string      `json:"domain"`
	Depth     int         `json:"depth"`
	Priority  float64     `json:"priority"`
	Status    CrawlStatus `json:"status"`
	ParentURL string      `json:"parent_url,omitempty"`
	Attempts  int         `json:"attempts"`
	CreatedAt time.Time   `json:"created_at"`
}

// CrawlerTaskResult pairs a task with its outcome.
type CrawlerTaskResult struct {
	Task     CrawlerTask      `json:"task"`
	Result   *DeepCrawlResult `json:"result,omitempty"`
	Error    error            `json:"-"`
	Duration time.Duration    `json:"duration"`
}

// DisplayMode controls how crawl progress is reported.
type DisplayMode int

const (
	DisplayQuiet    DisplayMode = iota // no output
	DisplaySummary                     // final summary only
	DisplayProgress                    // periodic progress updates
	DisplayVerbose                     // per-URL logging
)

// DispatchStats aggregates stats across all domains in a batch crawl.
type DispatchStats struct {
	mu             sync.Mutex
	Domains        map[string]*DomainState `json:"domains"`
	TotalTasks     int                     `json:"total_tasks"`
	CompletedTasks int                     `json:"completed_tasks"`
	FailedTasks    int                     `json:"failed_tasks"`
	StartTime      time.Time              `json:"start_time"`
}

// NewDispatchStats creates an empty stats tracker.
func NewDispatchStats() *DispatchStats {
	return &DispatchStats{
		Domains:   make(map[string]*DomainState),
		StartTime: time.Now(),
	}
}

// RecordResult updates stats for a completed task.
func (s *DispatchStats) RecordResult(tr CrawlerTaskResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	domain := tr.Task.Domain
	ds, ok := s.Domains[domain]
	if !ok {
		ds = &DomainState{Domain: domain}
		s.Domains[domain] = ds
	}

	if tr.Error != nil {
		s.FailedTasks++
		ds.URLsFailed++
		ds.ConsecutiveFails++
	} else {
		s.CompletedTasks++
		ds.URLsCrawled++
		ds.ConsecutiveFails = 0
		ds.LastCrawled = time.Now()
		if tr.Result != nil && tr.Result.Blocked {
			ds.URLsBlocked++
		}
		if tr.Duration > 0 {
			latMs := tr.Duration.Milliseconds()
			if ds.AvgLatencyMs == 0 {
				ds.AvgLatencyMs = latMs
			} else {
				ds.AvgLatencyMs = (ds.AvgLatencyMs*3 + latMs) / 4
			}
		}
	}
}

// Elapsed returns time since the dispatch started.
func (s *DispatchStats) Elapsed() time.Duration {
	return time.Since(s.StartTime)
}
