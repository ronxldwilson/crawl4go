package crawl

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemCacheGetSet(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		resp    CachedResponse
		wantOK  bool
	}{
		{
			name:   "basic set and get",
			url:    "https://example.com",
			resp:   CachedResponse{HTML: "<html>test</html>", StatusCode: 200, CachedAt: time.Now()},
			wantOK: true,
		},
		{
			name:   "get non-existent key",
			url:    "https://missing.com",
			resp:   CachedResponse{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewMemCache(10, time.Hour)

			if tt.wantOK {
				cache.Set(tt.url, tt.resp)
			}

			got, ok := cache.Get(tt.url)
			if ok != tt.wantOK {
				t.Fatalf("Get ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK {
				if got.HTML != tt.resp.HTML {
					t.Errorf("HTML = %q, want %q", got.HTML, tt.resp.HTML)
				}
				if got.StatusCode != tt.resp.StatusCode {
					t.Errorf("StatusCode = %d, want %d", got.StatusCode, tt.resp.StatusCode)
				}
			}
		})
	}
}

func TestMemCacheTTLExpiry(t *testing.T) {
	cache := NewMemCache(10, 50*time.Millisecond)

	cache.Set("https://example.com", CachedResponse{
		HTML:       "<html>expires</html>",
		StatusCode: 200,
		CachedAt:   time.Now(),
	})

	// Should be present immediately.
	if _, ok := cache.Get("https://example.com"); !ok {
		t.Fatal("entry should exist before TTL expires")
	}

	// Wait for TTL to pass.
	time.Sleep(100 * time.Millisecond)

	if _, ok := cache.Get("https://example.com"); ok {
		t.Error("entry should have expired after TTL")
	}
}

func TestMemCacheMaxEntriesEviction(t *testing.T) {
	cache := NewMemCache(2, time.Hour)

	// Insert three entries; the oldest should be evicted.
	cache.Set("https://a.com", CachedResponse{HTML: "a", StatusCode: 200, CachedAt: time.Now().Add(-2 * time.Second)})
	cache.Set("https://b.com", CachedResponse{HTML: "b", StatusCode: 200, CachedAt: time.Now().Add(-1 * time.Second)})
	cache.Set("https://c.com", CachedResponse{HTML: "c", StatusCode: 200, CachedAt: time.Now()})

	// "a" should have been evicted (oldest CachedAt).
	if _, ok := cache.Get("https://a.com"); ok {
		t.Error("oldest entry 'a' should have been evicted")
	}

	// "b" and "c" should still be present.
	if _, ok := cache.Get("https://b.com"); !ok {
		t.Error("entry 'b' should still exist")
	}
	if _, ok := cache.Get("https://c.com"); !ok {
		t.Error("entry 'c' should still exist")
	}
}

func TestMemCacheStats(t *testing.T) {
	cache := NewMemCache(10, time.Hour)

	cache.Set("https://a.com", CachedResponse{HTML: "a", StatusCode: 200, CachedAt: time.Now()})

	// One hit.
	cache.Get("https://a.com")
	// One miss.
	cache.Get("https://missing.com")

	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
	if stats.Entries != 1 {
		t.Errorf("Entries = %d, want 1", stats.Entries)
	}
	if stats.MaxEntries != 10 {
		t.Errorf("MaxEntries = %d, want 10", stats.MaxEntries)
	}
}

func TestMemCacheThreadSafety(t *testing.T) {
	cache := NewMemCache(100, time.Hour)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		url := fmt.Sprintf("https://example.com/%d", i)
		go func() {
			defer wg.Done()
			cache.Set(url, CachedResponse{HTML: url, StatusCode: 200, CachedAt: time.Now()})
		}()
		go func() {
			defer wg.Done()
			cache.Get(url)
		}()
	}
	wg.Wait()

	// If we get here without a race detector complaint, the test passes.
	stats := cache.Stats()
	if stats.Entries < 1 {
		t.Error("expected at least some entries after concurrent writes")
	}
}
