package crawl

import (
	"encoding/json"
	"os"
)

// Settings holds user-level configuration loaded from environment or file.
type Settings struct {
	// LLM
	LLMProvider string `json:"llm_provider"`
	LLMModel    string `json:"llm_model"`
	LLMAPIKey   string `json:"llm_api_key"`

	// Browser
	ZenPandaURL   string `json:"zenpanda_url"`
	MaxConcurrent int    `json:"max_concurrent"`
	DefaultWaitMs int    `json:"default_wait_ms"`

	// Proxy
	ProxyURL string `json:"proxy_url"`
	TorProxy string `json:"tor_proxy"`

	// Cache
	CacheDir string `json:"cache_dir"`
	CacheTTL int    `json:"cache_ttl_seconds"`

	// Logging
	LogLevel string `json:"log_level"`
	Verbose  bool   `json:"verbose"`
}

// DefaultSettings returns sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		LLMProvider:   "openai",
		LLMModel:      "gpt-4o-mini",
		ZenPandaURL:   "http://zenpanda:9222",
		MaxConcurrent: 16,
		DefaultWaitMs: 3000,
		CacheDir:      ".crawl4go/cache",
		CacheTTL:      3600,
		LogLevel:      "info",
	}
}

// LoadSettings loads settings from environment variables (CRAWL4GO_ prefix)
// and optionally from a JSON file path. Env vars override file values.
func LoadSettings(filePath string) Settings {
	s := DefaultSettings()

	// Load from file if provided
	if filePath != "" {
		if data, err := os.ReadFile(filePath); err == nil {
			json.Unmarshal(data, &s)
		}
	}

	// Override from env
	envOverride(&s.LLMProvider, "CRAWL4GO_LLM_PROVIDER")
	envOverride(&s.LLMModel, "CRAWL4GO_LLM_MODEL")
	envOverride(&s.LLMAPIKey, "CRAWL4GO_LLM_API_KEY")
	envOverride(&s.ZenPandaURL, "CRAWL4GO_ZENPANDA_URL")
	envOverride(&s.ProxyURL, "CRAWL4GO_PROXY_URL")
	envOverride(&s.TorProxy, "CRAWL4GO_TOR_PROXY")
	envOverride(&s.CacheDir, "CRAWL4GO_CACHE_DIR")
	envOverride(&s.LogLevel, "CRAWL4GO_LOG_LEVEL")

	return s
}

func envOverride(target *string, key string) {
	if v := os.Getenv(key); v != "" {
		*target = v
	}
}
