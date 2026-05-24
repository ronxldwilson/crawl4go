package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newValidator returns a CacheValidator whose HTTP client talks to the given
// test server (may be nil, in which case the default client is used).
func newValidator(server *httptest.Server) *CacheValidator {
	if server == nil {
		return NewCacheValidator(http.DefaultClient)
	}
	return NewCacheValidator(server.Client())
}

const sampleHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Hello World</title>
  <meta name="description" content="A test page">
  <meta property="og:title" content="Hello OG">
  <link rel="canonical" href="https://example.com/hello">
</head>
<body>Some body text</body>
</html>`

const altHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Different Title</title>
  <meta name="description" content="Changed description">
</head>
<body>Other body</body>
</html>`

// ---------------------------------------------------------------------------
// IsFresh tests
// ---------------------------------------------------------------------------

// TestCacheIsFresh_UnknownURL verifies that IsFresh returns (false, nil) when
// no cache entry exists for the URL (no network call should be made).
func TestCacheIsFresh_UnknownURL(t *testing.T) {
	cv := NewCacheValidator(http.DefaultClient)
	fresh, err := cv.IsFresh(context.Background(), "https://example.com/unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fresh {
		t.Fatal("expected false for unknown URL")
	}
}

// TestCacheIsFresh_304 verifies that a 304 response causes IsFresh to return
// (true, nil).
func TestCacheIsFresh_304(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cv := newValidator(ts)
	cv.Update(ts.URL+"/page", `"etag-abc"`, "", sampleHTML)

	fresh, err := cv.IsFresh(context.Background(), ts.URL+"/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fresh {
		t.Fatal("expected true for 304 response")
	}
}

// TestCacheIsFresh_200_SameFingerprint verifies that a 200 response with the
// same body fingerprint causes IsFresh to return (true, nil).
func TestCacheIsFresh_200_SameFingerprint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 200 — no conditional headers honoured.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleHTML))
	}))
	defer ts.Close()

	cv := newValidator(ts)
	cv.Update(ts.URL+"/page", "", "", sampleHTML)

	fresh, err := cv.IsFresh(context.Background(), ts.URL+"/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fresh {
		t.Fatal("expected true when fingerprint matches")
	}
}

// TestCacheIsFresh_200_ChangedContent verifies that a 200 response with a
// different body fingerprint causes IsFresh to return (false, nil).
func TestCacheIsFresh_200_ChangedContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(altHTML))
	}))
	defer ts.Close()

	cv := newValidator(ts)
	// Store entry with sampleHTML fingerprint; server will return altHTML.
	cv.Update(ts.URL+"/page", "", "", sampleHTML)

	fresh, err := cv.IsFresh(context.Background(), ts.URL+"/page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fresh {
		t.Fatal("expected false when fingerprint has changed")
	}
}

// TestCacheIsFresh_404 verifies that a 404 response causes IsFresh to return
// (false, nil).
func TestCacheIsFresh_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cv := newValidator(ts)
	cv.Update(ts.URL+"/gone", "", "", sampleHTML)

	fresh, err := cv.IsFresh(context.Background(), ts.URL+"/gone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fresh {
		t.Fatal("expected false for 404 response")
	}
}

// TestCacheIsFresh_NetworkError verifies that a network error causes IsFresh
// to return (false, non-nil error).
func TestCacheIsFresh_NetworkError(t *testing.T) {
	// Start a server then immediately close it to produce a connection-refused
	// error.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := ts.URL + "/page"
	ts.Close() // closed before the request is made

	cv := NewCacheValidator(http.DefaultClient)
	cv.Update(url, "", "", sampleHTML)

	fresh, err := cv.IsFresh(context.Background(), url)
	if err == nil {
		t.Fatal("expected a network error but got nil")
	}
	if fresh {
		t.Fatal("expected false on network error")
	}
}

// TestCacheIsFresh_CtxCancellation verifies that a cancelled context causes
// IsFresh to return (false, ctx.Err()).
func TestCacheIsFresh_CtxCancellation(t *testing.T) {
	// Server that blocks until the test is done, ensuring the client actually
	// sees the cancellation.
	unblock := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
	}))
	defer ts.Close()
	defer close(unblock)

	cv := newValidator(ts)
	cv.Update(ts.URL+"/slow", "", "", sampleHTML)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fresh, err := cv.IsFresh(ctx, ts.URL+"/slow")
	if err == nil {
		t.Fatal("expected context error but got nil")
	}
	if fresh {
		t.Fatal("expected false on context cancellation")
	}
}

// ---------------------------------------------------------------------------
// computeHeadFingerprint tests
// ---------------------------------------------------------------------------

// TestComputeHeadFingerprint_DifferentTitles verifies that pages with different
// titles produce different fingerprints.
func TestComputeHeadFingerprint_DifferentTitles(t *testing.T) {
	html1 := `<html><head><title>Title One</title></head></html>`
	html2 := `<html><head><title>Title Two</title></head></html>`

	fp1 := computeHeadFingerprint(html1)
	fp2 := computeHeadFingerprint(html2)

	if fp1 == fp2 {
		t.Errorf("expected different fingerprints for different titles, got %q for both", fp1)
	}
}

