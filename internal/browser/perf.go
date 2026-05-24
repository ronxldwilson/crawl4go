package browser

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ronxldwilson/crawl4go/internal/ua"
)

// PerfMetrics holds page performance timing data collected via CDP.
type PerfMetrics struct {
	TTFB             int64 `json:"ttfb_ms"`
	DOMContentLoaded int64 `json:"dom_content_loaded_ms"`
	LoadComplete     int64 `json:"load_complete_ms"`
	DOMNodes         int   `json:"dom_nodes"`
	ResourceCount    int   `json:"resource_count"`
	TransferSize     int64 `json:"transfer_size_bytes"`
	JSHeapSize       int64 `json:"js_heap_size_bytes"`
}

// CollectMetrics navigates to targetURL, waits waitMs milliseconds, then
// evaluates JavaScript to gather performance timing, resource counts, DOM
// node count, and JS heap size.
func (c *CDPClient) CollectMetrics(ctx context.Context, targetURL string, waitMs int) (*PerfMetrics, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	sess, err := openSession(ctx, c.browserWSURL())
	if err != nil {
		return nil, err
	}
	defer sess.close()

	sid := sess.sessionID

	configureStealthSession(sess.sendCmd, sid, ua.RandomUA().UserAgent)

	if _, err := sess.sendCmd("Page.navigate", map[string]string{"url": targetURL}, sid); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	waitForPageReady(sess, waitMs)

	// Single JS expression that collects all metrics at once.
	const metricsJS = `(function() {
		var t = performance.timing;
		var navStart = t.navigationStart || 0;
		var ttfb = (t.responseStart && navStart) ? t.responseStart - navStart : 0;
		var domContentLoaded = (t.domContentLoadedEventEnd && navStart) ? t.domContentLoadedEventEnd - navStart : 0;
		var loadComplete = (t.loadEventEnd && navStart) ? t.loadEventEnd - navStart : 0;

		var domNodes = document.querySelectorAll('*').length;

		var resources = performance.getEntriesByType('resource');
		var resourceCount = resources.length;
		var transferSize = 0;
		for (var i = 0; i < resources.length; i++) {
			transferSize += resources[i].transferSize || 0;
		}

		var jsHeapSize = 0;
		if (performance.memory) {
			jsHeapSize = performance.memory.usedJSHeapSize || 0;
		}

		return {
			ttfb: ttfb,
			domContentLoaded: domContentLoaded,
			loadComplete: loadComplete,
			domNodes: domNodes,
			resourceCount: resourceCount,
			transferSize: transferSize,
			jsHeapSize: jsHeapSize
		};
	})()`

	result, err := sess.sendCmd("Runtime.evaluate", map[string]any{
		"expression":    metricsJS,
		"returnByValue": true,
	}, sid)
	if err != nil {
		return nil, fmt.Errorf("evaluate metrics: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value struct {
				TTFB             int64 `json:"ttfb"`
				DOMContentLoaded int64 `json:"domContentLoaded"`
				LoadComplete     int64 `json:"loadComplete"`
				DOMNodes         int   `json:"domNodes"`
				ResourceCount    int   `json:"resourceCount"`
				TransferSize     int64 `json:"transferSize"`
				JSHeapSize       int64 `json:"jsHeapSize"`
			} `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}

	v := evalResult.Result.Value
	return &PerfMetrics{
		TTFB:             v.TTFB,
		DOMContentLoaded: v.DOMContentLoaded,
		LoadComplete:     v.LoadComplete,
		DOMNodes:         v.DOMNodes,
		ResourceCount:    v.ResourceCount,
		TransferSize:     v.TransferSize,
		JSHeapSize:       v.JSHeapSize,
	}, nil
}
