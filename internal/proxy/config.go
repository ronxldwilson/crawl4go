package proxy

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"
)

// ProxyType identifies the proxy protocol.
type ProxyType string

const (
	ProxyHTTP   ProxyType = "http"
	ProxyHTTPS  ProxyType = "https"
	ProxySocks5 ProxyType = "socks5"
)

// ProxyConfig is a richer proxy configuration with auth, type, and health tracking.
type ProxyConfig struct {
	URL      string    `json:"url"`
	Type     ProxyType `json:"type,omitempty"`
	Username string    `json:"username,omitempty"`
	Password string    `json:"password,omitempty"`

	// Region is a geo/region hint for the proxy (for region-aware rotation).
	Region string `json:"region,omitempty"`

	// Health tracking
	Healthy      bool      `json:"healthy"`
	LastUsed     time.Time `json:"last_used"`
	LastChecked  time.Time `json:"last_checked"`
	FailCount    int       `json:"fail_count"`
	SuccessCount int       `json:"success_count"`
	AvgLatencyMs int64     `json:"avg_latency_ms"`
}

// ProxyURL returns the full proxy URL with embedded credentials.
func (c *ProxyConfig) ProxyURL() string {
	if c.Username == "" {
		return c.URL
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return c.URL
	}
	u.User = url.UserPassword(c.Username, c.Password)
	return u.String()
}

// RecordSuccess updates stats after a successful request through this proxy.
func (c *ProxyConfig) RecordSuccess(latencyMs int64) {
	c.SuccessCount++
	c.FailCount = 0
	c.Healthy = true
	c.LastUsed = time.Now()
	if c.AvgLatencyMs == 0 {
		c.AvgLatencyMs = latencyMs
	} else {
		c.AvgLatencyMs = (c.AvgLatencyMs*3 + latencyMs) / 4 // exponential moving average
	}
}

// RecordFailure updates stats after a failed request through this proxy.
func (c *ProxyConfig) RecordFailure() {
	c.FailCount++
	c.LastUsed = time.Now()
	if c.FailCount >= 3 {
		c.Healthy = false
	}
}

// HTTPClientConfig configures HTTP client behavior for crawling.
type HTTPClientConfig struct {
	Timeout         time.Duration     `json:"timeout"`
	MaxRedirects    int               `json:"max_redirects"`
	FollowRedirects bool              `json:"follow_redirects"`
	DefaultHeaders  map[string]string `json:"default_headers,omitempty"`
	AcceptEncodings []string          `json:"accept_encodings,omitempty"`
	MaxResponseSize int64             `json:"max_response_size"`
	TLSSkipVerify   bool              `json:"tls_skip_verify"`
}

// DefaultHTTPClientConfig returns sensible defaults.
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		Timeout:         30 * time.Second,
		MaxRedirects:    10,
		FollowRedirects: true,
		DefaultHeaders: map[string]string{
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "en-US,en;q=0.5",
		},
		AcceptEncodings: []string{"gzip", "deflate"},
		MaxResponseSize: 10 * 1024 * 1024, // 10MB
	}
}

// BuildTransport creates an http.Transport with proxy and TLS settings.
func (c *HTTPClientConfig) BuildTransport(proxyURL string) *http.Transport {
	t := &http.Transport{}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
		}
	}
	if c.TLSSkipVerify {
		// Only for testing/debugging — not recommended for production.
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return t
}
