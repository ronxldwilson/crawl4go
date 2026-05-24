package proxy

import (
	"sync"
	"testing"
	"time"
)

func threeProxyPool() *Pool {
	return NewPool([]Config{
		{URL: "http://proxy1:8080"},
		{URL: "http://proxy2:8080"},
		{URL: "http://proxy3:8080"},
	})
}

// TestNextRoundRobin verifies that Next cycles through proxies in order.
func TestNextRoundRobin(t *testing.T) {
	pp := threeProxyPool()

	want := []string{
		"http://proxy1:8080",
		"http://proxy2:8080",
		"http://proxy3:8080",
		"http://proxy1:8080", // wraps around
	}

	for i, w := range want {
		got := pp.Next()
		if got.URL != w {
			t.Errorf("Next() call %d: got %q, want %q", i+1, got.URL, w)
		}
	}
}

// TestNextEmptyPool verifies that Next on an empty pool returns zero Config without panic.
func TestNextEmptyPool(t *testing.T) {
	pp := NewPool(nil)
	got := pp.Next()
	if got != (Config{}) {
		t.Errorf("Next() on empty pool: got %+v, want zero Config", got)
	}
}

// TestGetForSessionSameProxyWithinTTL verifies the same proxy is returned within TTL.
func TestGetForSessionSameProxyWithinTTL(t *testing.T) {
	pp := threeProxyPool()
	ttl := 5 * time.Second

	first := pp.GetForSession("session-1", ttl)
	second := pp.GetForSession("session-1", ttl)

	if first.URL != second.URL {
		t.Errorf("GetForSession returned different proxy within TTL: %q vs %q", first.URL, second.URL)
	}
}

// TestGetForSessionZeroTTL verifies that TTL=0 always returns the same proxy.
func TestGetForSessionZeroTTL(t *testing.T) {
	pp := threeProxyPool()

	first := pp.GetForSession("session-zero", 0)
	for i := 0; i < 5; i++ {
		got := pp.GetForSession("session-zero", 0)
		if got.URL != first.URL {
			t.Errorf("GetForSession(ttl=0) call %d: got %q, want %q", i+1, got.URL, first.URL)
		}
	}
}

// TestGetForSessionAfterTTLExpired verifies that a new proxy binding is created after TTL.
func TestGetForSessionAfterTTLExpired(t *testing.T) {
	pp := threeProxyPool()
	ttl := 50 * time.Millisecond

	first := pp.GetForSession("session-exp", ttl)

	// Manually set the boundAt to a past time so the TTL is considered expired.
	pp.mu.Lock()
	pp.sessions["session-exp"].boundAt = time.Now().Add(-100 * time.Millisecond)
	pp.mu.Unlock()

	second := pp.GetForSession("session-exp", ttl)

	// After TTL, a new binding is created; the returned proxy may differ
	// (depends on pool state), but a new session entry must exist.
	pp.mu.Lock()
	binding, ok := pp.sessions["session-exp"]
	pp.mu.Unlock()

	if !ok {
		t.Fatal("GetForSession should have created a new binding after TTL expiry")
	}
	// The new binding's boundAt should be recent (< 1 second ago).
	if time.Since(binding.boundAt) > time.Second {
		t.Errorf("new binding has stale boundAt: %v", binding.boundAt)
	}
	_ = first
	_ = second
}

// TestGetForSessionIndependentSessions verifies two session IDs get separate bindings.
func TestGetForSessionIndependentSessions(t *testing.T) {
	pp := threeProxyPool()
	ttl := 5 * time.Second

	a := pp.GetForSession("session-A", ttl)
	b := pp.GetForSession("session-B", ttl)

	// Both sessions should be consistently bound.
	for i := 0; i < 3; i++ {
		if got := pp.GetForSession("session-A", ttl); got.URL != a.URL {
			t.Errorf("session-A changed unexpectedly on call %d", i+1)
		}
		if got := pp.GetForSession("session-B", ttl); got.URL != b.URL {
			t.Errorf("session-B changed unexpectedly on call %d", i+1)
		}
	}
}

// TestReleaseSessionCreatesNewBinding verifies that after ReleaseSession the next
// GetForSession creates a fresh binding.
func TestReleaseSessionCreatesNewBinding(t *testing.T) {
	pp := NewPool([]Config{
		{URL: "http://proxy1:8080"},
		{URL: "http://proxy2:8080"},
	})
	ttl := 5 * time.Second

	first := pp.GetForSession("sess", ttl)
	pp.ReleaseSession("sess")

	pp.mu.Lock()
	_, stillBound := pp.sessions["sess"]
	pp.mu.Unlock()

	if stillBound {
		t.Error("session should have been removed after ReleaseSession")
	}

	// Next GetForSession should create a new binding (possibly different proxy).
	second := pp.GetForSession("sess", ttl)
	_ = first
	_ = second
}

// TestCleanupExpired verifies expired sessions are removed and unexpired ones are kept.
func TestCleanupExpired(t *testing.T) {
	pp := threeProxyPool()

	ttl := 100 * time.Millisecond

	// Create two sessions.
	pp.GetForSession("expire-me", ttl)
	pp.GetForSession("keep-me", ttl)

	// Back-date "expire-me" so it is expired.
	pp.mu.Lock()
	pp.sessions["expire-me"].boundAt = time.Now().Add(-200 * time.Millisecond)
	pp.mu.Unlock()

	pp.CleanupExpired()

	pp.mu.Lock()
	_, expiredExists := pp.sessions["expire-me"]
	_, keepExists := pp.sessions["keep-me"]
	pp.mu.Unlock()

	if expiredExists {
		t.Error("CleanupExpired should have removed the expired session")
	}
	if !keepExists {
		t.Error("CleanupExpired should have kept the unexpired session")
	}
}

// TestConcurrentNextAndGetForSession runs concurrent Next and GetForSession calls
// to detect data races (use -race flag).
func TestConcurrentNextAndGetForSession(t *testing.T) {
	pp := threeProxyPool()
	ttl := time.Second

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			pp.Next()
		}(i)
		go func(i int) {
			defer wg.Done()
			pp.GetForSession("concurrent-sess", ttl)
		}(i)
	}
	wg.Wait()
}

// TestSize verifies that Size returns the correct number of proxies.
func TestSize(t *testing.T) {
	tests := []struct {
		name    string
		proxies []Config
		want    int
	}{
		{"empty", nil, 0},
		{"one", []Config{{URL: "http://p1"}}, 1},
		{"three", []Config{{URL: "http://p1"}, {URL: "http://p2"}, {URL: "http://p3"}}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pp := NewPool(tt.proxies)
			if got := pp.Size(); got != tt.want {
				t.Errorf("Size() = %d, want %d", got, tt.want)
			}
		})
	}
}
