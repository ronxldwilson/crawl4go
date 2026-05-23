package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("ws connect: %w", err)
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	var msgID atomic.Int64

	sendCmd := func(method string, params any, sessionID string) (json.RawMessage, error) {
		id := int(msgID.Add(1))
		p, _ := json.Marshal(params)
		msg := cdpMessage{ID: id, Method: method, Params: p, SessionID: sessionID}
		if err := conn.WriteJSON(msg); err != nil {
			return nil, err
		}
		for {
			var resp cdpMessage
			if err := conn.ReadJSON(&resp); err != nil {
				return nil, err
			}
			if resp.ID == id {
				if resp.Error != nil {
					return nil, fmt.Errorf("cdp error %d: %s", resp.Error.Code, resp.Error.Message)
				}
				return resp.Result, nil
			}
		}
	}

	// Create a new browser target.
	createResult, err := sendCmd("Target.createTarget", map[string]string{"url": "about:blank"}, "")
	if err != nil {
		return nil, fmt.Errorf("create target: %w", err)
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	json.Unmarshal(createResult, &created)
	targetID := created.TargetID
	defer sendCmd("Target.closeTarget", map[string]string{"targetId": targetID}, "")

	// Attach to the target with a flattened session.
	attachResult, err := sendCmd("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if err != nil {
		return nil, fmt.Errorf("attach target: %w", err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(attachResult, &attached)
	sid := attached.SessionID

	// Navigate to the target URL.
	if _, err := sendCmd("Page.navigate", map[string]string{"url": targetURL}, sid); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Wait for the page to settle.
	select {
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

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

	result, err := sendCmd("Runtime.evaluate", map[string]any{
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
