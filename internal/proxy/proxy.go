package proxy

import (
	"sync"
	"time"
)

type Config struct {
	URL      string
	Username string
	Password string
}

type sessionBinding struct {
	proxy   Config
	boundAt time.Time
	ttl     time.Duration
}

type Pool struct {
	mu       sync.Mutex
	proxies  []Config
	index    int
	sessions map[string]*sessionBinding
}

func NewPool(proxies []Config) *Pool {
	return &Pool{
		proxies:  proxies,
		sessions: make(map[string]*sessionBinding),
	}
}

func NewSinglePool(proxyURL string) *Pool {
	return NewPool([]Config{{URL: proxyURL}})
}

func (pp *Pool) Next() Config {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if len(pp.proxies) == 0 {
		return Config{}
	}
	proxy := pp.proxies[pp.index%len(pp.proxies)]
	pp.index++
	return proxy
}

func (pp *Pool) GetForSession(sessionID string, ttl time.Duration) Config {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	if binding, ok := pp.sessions[sessionID]; ok {
		if ttl == 0 || time.Since(binding.boundAt) < binding.ttl {
			return binding.proxy
		}
		delete(pp.sessions, sessionID)
	}

	if len(pp.proxies) == 0 {
		return Config{}
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

func (pp *Pool) ReleaseSession(sessionID string) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	delete(pp.sessions, sessionID)
}

func (pp *Pool) CleanupExpired() {
	pp.mu.Lock()
	defer pp.mu.Unlock()

	now := time.Now()
	for id, binding := range pp.sessions {
		if binding.ttl > 0 && now.Sub(binding.boundAt) >= binding.ttl {
			delete(pp.sessions, id)
		}
	}
}

func (pp *Pool) Size() int {
	return len(pp.proxies)
}
