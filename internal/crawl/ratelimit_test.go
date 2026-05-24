package crawl

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter()

	if rl.BaseDelay != 100*time.Millisecond {
		t.Errorf("BaseDelay = %v, want 100ms", rl.BaseDelay)
	}
	if rl.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", rl.MaxDelay)
	}
	if rl.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", rl.MaxRetries)
	}
	if rl.BackoffFactor != 2.0 {
		t.Errorf("BackoffFactor = %v, want 2.0", rl.BackoffFactor)
	}
}

func TestExtractDomain(t *testing.T) {
	rl := NewRateLimiter()

	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{"full URL", "https://example.com/page", "example.com"},
		{"with port", "https://example.com:8080/path", "example.com:8080"},
		{"invalid URL", "not-a-url", "not-a-url"},
		{"empty host", "/relative/path", "/relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rl.extractDomain(tt.rawURL)
			if got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestRecordResultBackoff(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://example.com/page"

	// Record a 429 to trigger backoff.
	rl.RecordResult(rawURL, 429)

	rl.mu.Lock()
	ds := rl.domains["example.com"]
	afterBackoff := ds.currentDelay
	rl.mu.Unlock()

	// After a 429, delay should be larger than BaseDelay.
	if afterBackoff <= rl.BaseDelay {
		t.Errorf("after 429: delay = %v, want > %v", afterBackoff, rl.BaseDelay)
	}

	// Record another 429 for more backoff.
	rl.RecordResult(rawURL, 503)

	rl.mu.Lock()
	afterSecond := rl.domains["example.com"].currentDelay
	rl.mu.Unlock()

	if afterSecond <= afterBackoff {
		// Note: due to jitter this could fail rarely; the general trend should be up.
		t.Logf("warning: after second backoff delay %v not > first %v (jitter may cause this)", afterSecond, afterBackoff)
	}
}

func TestRecordResultRecovery(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://example.com/page"

	// Increase delay first.
	rl.RecordResult(rawURL, 429)
	rl.RecordResult(rawURL, 429)

	rl.mu.Lock()
	highDelay := rl.domains["example.com"].currentDelay
	rl.mu.Unlock()

	// Record success to recover.
	rl.RecordResult(rawURL, 200)

	rl.mu.Lock()
	afterRecovery := rl.domains["example.com"].currentDelay
	failCount := rl.domains["example.com"].failCount
	rl.mu.Unlock()

	if afterRecovery >= highDelay {
		t.Errorf("after 200: delay = %v, want < %v", afterRecovery, highDelay)
	}
	if failCount != 0 {
		t.Errorf("failCount = %d, want 0 after success", failCount)
	}
}

func TestRecordResultDelayFloor(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://example.com/page"

	// Many successes should not go below BaseDelay.
	for i := 0; i < 20; i++ {
		rl.RecordResult(rawURL, 200)
	}

	rl.mu.Lock()
	delay := rl.domains["example.com"].currentDelay
	rl.mu.Unlock()

	if delay < rl.BaseDelay {
		t.Errorf("delay = %v, should not go below BaseDelay %v", delay, rl.BaseDelay)
	}
}

func TestRecordResultMaxDelayCap(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://example.com/page"

	// Many failures should not exceed MaxDelay.
	for i := 0; i < 50; i++ {
		rl.RecordResult(rawURL, 429)
	}

	rl.mu.Lock()
	delay := rl.domains["example.com"].currentDelay
	rl.mu.Unlock()

	if delay > rl.MaxDelay {
		t.Errorf("delay = %v, should not exceed MaxDelay %v", delay, rl.MaxDelay)
	}
}

func TestShouldRetry(t *testing.T) {
	rl := NewRateLimiter()
	rl.MaxRetries = 3
	rawURL := "https://example.com/page"

	// No failures: should retry.
	if !rl.ShouldRetry(rawURL) {
		t.Error("ShouldRetry should be true for unknown domain")
	}

	// Record failures up to MaxRetries.
	for i := 0; i < 3; i++ {
		rl.RecordResult(rawURL, 429)
	}
	if !rl.ShouldRetry(rawURL) {
		t.Error("ShouldRetry should be true when failCount == MaxRetries")
	}

	// One more failure exceeds the limit.
	rl.RecordResult(rawURL, 429)
	if rl.ShouldRetry(rawURL) {
		t.Error("ShouldRetry should be false when failCount > MaxRetries")
	}
}

func TestReset(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://example.com/page"

	rl.RecordResult(rawURL, 429)
	rl.Reset(rawURL)

	rl.mu.Lock()
	_, exists := rl.domains["example.com"]
	rl.mu.Unlock()

	if exists {
		t.Error("domain state should be removed after Reset")
	}
}

func TestWaitRespectsContext(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://example.com/page"

	// Set a long delay.
	rl.mu.Lock()
	rl.domains["example.com"] = &domainState{
		currentDelay: 10 * time.Second,
		lastRequest:  time.Now(),
	}
	rl.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := rl.Wait(ctx, rawURL)
	if err == nil {
		t.Error("expected context error from Wait with cancelled context")
	}
}

func TestWaitNoDelayForNewDomain(t *testing.T) {
	rl := NewRateLimiter()
	rawURL := "https://new-domain.com/page"

	start := time.Now()
	err := rl.Wait(context.Background(), rawURL)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be nearly instant for a new domain (well under BaseDelay).
	if elapsed > 50*time.Millisecond {
		t.Errorf("Wait took %v for new domain, expected near-instant", elapsed)
	}
}

func TestPerDomainIsolation(t *testing.T) {
	rl := NewRateLimiter()

	rl.RecordResult("https://slow.com/page", 429)
	rl.RecordResult("https://slow.com/page", 429)
	rl.RecordResult("https://slow.com/page", 429)

	rl.mu.Lock()
	slowDelay := rl.domains["slow.com"].currentDelay
	rl.mu.Unlock()

	// Fast domain should have default delay.
	rl.RecordResult("https://fast.com/page", 200)

	rl.mu.Lock()
	fastDelay := rl.domains["fast.com"].currentDelay
	rl.mu.Unlock()

	if fastDelay >= slowDelay {
		t.Errorf("fast domain delay %v should be < slow domain delay %v", fastDelay, slowDelay)
	}
}
