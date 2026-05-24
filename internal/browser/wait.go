package browser

import (
	"encoding/json"
	"time"
)

// waitForPageReady enables CDP lifecycle events and waits for the networkIdle
// signal, falling back to a simple timer if the event doesn't fire within
// waitMs milliseconds. This is strictly best-effort: errors from enabling
// lifecycle events or from the wait itself are silently ignored so that callers
// always get at least the timer-based wait they had before.
func waitForPageReady(sess *cdpSession, waitMs int) {
	if waitMs <= 0 {
		return
	}

	timeout := time.Duration(waitMs) * time.Millisecond

	// Enable Page lifecycle events so the browser emits Page.lifecycleEvent.
	// Ignore the error — if the command fails (e.g. the browser doesn't
	// support it), we fall through to the timer below.
	sess.sendCmd("Page.setLifecycleEventsEnabled", map[string]any{"enabled": true}, sess.sessionID)

	// Register a handler that only fires for the "networkIdle" lifecycle
	// event and wait up to the full timeout for it.
	ch := make(chan struct{}, 1)
	sess.onEvent("Page.lifecycleEvent", func(params json.RawMessage) {
		var ev struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(params, &ev) == nil && ev.Name == "networkIdle" {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	})

	select {
	case <-ch:
		// networkIdle fired — page is ready.
	case <-time.After(timeout):
		// Timeout reached — fall back gracefully; the page had waitMs to settle.
	}
}
