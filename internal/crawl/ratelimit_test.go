package crawl

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestRL returns a RateLimiter with a short BaseDelay suitable for fast tests.
func newTestRL() *RateLimiter {
	rl := NewRateLimiter()
	rl.BaseDelay = 50 * time.Millisecond
	rl.MaxDelay = 500 * time.Millisecond
	rl.MaxRetries = 3
	rl.BackoffFactor = 2.0
	return rl
}

// TestRateWaitFirstRequest verifies that the first Wait for a new domain returns
// immediately (no previous lastRequest means elapsed > delay).
func TestRateWaitFirstRequest(t *testing.T) {
	rl := newTestRL()
	start := time.Now()
	if err := rl.Wait(context.Background(), "https://example.com/page"); err != nil {
		t.Fatalf("Wait returned unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Millisecond {
		t.Errorf("first Wait took %v, expected near-instant", elapsed)
	}
}

// TestRateWaitSecondRequestBlocks verifies that a second Wait within BaseDelay
// blocks for approximately the remaining duration.
func TestRateWaitSecondRequestBlocks(t *testing.T) {
	rl := newTestRL()

	// First call marks lastRequest.
	if err := rl.Wait(context.Background(), "https://example.com/"); err != nil {
		t.Fatalf("first Wait: %v", err)
	}

	start := time.Now()
	if err := rl.Wait(context.Background(), "https://example.com/page2"); err != nil {
		t.Fatalf("second Wait: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited close to BaseDelay (allow generous window for CI).
	if elapsed < 30*time.Millisecond {
		t.Errorf("second Wait returned too quickly (%v); expected ~%v", elapsed, rl.BaseDelay)
	}
}

// TestRateWaitContextCancellation verifies that a pending Wait returns ctx.Err()
// when the context is cancelled before the delay expires.
func TestRateWaitContextCancellation(t *testing.T) {
	rl := newTestRL()
	rl.BaseDelay = 2 * time.Second // long delay so cancel fires first

	// Prime lastRequest so the next Wait must block.
	if err := rl.Wait(context.Background(), "https://slow.com/"); err != nil {
		t.Fatalf("prime Wait: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := rl.Wait(ctx, "https://slow.com/page")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Wait should have returned an error on ctx cancellation")
	}
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Errorf("Wait error = %v, want DeadlineExceeded or Canceled", err)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("Wait blocked for %v after ctx was cancelled; expected fast return", elapsed)
	}
}

// TestRateRecordResult429DoublesDelay verifies that a 429 response doubles the
// current delay and increments failCount.
func TestRateRecordResult429DoublesDelay(t *testing.T) {
	rl := newTestRL()

	// Ensure domain state exists.
	rl.Wait(context.Background(), "https://example.com/") //nolint:errcheck

	rl.mu.Lock()
	ds := rl.domains["example.com"]
	before := ds.currentDelay
	rl.mu.Unlock()

	rl.RecordResult("https://example.com/", 429)

	rl.mu.Lock()
	after := ds.currentDelay
	failCount := ds.failCount
	rl.mu.Unlock()

	// With ±25% jitter the backed delay is in [backed*0.75, backed*1.25].
	backed := float64(before) * rl.BackoffFactor
	lo := time.Duration(backed * 0.75)
	hi := time.Duration(backed * 1.25)
	if after < lo || after > hi {
		t.Errorf("after 429 delay = %v, want in [%v, %v]", after, lo, hi)
	}
	if failCount != 1 {
		t.Errorf("failCount = %d, want 1", failCount)
	}
}

// TestRateRecordResult503AlsoBacksOff verifies that 503 behaves the same as 429.
func TestRateRecordResult503AlsoBacksOff(t *testing.T) {
	rl := newTestRL()
	rl.Wait(context.Background(), "https://example.com/") //nolint:errcheck

	rl.mu.Lock()
	ds := rl.domains["example.com"]
	before := ds.currentDelay
	rl.mu.Unlock()

	rl.RecordResult("https://example.com/", 503)

	rl.mu.Lock()
	after := ds.currentDelay
	failCount := ds.failCount
	rl.mu.Unlock()

	backed := float64(before) * rl.BackoffFactor
	lo := time.Duration(backed * 0.75)
	hi := time.Duration(backed * 1.25)
	if after < lo || after > hi {
		t.Errorf("after 503 delay = %v, want in [%v, %v]", after, lo, hi)
	}
	if failCount != 1 {
		t.Errorf("failCount = %d, want 1", failCount)
	}
}

// TestRateRecordResult200AfterBackoff verifies that a 2xx response reduces delay
// toward BaseDelay and resets failCount to zero.
func TestRateRecordResult200AfterBackoff(t *testing.T) {
	rl := newTestRL()
	rl.Wait(context.Background(), "https://example.com/") //nolint:errcheck

	// Drive failCount up.
	rl.RecordResult("https://example.com/", 429)
	rl.RecordResult("https://example.com/", 429)

	rl.mu.Lock()
	ds := rl.domains["example.com"]
	delayAfterBackoff := ds.currentDelay
	rl.mu.Unlock()

	rl.RecordResult("https://example.com/", 200)

	rl.mu.Lock()
	delayAfterRecovery := ds.currentDelay
	failCount := ds.failCount
	rl.mu.Unlock()

	if delayAfterRecovery >= delayAfterBackoff {
		t.Errorf("delay did not decrease after 200: before=%v after=%v", delayAfterBackoff, delayAfterRecovery)
	}
	if failCount != 0 {
		t.Errorf("failCount = %d after 200, want 0", failCount)
	}
	if delayAfterRecovery < rl.BaseDelay {
		t.Errorf("delay %v fell below BaseDelay %v", delayAfterRecovery, rl.BaseDelay)
	}
}

// TestRateBackoffNeverExceedsMaxDelay verifies that repeated 429s cannot push the
// delay above MaxDelay.
func TestRateBackoffNeverExceedsMaxDelay(t *testing.T) {
	rl := newTestRL()
	rl.Wait(context.Background(), "https://example.com/") //nolint:errcheck

	for range 20 {
		rl.RecordResult("https://example.com/", 429)
	}

	rl.mu.Lock()
	delay := rl.domains["example.com"].currentDelay
	rl.mu.Unlock()

	if delay > rl.MaxDelay {
		t.Errorf("delay %v exceeds MaxDelay %v", delay, rl.MaxDelay)
	}
}

// TestRateShouldRetryBelowLimit verifies ShouldRetry is true when failCount is at
// or below MaxRetries, and false when exceeded.
func TestRateShouldRetryBelowLimit(t *testing.T) {
	rl := newTestRL()

	// No state yet → should retry.
	if !rl.ShouldRetry("https://example.com/") {
		t.Error("ShouldRetry should be true for unknown domain")
	}

	// Increment failures up to MaxRetries.
	for range rl.MaxRetries {
		rl.RecordResult("https://example.com/", 429)
	}
	if !rl.ShouldRetry("https://example.com/") {
		t.Errorf("ShouldRetry should be true at failCount == MaxRetries (%d)", rl.MaxRetries)
	}

	// One more failure puts it over the limit.
	rl.RecordResult("https://example.com/", 429)
	if rl.ShouldRetry("https://example.com/") {
		t.Error("ShouldRetry should be false when failCount > MaxRetries")
	}
}

// TestRateReset verifies that Reset removes the domain state entirely.
func TestRateReset(t *testing.T) {
	rl := newTestRL()
	rl.Wait(context.Background(), "https://example.com/") //nolint:errcheck
	rl.RecordResult("https://example.com/", 429)

	rl.mu.Lock()
	_, existed := rl.domains["example.com"]
	rl.mu.Unlock()
	if !existed {
		t.Fatal("domain state should exist before Reset")
	}

	rl.Reset("https://example.com/page")

	rl.mu.Lock()
	_, stillExists := rl.domains["example.com"]
	rl.mu.Unlock()
	if stillExists {
		t.Error("domain state should be removed after Reset")
	}

	// After reset, ShouldRetry must be true again.
	if !rl.ShouldRetry("https://example.com/") {
		t.Error("ShouldRetry should be true after Reset")
	}
}

// TestRateDomainIsolation verifies that two distinct domains maintain independent
// state (backoff on one does not affect the other).
func TestRateDomainIsolation(t *testing.T) {
	rl := newTestRL()

	rl.Wait(context.Background(), "https://a.com/") //nolint:errcheck
	rl.Wait(context.Background(), "https://b.com/") //nolint:errcheck

	// Back off domain a repeatedly.
	for range 4 {
		rl.RecordResult("https://a.com/", 429)
	}

	rl.mu.Lock()
	delayA := rl.domains["a.com"].currentDelay
	failA := rl.domains["a.com"].failCount
	delayB := rl.domains["b.com"].currentDelay
	failB := rl.domains["b.com"].failCount
	rl.mu.Unlock()

	if delayA <= delayB {
		t.Errorf("domain a delay (%v) should be greater than domain b delay (%v) after backoff", delayA, delayB)
	}
	if failA == 0 {
		t.Error("domain a failCount should be > 0")
	}
	if failB != 0 {
		t.Errorf("domain b failCount = %d, want 0 (should be unaffected)", failB)
	}
}

// TestRateConcurrentSafety verifies that concurrent Wait and RecordResult calls do
// not race. Run with -race to catch data races.
func TestRateConcurrentSafety(t *testing.T) {
	rl := newTestRL()
	rl.BaseDelay = 1 * time.Millisecond // keep the test fast

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		url := "https://example.com/"
		if i%2 == 0 {
			url = "https://other.com/"
		}
		go func(u string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			rl.Wait(ctx, u) //nolint:errcheck
		}(url)
		go func(u string, idx int) {
			defer wg.Done()
			code := 200
			if idx%3 == 0 {
				code = 429
			}
			rl.RecordResult(u, code)
		}(url, i)
	}

	wg.Wait()
	// Reaching here without the race detector firing is the assertion.
}

// TestRateJitterRange verifies that after a 429 the new delay falls within the
// documented ±25% jitter band around the doubled base.
func TestRateJitterRange(t *testing.T) {
	rl := newTestRL()
	rl.Wait(context.Background(), "https://example.com/") //nolint:errcheck

	rl.mu.Lock()
	before := rl.domains["example.com"].currentDelay
	rl.mu.Unlock()

	backed := float64(before) * rl.BackoffFactor
	lo := time.Duration(backed * 0.75)
	hi := time.Duration(backed * 1.25)

	// Sample multiple times to reduce the chance a single lucky roll passes.
	const samples = 50
	for range samples {
		// Reset delay to a known value before each sample.
		rl.mu.Lock()
		rl.domains["example.com"].currentDelay = before
		rl.domains["example.com"].failCount = 0
		rl.mu.Unlock()

		rl.RecordResult("https://example.com/", 429)

		rl.mu.Lock()
		after := rl.domains["example.com"].currentDelay
		rl.mu.Unlock()

		if after < lo || after > hi {
			t.Errorf("sample delay %v outside jitter band [%v, %v]", after, lo, hi)
		}
	}
}
