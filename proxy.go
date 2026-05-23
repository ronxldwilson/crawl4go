package main

import (
	"sync"
	"time"
)

type ProxyConfig struct {
	URL      string
	Username string
	Password string
}

type sessionBinding struct {
	proxy     ProxyConfig
	boundAt   time.Time
	ttl       time.Duration
}

type ProxyPool struct {
	mu       sync.Mutex
	proxies  []ProxyConfig
	index    int
	sessions map[string]*sessionBinding
}

func NewProxyPool(proxies []ProxyConfig) *ProxyPool {
	return &ProxyPool{
		proxies:  proxies,
		sessions: make(map[string]*sessionBinding),
	}
}

// NewSingleProxyPool creates a pool with a single proxy URL (e.g., Tor proxy).
func NewSingleProxyPool(proxyURL string) *ProxyPool {
	return NewProxyPool([]ProxyConfig{{URL: proxyURL}})
}

// Next returns the next proxy in round-robin order.
func (pp *ProxyPool) Next() ProxyConfig {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if len(pp.proxies) == 0 {
		return ProxyConfig{}
	}
	proxy := pp.proxies[pp.index%len(pp.proxies)]
	pp.index++
	return proxy
}

// GetForSession returns a sticky proxy for the given session ID.
// If the session already has a binding (and it hasn't expired), return the same proxy.
// Otherwise, assign the next proxy and bind it.
func (pp *ProxyPool) GetForSession(sessionID string, ttl time.Duration) ProxyConfig {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if binding, ok := pp.sessions[sessionID]; ok {
		if ttl == 0 || time.Since(binding.boundAt) < binding.ttl {
			return binding.proxy
		}
		delete(pp.sessions, sessionID)
	}

	if len(pp.proxies) == 0 {
		return ProxyConfig{}
	}
	proxy := pp.proxies[pp.index%len(pp.proxies)]
	pp.index++

	pp.sessions[sessionID] = &sessionBinding{
		proxy:   proxy,
		boundAt: time.Now(),
		ttl:     ttl,
	}
	return proxy
}

// ReleaseSession removes a session's proxy binding.
func (pp *ProxyPool) ReleaseSession(sessionID string) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	delete(pp.sessions, sessionID)
}

// CleanupExpired removes all expired session bindings.
func (pp *ProxyPool) CleanupExpired() {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	now := time.Now()
	for id, binding := range pp.sessions {
		if binding.ttl > 0 && now.Sub(binding.boundAt) >= binding.ttl {
			delete(pp.sessions, id)
		}
	}
}

// Size returns the number of proxies in the pool.
func (pp *ProxyPool) Size() int {
	return len(pp.proxies)
}
