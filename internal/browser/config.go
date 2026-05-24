package browser

import "time"

// BrowserConfig holds all configuration for a CDP browser session via ZenPanda.
type BrowserConfig struct {
	// Connection
	ZenPandaURL    string        `json:"zenpanda_url"`
	MaxConcurrent  int           `json:"max_concurrent"`
	ConnectTimeout time.Duration `json:"connect_timeout"`

	// Page behavior
	DefaultWaitMs  int           `json:"default_wait_ms"`
	DefaultScroll  bool          `json:"default_scroll"`
	MaxScrollSteps int           `json:"max_scroll_steps"`
	ScrollTimeout  time.Duration `json:"scroll_timeout"`

	// Content extraction
	InjectScripts    bool `json:"inject_scripts"`
	FlattenShadowDOM bool `json:"flatten_shadow_dom"`
	ProcessIframes   bool `json:"process_iframes"`
	RemoveOverlays   bool `json:"remove_overlays"`

	// Viewport
	ViewportWidth  int `json:"viewport_width"`
	ViewportHeight int `json:"viewport_height"`

	// Stealth
	RotateUserAgent bool   `json:"rotate_user_agent"`
	CustomUserAgent string `json:"custom_user_agent"`

	// Network
	ProxyURL    string `json:"proxy_url"`
	BlockImages bool   `json:"block_images"`
	BlockMedia  bool   `json:"block_media"`

	// Capture
	CaptureNetwork bool `json:"capture_network"`
	CaptureConsole bool `json:"capture_console"`

	// Isolation
	UseBrowserContext bool `json:"use_browser_context"`

	// Geolocation (#100)
	Geolocation *GeolocationConfig `json:"geolocation,omitempty"`
}

// GeolocationConfig holds latitude/longitude/accuracy for geolocation overrides.
type GeolocationConfig struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy"`
}

// DefaultBrowserConfig returns a BrowserConfig with sensible defaults for
// ZenPanda-based crawling.
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		ZenPandaURL:       "http://zenpanda:9222",
		MaxConcurrent:     16,
		ConnectTimeout:    10 * time.Second,
		DefaultWaitMs:     3000,
		DefaultScroll:     true,
		MaxScrollSteps:    10,
		ScrollTimeout:     10 * time.Second,
		InjectScripts:     true,
		FlattenShadowDOM:  true,
		ProcessIframes:    true,
		RemoveOverlays:    true,
		ViewportWidth:     1920,
		ViewportHeight:    1080,
		RotateUserAgent:   true,
		UseBrowserContext: true,
	}
}

// applyGeolocation sets the geolocation override on a CDP session if configured.
func applyGeolocation(sendCmd sendCmdFunc, sessionID string, geo *GeolocationConfig) {
	if geo == nil {
		return
	}
	sendCmd("Emulation.setGeolocationOverride", map[string]any{
		"latitude":  geo.Latitude,
		"longitude": geo.Longitude,
		"accuracy":  geo.Accuracy,
	}, sessionID)
}

// applyViewport sets the device metrics override on a CDP session.
func applyConfigViewport(sendCmd sendCmdFunc, sessionID string, width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	sendCmd("Emulation.setDeviceMetricsOverride", map[string]any{
		"width":             width,
		"height":            height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	}, sessionID)
}

// applyNetworkBlocking sets up request interception to block images/media if configured.
func applyNetworkBlocking(sendCmd sendCmdFunc, sessionID string, blockImages, blockMedia bool) {
	if !blockImages && !blockMedia {
		return
	}
	patterns := []map[string]string{}
	if blockImages {
		patterns = append(patterns, map[string]string{"resourceType": "Image"})
	}
	if blockMedia {
		patterns = append(patterns, map[string]string{"resourceType": "Media"})
	}
	sendCmd("Network.setBlockedURLs", nil, sessionID) // clear first
	sendCmd("Fetch.enable", map[string]any{"patterns": patterns}, sessionID)
}
