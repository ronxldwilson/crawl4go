package storage

import (
	"testing"
	"time"
)

func TestFileCachePutGetRoundTrip(t *testing.T) {
	fc, err := NewFileCache(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}

	want := &CachedResult{
		URL:        "https://example.com/page",
		HTML:       "<html>hi</html>",
		Markdown:   "# hi",
		StatusCode: 200,
		FetchedAt:  time.Now().UTC().Truncate(time.Second),
		Headers:    `{"content-type":"text/html"}`,
	}
	if err := fc.Put(want.URL, want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := fc.Get(want.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil for a stored entry")
	}
	if got.URL != want.URL || got.HTML != want.HTML || got.Markdown != want.Markdown ||
		got.StatusCode != want.StatusCode || got.Headers != want.Headers {
		t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestFileCacheMissReturnsNil(t *testing.T) {
	fc, err := NewFileCache(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	got, err := fc.Get("https://nope.example/missing")
	if err != nil {
		t.Fatalf("Get returned error for a miss: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a miss, got %+v", got)
	}
}

func TestFileCacheTTLExpiry(t *testing.T) {
	fc, err := NewFileCache(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	url := "https://example.com/stale"
	// Stored two hours ago → older than the 1h TTL → must be treated as a miss.
	if err := fc.Put(url, &CachedResult{URL: url, FetchedAt: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := fc.Get(url)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("expected expired entry to be a miss, got %+v", got)
	}
	// Expired entry should have been removed on read.
	if got2, _ := fc.Get(url); got2 != nil {
		t.Error("expired entry was not purged on read")
	}
}

func TestFileCacheTTLZeroNeverExpires(t *testing.T) {
	fc, err := NewFileCache(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	url := "https://example.com/forever"
	if err := fc.Put(url, &CachedResult{URL: url, FetchedAt: time.Now().Add(-1000 * time.Hour)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := fc.Get(url)
	if err != nil || got == nil {
		t.Errorf("ttl=0 should never expire; got=%v err=%v", got, err)
	}
}

func TestFileCacheDelete(t *testing.T) {
	fc, err := NewFileCache(t.TempDir(), 0)
	if err != nil {
		t.Fatalf("NewFileCache: %v", err)
	}
	url := "https://example.com/del"
	_ = fc.Put(url, &CachedResult{URL: url, FetchedAt: time.Now()})
	if err := fc.Delete(url); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := fc.Get(url); got != nil {
		t.Error("entry still present after Delete")
	}
	// Deleting a missing entry is not an error.
	if err := fc.Delete("https://example.com/never"); err != nil {
		t.Errorf("Delete of missing entry should be nil, got %v", err)
	}
}
