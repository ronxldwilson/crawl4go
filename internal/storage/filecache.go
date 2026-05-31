package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileCache is a dependency-free, persistent crawl-result cache backed by the
// filesystem. Each entry is stored as a JSON file named by the SHA-256 of its
// URL, so the cache survives restarts without pulling in a SQL driver. It is
// safe for concurrent use.
//
// It deliberately mirrors the CRUD surface of CrawlDB (Get/Put/Delete/Close)
// using the same CachedResult type, so callers can choose either backend.
type FileCache struct {
	dir string
	ttl time.Duration // zero means entries never expire

	mu sync.RWMutex // guards filesystem writes against concurrent same-key access
}

// NewFileCache returns a FileCache rooted at dir, creating the directory if it
// does not exist. A ttl of zero disables expiry.
func NewFileCache(dir string, ttl time.Duration) (*FileCache, error) {
	if dir == "" {
		return nil, errors.New("storage: file cache dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: create cache dir: %w", err)
	}
	return &FileCache{dir: dir, ttl: ttl}, nil
}

// path returns the on-disk path for a URL's cache entry.
func (c *FileCache) path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])+".json")
}

// Get returns the cached result for url, or nil (without error) when there is
// no entry or the entry has expired. Expired entries are removed on read.
func (c *FileCache) Get(url string) (*CachedResult, error) {
	c.mu.RLock()
	data, err := os.ReadFile(c.path(url))
	c.mu.RUnlock()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: read cache: %w", err)
	}

	var r CachedResult
	if err := json.Unmarshal(data, &r); err != nil {
		// Treat a corrupt entry as a miss and drop it.
		_ = c.Delete(url)
		return nil, nil
	}

	if c.ttl > 0 && time.Since(r.FetchedAt) > c.ttl {
		_ = c.Delete(url)
		return nil, nil
	}
	return &r, nil
}

// Put writes or replaces the cached result for url. The write is atomic: data
// is written to a temp file in the same directory and renamed into place.
func (c *FileCache) Put(url string, r *CachedResult) error {
	if r == nil {
		return errors.New("storage: nil cached result")
	}
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("storage: marshal cache: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	final := c.path(url)
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("storage: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("storage: write cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("storage: close cache: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("storage: commit cache: %w", err)
	}
	return nil
}

// Delete removes the cache entry for url. A missing entry is not an error.
func (c *FileCache) Delete(url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.Remove(c.path(url)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage: delete cache: %w", err)
	}
	return nil
}

// Close releases resources. It is a no-op for the filesystem cache and exists
// for parity with CrawlDB.
func (c *FileCache) Close() error { return nil }
