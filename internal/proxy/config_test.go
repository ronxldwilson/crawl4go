package proxy

import (
	"strings"
	"testing"
	"time"
)

// ---- ProxyConfig.ProxyURL ---------------------------------------------------

func TestProxyConfig_ProxyURL_NoCredentials(t *testing.T) {
	pc := &ProxyConfig{URL: "http://proxy.example.com:8080"}
	got := pc.ProxyURL()
	if got != "http://proxy.example.com:8080" {
		t.Errorf("got %q, want %q", got, "http://proxy.example.com:8080")
	}
}

func TestProxyConfig_ProxyURL_WithCredentials(t *testing.T) {
	pc := &ProxyConfig{
		URL:      "http://proxy.example.com:8080",
		Username: "user",
		Password: "pass",
	}
	got := pc.ProxyURL()
	if !strings.Contains(got, "user:pass@") {
		t.Errorf("expected credentials in URL, got %q", got)
	}
	if !strings.Contains(got, "proxy.example.com:8080") {
		t.Errorf("expected host in URL, got %q", got)
	}
}

func TestProxyConfig_ProxyURL_UsernameOnly(t *testing.T) {
	pc := &ProxyConfig{
		URL:      "http://proxy.example.com:3128",
		Username: "onlyuser",
		Password: "",
	}
	got := pc.ProxyURL()
	if !strings.Contains(got, "onlyuser") {
		t.Errorf("expected username in URL, got %q", got)
	}
}

func TestProxyConfig_ProxyURL_InvalidURL(t *testing.T) {
	pc := &ProxyConfig{
		URL:      "://bad-url",
		Username: "user",
		Password: "pass",
	}
	// url.Parse("://bad-url") fails; ProxyURL should fall back to the raw URL.
	got := pc.ProxyURL()
	if got != "://bad-url" {
		t.Errorf("expected raw URL fallback, got %q", got)
	}
}

// ---- ProxyConfig.RecordSuccess ----------------------------------------------

func TestProxyConfig_RecordSuccess_Basic(t *testing.T) {
	pc := &ProxyConfig{Healthy: false, FailCount: 5}
	before := time.Now()
	pc.RecordSuccess(100)
	after := time.Now()

	if pc.SuccessCount != 1 {
		t.Errorf("SuccessCount: got %d, want 1", pc.SuccessCount)
	}
	if pc.FailCount != 0 {
		t.Errorf("FailCount should be reset to 0, got %d", pc.FailCount)
	}
	if !pc.Healthy {
		t.Error("Healthy should be true after success")
	}
	if pc.LastUsed.Before(before) || pc.LastUsed.After(after) {
		t.Error("LastUsed should be within the test window")
	}
	if pc.AvgLatencyMs != 100 {
		t.Errorf("AvgLatencyMs: got %d, want 100 (first sample)", pc.AvgLatencyMs)
	}
}

func TestProxyConfig_RecordSuccess_LatencyEMA(t *testing.T) {
	pc := &ProxyConfig{AvgLatencyMs: 100}
	pc.RecordSuccess(200)
	// EMA formula: (100*3 + 200) / 4 = 125
	if pc.AvgLatencyMs != 125 {
		t.Errorf("AvgLatencyMs after EMA: got %d, want 125", pc.AvgLatencyMs)
	}
}

func TestProxyConfig_RecordSuccess_MultipleIncrements(t *testing.T) {
	pc := &ProxyConfig{}
	pc.RecordSuccess(50)
	pc.RecordSuccess(50)
	if pc.SuccessCount != 2 {
		t.Errorf("SuccessCount: got %d, want 2", pc.SuccessCount)
	}
}

// ---- ProxyConfig.RecordFailure ----------------------------------------------

func TestProxyConfig_RecordFailure_IncrementCount(t *testing.T) {
	pc := &ProxyConfig{Healthy: true}
	pc.RecordFailure()
	if pc.FailCount != 1 {
		t.Errorf("FailCount: got %d, want 1", pc.FailCount)
	}
	// One failure — still healthy.
	if !pc.Healthy {
		t.Error("should still be healthy after 1 failure")
	}
}

func TestProxyConfig_RecordFailure_MarkUnhealthyAtThree(t *testing.T) {
	pc := &ProxyConfig{Healthy: true}
	pc.RecordFailure()
	pc.RecordFailure()
	if !pc.Healthy {
		t.Error("should still be healthy after 2 failures")
	}
	pc.RecordFailure()
	if pc.Healthy {
		t.Error("should be unhealthy after 3 failures")
	}
	if pc.FailCount != 3 {
		t.Errorf("FailCount: got %d, want 3", pc.FailCount)
	}
}

func TestProxyConfig_RecordFailure_UpdatesLastUsed(t *testing.T) {
	pc := &ProxyConfig{Healthy: true}
	before := time.Now()
	pc.RecordFailure()
	after := time.Now()

	if pc.LastUsed.Before(before) || pc.LastUsed.After(after) {
		t.Error("LastUsed should be within the test window")
	}
}

// ---- HTTPClientConfig.BuildTransport ----------------------------------------

func TestHTTPClientConfig_BuildTransport_NoProxy(t *testing.T) {
	cfg := DefaultHTTPClientConfig()
	tr := cfg.BuildTransport("")
	if tr == nil {
		t.Fatal("BuildTransport returned nil")
	}
	// No proxy URL was given; Proxy field should be nil.
	// (It is set only when proxyURL != "")
	if tr.Proxy != nil {
		t.Error("expected Proxy to be nil when no proxyURL is given")
	}
}

func TestHTTPClientConfig_BuildTransport_WithProxy(t *testing.T) {
	cfg := DefaultHTTPClientConfig()
	tr := cfg.BuildTransport("http://proxy.local:8080")
	if tr == nil {
		t.Fatal("BuildTransport returned nil")
	}
	if tr.Proxy == nil {
		t.Error("expected Proxy to be set when proxyURL is provided")
	}
}

func TestHTTPClientConfig_BuildTransport_TLSSkipVerify(t *testing.T) {
	cfg := DefaultHTTPClientConfig()
	cfg.TLSSkipVerify = true
	tr := cfg.BuildTransport("")
	if tr == nil {
		t.Fatal("BuildTransport returned nil")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig to be set when TLSSkipVerify=true")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestHTTPClientConfig_BuildTransport_TLSSkipVerifyFalse(t *testing.T) {
	cfg := DefaultHTTPClientConfig()
	cfg.TLSSkipVerify = false
	tr := cfg.BuildTransport("")
	if tr.TLSClientConfig != nil {
		t.Error("TLSClientConfig should be nil when TLSSkipVerify=false")
	}
}

// ---- DefaultHTTPClientConfig ------------------------------------------------

func TestDefaultHTTPClientConfig(t *testing.T) {
	cfg := DefaultHTTPClientConfig()

	tests := []struct {
		name string
		ok   bool
	}{
		{"Timeout=30s", cfg.Timeout == 30*time.Second},
		{"MaxRedirects=10", cfg.MaxRedirects == 10},
		{"FollowRedirects=true", cfg.FollowRedirects},
		{"MaxResponseSize=10MB", cfg.MaxResponseSize == 10*1024*1024},
		{"TLSSkipVerify=false", !cfg.TLSSkipVerify},
		{"Accept header set", cfg.DefaultHeaders["Accept"] != ""},
		{"Accept-Language header set", cfg.DefaultHeaders["Accept-Language"] != ""},
		{"gzip encoding present", len(cfg.AcceptEncodings) > 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.ok {
				t.Errorf("condition failed: %s", tc.name)
			}
		})
	}
}
