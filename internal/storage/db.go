// Package storage provides persistent crawl-result caching backed by a SQL
// database.  The implementation is driver-agnostic: it uses only database/sql
// interfaces, so any compatible driver (e.g. modernc.org/sqlite,
// mattn/go-sqlite3, or even PostgreSQL) can be plugged in by importing the
// driver package elsewhere and passing the correct DSN to NewCrawlDB.
package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// CachedResult represents a single cached crawl response stored in the
// database.
type CachedResult struct {
	URL        string
	HTML       string
	Markdown   string
	StatusCode int
	FetchedAt  time.Time
	Headers    string // JSON-encoded response headers
}

// CrawlDB wraps a *sql.DB and exposes CRUD helpers for crawl-result caching.
// Connection pooling is handled by sql.DB's built-in pool; callers may tune it
// via the underlying DB field after construction.
type CrawlDB struct {
	DB *sql.DB

	// Retry configuration for executeWithRetry.
	maxRetries int
	baseDelay  time.Duration
}

// NewCrawlDB opens (or creates) a SQLite database at dbPath and returns a
// ready-to-use CrawlDB.  The caller must supply the driver name that was
// registered via a blank import (e.g. "sqlite" for modernc.org/sqlite).
//
// If driverName is empty it defaults to "sqlite".
func NewCrawlDB(driverName, dbPath string) (*CrawlDB, error) {
	if driverName == "" {
		driverName = "sqlite"
	}

	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage: open db: %w", err)
	}

	// Sensible pool defaults — callers can override via cdb.DB.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: ping db: %w", err)
	}

	return &CrawlDB{
		DB:         db,
		maxRetries: 3,
		baseDelay:  100 * time.Millisecond,
	}, nil
}

// InitSchema creates the crawl_cache table if it does not already exist.
func (c *CrawlDB) InitSchema() error {
	const ddl = `CREATE TABLE IF NOT EXISTS crawl_cache (
		url         TEXT PRIMARY KEY,
		html        TEXT,
		markdown    TEXT,
		status_code INTEGER,
		fetched_at  TIMESTAMP,
		headers     TEXT
	)`

	return c.executeWithRetry(func() error {
		_, err := c.DB.Exec(ddl)
		return err
	})
}

// Get retrieves the cached result for url. It returns nil (without error) when
// no row exists for the given URL.
func (c *CrawlDB) Get(url string) (*CachedResult, error) {
	var result *CachedResult
	err := c.executeWithRetry(func() error {
		row := c.DB.QueryRow(
			`SELECT url, html, markdown, status_code, fetched_at, headers
			   FROM crawl_cache WHERE url = ?`, url)

		r := CachedResult{}
		if err := row.Scan(&r.URL, &r.HTML, &r.Markdown, &r.StatusCode, &r.FetchedAt, &r.Headers); err != nil {
			if err == sql.ErrNoRows {
				result = nil
				return nil
			}
			return err
		}
		result = &r
		return nil
	})
	return result, err
}

// Put inserts or replaces the cached result for the given URL.
func (c *CrawlDB) Put(url string, r *CachedResult) error {
	return c.executeWithRetry(func() error {
		_, err := c.DB.Exec(
			`INSERT OR REPLACE INTO crawl_cache (url, html, markdown, status_code, fetched_at, headers)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			url, r.HTML, r.Markdown, r.StatusCode, r.FetchedAt, r.Headers)
		return err
	})
}

// Delete removes the cached result for the given URL.
func (c *CrawlDB) Delete(url string) error {
	return c.executeWithRetry(func() error {
		_, err := c.DB.Exec(`DELETE FROM crawl_cache WHERE url = ?`, url)
		return err
	})
}

// Close closes the underlying database connection pool.
func (c *CrawlDB) Close() error {
	return c.DB.Close()
}

// executeWithRetry runs fn up to c.maxRetries times with linear backoff.
// The delay between attempt i and i+1 is (i+1) * c.baseDelay.
func (c *CrawlDB) executeWithRetry(fn func() error) error {
	var err error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if attempt < c.maxRetries-1 {
			time.Sleep(time.Duration(attempt+1) * c.baseDelay)
		}
	}
	return fmt.Errorf("storage: operation failed after %d attempts: %w", c.maxRetries, err)
}
