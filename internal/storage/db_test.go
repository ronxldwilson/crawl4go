package storage

import (
	"errors"
	"testing"
	"time"
)

func TestNewCrawlDB_InvalidDriver(t *testing.T) {
	_, err := NewCrawlDB("nonexistent_driver", ":memory:")
	if err == nil {
		t.Fatal("expected error for unregistered driver, got nil")
	}
}

func TestNewCrawlDB_EmptyDriverDefaultsSQLite(t *testing.T) {
	// "sqlite" driver is not registered in this module, so we still expect an
	// error — but the code path that sets the default driver name is exercised.
	_, err := NewCrawlDB("", ":memory:")
	if err == nil {
		t.Fatal("expected error because no sqlite driver is imported, got nil")
	}
}

func TestCachedResult_Fields(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	r := CachedResult{
		URL:        "https://example.com",
		HTML:       "<html></html>",
		Markdown:   "# hello",
		StatusCode: 200,
		FetchedAt:  now,
		Headers:    `{"Content-Type":"text/html"}`,
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"URL", r.URL, "https://example.com"},
		{"HTML", r.HTML, "<html></html>"},
		{"Markdown", r.Markdown, "# hello"},
		{"StatusCode", r.StatusCode, 200},
		{"FetchedAt", r.FetchedAt, now},
		{"Headers", r.Headers, `{"Content-Type":"text/html"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

var errSentinel = errors.New("test error")

func TestExecuteWithRetry_SucceedsFirstAttempt(t *testing.T) {
	cdb := &CrawlDB{maxRetries: 3, baseDelay: time.Millisecond}
	calls := 0
	err := cdb.executeWithRetry(func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestExecuteWithRetry_RetriesOnError(t *testing.T) {
	cdb := &CrawlDB{maxRetries: 3, baseDelay: time.Millisecond}
	calls := 0
	err := cdb.executeWithRetry(func() error {
		calls++
		return errSentinel
	})
	if err == nil {
		t.Fatal("expected error after retries, got nil")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (maxRetries), got %d", calls)
	}
}

func TestExecuteWithRetry_SucceedsOnSecondAttempt(t *testing.T) {
	cdb := &CrawlDB{maxRetries: 3, baseDelay: time.Millisecond}
	calls := 0
	err := cdb.executeWithRetry(func() error {
		calls++
		if calls < 2 {
			return errSentinel
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestExecuteWithRetry_ZeroRetries(t *testing.T) {
	// maxRetries=0 means the loop body never runs; fn is never called.
	cdb := &CrawlDB{maxRetries: 0, baseDelay: time.Millisecond}
	calls := 0
	_ = cdb.executeWithRetry(func() error {
		calls++
		return nil
	})
	if calls != 0 {
		t.Fatalf("expected 0 calls with maxRetries=0, got %d", calls)
	}
}

func TestExecuteWithRetry_ErrorWrapped(t *testing.T) {
	cdb := &CrawlDB{maxRetries: 2, baseDelay: time.Millisecond}
	err := cdb.executeWithRetry(func() error { return errSentinel })
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errSentinel) {
		t.Errorf("expected wrapped errSentinel, got %v", err)
	}
}