// TestComputeHeadFingerprint_Identical verifies that identical HTML produces
// the same fingerprint on repeated calls.
func TestComputeHeadFingerprint_Identical(t *testing.T) {
	fp1 := computeHeadFingerprint(sampleHTML)
	fp2 := computeHeadFingerprint(sampleHTML)

	if fp1 != fp2 {
		t.Errorf("expected same fingerprint for identical HTML, got %q and %q", fp1, fp2)
	}
}

// TestComputeHeadFingerprint_MalformedHTML verifies that malformed HTML that
// cannot be parsed still returns a non-empty fingerprint (the raw SHA-256
// fallback).  golang.org/x/net/html actually tolerates almost any input, so
// we test the fallback indirectly: ensure we always get a 64-char hex digest.
func TestComputeHeadFingerprint_MalformedHTML(t *testing.T) {
	malformed := strings.Repeat("<<<<>>>>>", 50) // junk that is not valid HTML

	fp := computeHeadFingerprint(malformed)
	if len(fp) != 64 {
		t.Errorf("expected 64-char hex fingerprint, got %q (len %d)", fp, len(fp))
	}
}

// TestComputeHeadFingerprint_EmptyContent verifies that an empty string
// produces a deterministic 64-char fingerprint.
func TestComputeHeadFingerprint_EmptyContent(t *testing.T) {
	fp1 := computeHeadFingerprint("")
	fp2 := computeHeadFingerprint("")
	if len(fp1) != 64 {
		t.Errorf("expected 64-char hex fingerprint, got len %d", len(fp1))
	}
	if fp1 != fp2 {
		t.Error("expected same fingerprint for empty content on repeated calls")
	}
}

// ---------------------------------------------------------------------------
// Get / Remove tests
// ---------------------------------------------------------------------------

// TestCacheGet_StoresAndRetrieves verifies that Get returns the entry stored
// by Update.
func TestCacheGet_StoresAndRetrieves(t *testing.T) {
	cv := NewCacheValidator(http.DefaultClient)
	url := "https://example.com/page"

	if cv.Get(url) != nil {
		t.Fatal("expected nil for unknown URL")
	}

	cv.Update(url, `"etag-1"`, "Mon, 01 Jan 2024 00:00:00 GMT", sampleHTML)

	entry := cv.Get(url)
	if entry == nil {
		t.Fatal("expected non-nil entry after Update")
	}
	if entry.URL != url {
		t.Errorf("URL = %q, want %q", entry.URL, url)
	}
	if entry.ETag != `"etag-1"` {
		t.Errorf("ETag = %q, want %q", entry.ETag, `"etag-1"`)
	}
	if entry.LastModified != "Mon, 01 Jan 2024 00:00:00 GMT" {
		t.Errorf("LastModified = %q", entry.LastModified)
	}
	if entry.HeadFingerprint == "" {
		t.Error("HeadFingerprint should not be empty")
	}
}

// TestCacheRemove_DeletesEntry verifies that Remove makes Get return nil.
func TestCacheRemove_DeletesEntry(t *testing.T) {
	cv := NewCacheValidator(http.DefaultClient)
	url := "https://example.com/page"

	cv.Update(url, "", "", sampleHTML)
	if cv.Get(url) == nil {
		t.Fatal("expected entry after Update")
	}

	cv.Remove(url)
	if cv.Get(url) != nil {
		t.Fatal("expected nil after Remove")
	}
}

// TestCacheRemove_NonExistentURL verifies that Remove on an unknown URL does
// not panic or error.
func TestCacheRemove_NonExistentURL(t *testing.T) {
	cv := NewCacheValidator(http.DefaultClient)
	cv.Remove("https://example.com/nope") // must not panic
}

// ---------------------------------------------------------------------------
// Update idempotency / replacement
// ---------------------------------------------------------------------------

// TestCacheUpdate_ReplacesExistingEntry verifies that calling Update twice for
// the same URL replaces the previous entry.
func TestCacheUpdate_ReplacesExistingEntry(t *testing.T) {
	cv := NewCacheValidator(http.DefaultClient)
	url := "https://example.com/page"

	cv.Update(url, `"etag-1"`, "", sampleHTML)
	fp1 := cv.Get(url).HeadFingerprint

	cv.Update(url, `"etag-2"`, "", altHTML)
	entry := cv.Get(url)

	if entry.ETag != `"etag-2"` {
		t.Errorf("ETag = %q after second Update, want %q", entry.ETag, `"etag-2"`)
	}
	fp2 := entry.HeadFingerprint
	if fp1 == fp2 {
		t.Error("expected fingerprint to change after Update with different content")
	}
}

// ---------------------------------------------------------------------------
// Concurrency / race-detector test
// ---------------------------------------------------------------------------

// TestCacheConcurrentUpdateAndIsFresh runs concurrent Update and IsFresh calls
// to verify there are no data races (run with -race).
func TestCacheConcurrentUpdateAndIsFresh(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer ts.Close()

	cv := newValidator(ts)
	url := ts.URL + "/page"
	cv.Update(url, `"etag-init"`, "", sampleHTML)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			cv.Update(url, `"etag-x"`, "", sampleHTML)
		}(i)

		go func() {
			defer wg.Done()
			_, _ = cv.IsFresh(context.Background(), url)
		}()
	}

	wg.Wait()
}
