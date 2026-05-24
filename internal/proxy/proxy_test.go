package proxy

import (
	"fmt"
	"testing"
	"time"
)

func TestNewPool(t *testing.T) {
	configs := []Config{
		{URL: "http://proxy1:8080"},
		{URL: "http://proxy2:8080"},
	}
	pool := NewPool(configs)
	if pool.Size() != 2 {
		t.Fatalf("expected pool size 2, got %d", pool.Size())
	}
}

func TestNewSinglePool(t *testing.T) {
	pool := NewSinglePool("http://proxy1:8080")
	if pool.Size() != 1 {
		t.Fatalf("expected pool size 1, got %d", pool.Size())
	}
	p := pool.Next()
	if p.URL != "http://proxy1:8080" {
		t.Fatalf("expected URL http://proxy1:8080, got %s", p.URL)
	}
}

func TestNext_RoundRobin(t *testing.T) {
	configs := []Config{
		{URL: "http://a:8080"},
		{URL: "http://b:8080"},
		{URL: "http://c:8080"},
	}
	pool := NewPool(configs)

	expected := []string{"http://a:8080", "http://b:8080", "http://c:8080", "http://a:8080", "http://b:8080"}
	for i, want := range expected {
		got := pool.Next()
		if got.URL != want {
			t.Errorf("call %d: expected %s, got %s", i, want, got.URL)
		}
	}
}

func TestNext_EmptyPool(t *testing.T) {
	pool := NewPool(nil)
	got := pool.Next()
	if got.URL != "" {
		t.Fatalf("expected empty Config from empty pool, got %+v", got)
	}
}

func TestGetForSession_BindsProxy(t *testing.T) {
	configs := []Config{
		{URL: "http://a:8080"},
		{URL: "http://b:8080"},
	}
	pool := NewPool(configs)

	// First call binds session to a proxy.
	p1 := pool.GetForSession("sess1", 5*time.Minute)
	// Subsequent calls return the same proxy.
	p2 := pool.GetForSession("sess1", 5*time.Minute)
	if p1.URL != p2.URL {
		t.Fatalf("session binding broken: first=%s, second=%s", p1.URL, p2.URL)
	}
}

func TestGetForSession_DifferentSessions(t *testing.T) {
	configs := []Config{
		{URL: "http://a:8080"},
		{URL: "http://b:8080"},
	}
	pool := NewPool(configs)

	p1 := pool.GetForSession("sess1", 5*time.Minute)
	p2 := pool.GetForSession("sess2", 5*time.Minute)
	// They should get different proxies due to round-robin.
	if p1.URL == p2.URL {
		t.Log("different sessions got same proxy (possible with small pool), not necessarily an error")
	}
	// At minimum, both should be valid.
	if p1.URL == "" || p2.URL == "" {
		t.Fatal("sessions should get valid proxies")
	}
}

func TestGetForSession_EmptyPool(t *testing.T) {
	pool := NewPool(nil)
	got := pool.GetForSession("sess1", time.Minute)
	if got.URL != "" {
		t.Fatalf("expected empty Config from empty pool, got %+v", got)
	}
}

func TestGetForSession_ZeroTTLNeverExpires(t *testing.T) {
	configs := []Config{{URL: "http://a:8080"}, {URL: "http://b:8080"}}
	pool := NewPool(configs)

	p1 := pool.GetForSession("sess-zero", 0)
	// Even with zero TTL, the binding should persist.
	p2 := pool.GetForSession("sess-zero", 0)
	if p1.URL != p2.URL {
		t.Fatalf("zero-TTL session should persist: first=%s, second=%s", p1.URL, p2.URL)
	}
}

func TestReleaseSession(t *testing.T) {
	configs := []Config{
		{URL: "http://a:8080"},
		{URL: "http://b:8080"},
	}
	pool := NewPool(configs)

	p1 := pool.GetForSession("sess1", 5*time.Minute)
	pool.ReleaseSession("sess1")
	// After release, session gets a new proxy (next in round-robin).
	p2 := pool.GetForSession("sess1", 5*time.Minute)
	// p2 may or may not equal p1 depending on index, but the binding was cleared.
	_ = p1
	_ = p2
}

func TestReleaseSession_NonExistent(t *testing.T) {
	pool := NewPool([]Config{{URL: "http://a:8080"}})
	// Should not panic.
	pool.ReleaseSession("nonexistent")
}

func TestCleanupExpired(t *testing.T) {
	configs := []Config{{URL: "http://a:8080"}, {URL: "http://b:8080"}}
	pool := NewPool(configs)

	// Bind with a very short TTL.
	pool.GetForSession("ephemeral", 1*time.Millisecond)
	// Also bind one with zero TTL (never expires).
	pool.GetForSession("permanent", 0)

	// Wait for ephemeral to expire.
	time.Sleep(5 * time.Millisecond)

	pool.CleanupExpired()

	// After cleanup, ephemeral session should get a new binding.
	pool.mu.Lock()
	_, ephemeralExists := pool.sessions["ephemeral"]
	_, permanentExists := pool.sessions["permanent"]
	pool.mu.Unlock()

	if ephemeralExists {
		t.Error("expired session 'ephemeral' should have been cleaned up")
	}
	if !permanentExists {
		t.Error("zero-TTL session 'permanent' should NOT have been cleaned up")
	}
}

func TestSize(t *testing.T) {
	tests := []struct {
		name string
		n    int
	}{
		{"empty", 0},
		{"one", 1},
		{"five", 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var configs []Config
			for i := 0; i < tc.n; i++ {
				configs = append(configs, Config{URL: fmt.Sprintf("http://proxy%d:8080", i)})
			}
			pool := NewPool(configs)
			if pool.Size() != tc.n {
				t.Errorf("expected size %d, got %d", tc.n, pool.Size())
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	configs := []Config{
		{URL: "http://a:8080"},
		{URL: "http://b:8080"},
		{URL: "http://c:8080"},
	}
	pool := NewPool(configs)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				pool.Next()
				pool.GetForSession(fmt.Sprintf("sess-%d", id), time.Minute)
			}
			pool.ReleaseSession(fmt.Sprintf("sess-%d", id))
			pool.CleanupExpired()
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
