package crawl

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryDispatcherConfig configures the memory-adaptive dispatcher.
type MemoryDispatcherConfig struct {
	MaxConcurrent    int           // max parallel tasks
	MemoryThreshold  float64       // 0-1, fraction of system memory that triggers throttling
	CheckInterval    time.Duration // how often to check memory
	PriorityLevels   int           // number of priority buckets (default 3)
	AntiStarvationMs int           // max time a low-priority task waits before promotion
}

// DefaultMemoryDispatcherConfig returns sensible defaults.
func DefaultMemoryDispatcherConfig() MemoryDispatcherConfig {
	return MemoryDispatcherConfig{
		MaxConcurrent:    10,
		MemoryThreshold:  0.85,
		CheckInterval:    5 * time.Second,
		PriorityLevels:   3,
		AntiStarvationMs: 30000,
	}
}

// MemoryDispatcher runs crawl tasks with memory-pressure awareness and
// priority-based scheduling with anti-starvation.
type MemoryDispatcher struct {
	cfg     MemoryDispatcherConfig
	crawlFn CrawlFunc
	stats   *DispatchStats

	sem      chan struct{}
	mu       sync.Mutex
	queues   [][]CrawlerTask // priority queues, index 0 = highest
	enqueued atomic.Int64
}

// NewMemoryDispatcher creates a dispatcher.
func NewMemoryDispatcher(cfg MemoryDispatcherConfig, crawlFn CrawlFunc) *MemoryDispatcher {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 10
	}
	if cfg.PriorityLevels <= 0 {
		cfg.PriorityLevels = 3
	}
	return &MemoryDispatcher{
		cfg:     cfg,
		crawlFn: crawlFn,
		stats:   NewDispatchStats(),
		sem:     make(chan struct{}, cfg.MaxConcurrent),
		queues:  make([][]CrawlerTask, cfg.PriorityLevels),
	}
}

// Submit adds a task to the appropriate priority queue.
func (d *MemoryDispatcher) Submit(task CrawlerTask) {
	level := int(task.Priority * float64(d.cfg.PriorityLevels-1))
	if level < 0 {
		level = 0
	}
	if level >= d.cfg.PriorityLevels {
		level = d.cfg.PriorityLevels - 1
	}
	// Invert: higher priority value = lower queue index
	queueIdx := d.cfg.PriorityLevels - 1 - level

	d.mu.Lock()
	d.queues[queueIdx] = append(d.queues[queueIdx], task)
	d.mu.Unlock()
	d.enqueued.Add(1)
}

// Run processes all queued tasks, respecting memory pressure and priority.
// Blocks until all tasks complete or ctx is cancelled.
func (d *MemoryDispatcher) Run(ctx context.Context) *DispatchStats {
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return d.stats
		default:
		}

		// Check memory pressure
		if d.isMemoryHigh() {
			slog.Debug("memory pressure high, throttling", "threshold", d.cfg.MemoryThreshold)
			time.Sleep(d.cfg.CheckInterval)
			// Promote starved low-priority tasks
			d.promoteStarved()
			continue
		}

		task, ok := d.dequeue()
		if !ok {
			// No more tasks
			break
		}

		// Acquire semaphore
		select {
		case d.sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return d.stats
		}

		wg.Add(1)
		go func(t CrawlerTask) {
			defer wg.Done()
			defer func() { <-d.sem }()

			start := time.Now()
			result, err := d.crawlFn(ctx, t.URL)
			duration := time.Since(start)

			tr := CrawlerTaskResult{
				Task:     t,
				Result:   result,
				Error:    err,
				Duration: duration,
			}
			d.stats.RecordResult(tr)
		}(task)
	}

	wg.Wait()
	return d.stats
}

// Stats returns the current dispatch stats.
func (d *MemoryDispatcher) Stats() *DispatchStats {
	return d.stats
}

func (d *MemoryDispatcher) dequeue() (CrawlerTask, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Highest priority first
	for i := range d.queues {
		if len(d.queues[i]) > 0 {
			task := d.queues[i][0]
			d.queues[i] = d.queues[i][1:]
			return task, true
		}
	}
	return CrawlerTask{}, false
}

func (d *MemoryDispatcher) promoteStarved() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	threshold := time.Duration(d.cfg.AntiStarvationMs) * time.Millisecond

	// Move starved tasks from lower priority to higher
	for i := len(d.queues) - 1; i > 0; i-- {
		var remaining []CrawlerTask
		for _, t := range d.queues[i] {
			if now.Sub(t.CreatedAt) > threshold {
				d.queues[i-1] = append(d.queues[i-1], t)
			} else {
				remaining = append(remaining, t)
			}
		}
		d.queues[i] = remaining
	}
}

func (d *MemoryDispatcher) isMemoryHigh() bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// Use HeapInuse as a proxy for Go memory usage
	// Compare against a reasonable threshold (e.g., 1GB default)
	heapMB := m.HeapInuse / (1024 * 1024)
	// Simple heuristic: if heap > 500MB, consider high
	return heapMB > 500 && d.cfg.MemoryThreshold < 1.0
}
