package crawl

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type robotsEntry struct {
	rules     []robotsRule
	fetchedAt time.Time
}

type robotsRule struct {
	userAgent string
	disallow  []string
	allow     []string
}

type RobotsChecker struct {
	mu     sync.RWMutex
	cache  map[string]*robotsEntry
	client *http.Client
	ttl    time.Duration
}

func NewRobotsChecker() *RobotsChecker {
	return &RobotsChecker{
		cache:  make(map[string]*robotsEntry),
		client: &http.Client{Timeout: 10 * time.Second},
		ttl:    7 * 24 * time.Hour,
	}
}

func (rc *RobotsChecker) CanFetch(ctx context.Context, userAgent, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := u.Scheme + "://" + u.Host

	entry := rc.getEntry(host)
	if entry == nil {
		entry = rc.fetchAndCache(ctx, host)
	}
	if entry == nil {
		return true
	}

	path := u.Path
	if path == "" {
		path = "/"
	}

	return rc.isAllowed(entry, userAgent, path)
}

func (rc *RobotsChecker) getEntry(host string) *robotsEntry {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	entry, ok := rc.cache[host]
	if !ok {
		return nil
	}
	if time.Since(entry.fetchedAt) > rc.ttl {
		return nil
	}
	return entry
}

func (rc *RobotsChecker) fetchAndCache(ctx context.Context, host string) *robotsEntry {
	robotsURL := host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return rc.cacheEmpty(host)
	}

	resp, err := rc.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return rc.cacheEmpty(host)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return rc.cacheEmpty(host)
	}

	entry := rc.parseRobots(string(body))

	rc.mu.Lock()
	rc.cache[host] = entry
	rc.mu.Unlock()

	return entry
}

func (rc *RobotsChecker) cacheEmpty(host string) *robotsEntry {
	entry := &robotsEntry{fetchedAt: time.Now()}
	rc.mu.Lock()
	rc.cache[host] = entry
	rc.mu.Unlock()
	return entry
}

func (rc *RobotsChecker) parseRobots(content string) *robotsEntry {
	entry := &robotsEntry{fetchedAt: time.Now()}
	var current *robotsRule

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "user-agent":
			entry.rules = append(entry.rules, robotsRule{userAgent: strings.ToLower(value)})
			current = &entry.rules[len(entry.rules)-1]
		case "disallow":
			if current != nil && value != "" {
				current.disallow = append(current.disallow, value)
			}
		case "allow":
			if current != nil && value != "" {
				current.allow = append(current.allow, value)
			}
		}
	}

	return entry
}

func (rc *RobotsChecker) isAllowed(entry *robotsEntry, userAgent, path string) bool {
	ua := strings.ToLower(userAgent)

	var matchedRule *robotsRule
	for i := range entry.rules {
		r := &entry.rules[i]
		if r.userAgent == "*" || strings.Contains(ua, r.userAgent) {
			matchedRule = r
			if r.userAgent != "*" {
				break
			}
		}
	}

	if matchedRule == nil {
		return true
	}

	for _, pattern := range matchedRule.allow {
		if pathMatches(path, pattern) {
			return true
		}
	}

	for _, pattern := range matchedRule.disallow {
		if pathMatches(path, pattern) {
			return false
		}
	}

	return true
}

func pathMatches(path, pattern string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, pattern[:len(pattern)-1])
	}
	if strings.HasSuffix(pattern, "$") {
		return path == pattern[:len(pattern)-1]
	}
	return strings.HasPrefix(path, pattern)
}
