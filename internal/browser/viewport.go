package browser

import (
	"encoding/json"
)

// adjustViewport resizes the viewport to fit the page content dimensions,
// ensuring responsive layouts render at their true size for screenshot and
// content extraction. This is a best-effort operation.
func adjustViewport(sendCmd sendCmdFunc, sessionID string) {
	// Get content dimensions via Page.getLayoutMetrics
	result, err := sendCmd("Page.getLayoutMetrics", nil, sessionID)
	if err != nil {
		return
	}
	var metrics struct {
		ContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"contentSize"`
	}
	if json.Unmarshal(result, &metrics) != nil || metrics.ContentSize.Width <= 0 {
		return
	}
	// Clamp to reasonable maximums
	w := int(metrics.ContentSize.Width)
	h := int(metrics.ContentSize.Height)
	if w > 1920 {
		w = 1920
	}
	if h > 16384 {
		h = 16384
	}
	if w < 800 {
		w = 800
	}

	sendCmd("Emulation.setDeviceMetricsOverride", map[string]any{
		"width":             w,
		"height":            h,
		"deviceScaleFactor": 1,
		"mobile":            false,
	}, sessionID)
}
