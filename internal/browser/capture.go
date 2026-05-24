package browser

import (
	"encoding/json"
	"sync"
)

// NetworkRequest holds metadata about a single network request observed during
// a page crawl.
type NetworkRequest struct {
	URL      string `json:"url"`
	Method   string `json:"method"`
	Status   int    `json:"status"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// ConsoleMessage holds a single console API call captured from the page.
type ConsoleMessage struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// PageCapture accumulates network requests and console messages that arrive
// while a page is being crawled.
type PageCapture struct {
	mu       sync.Mutex
	Requests []NetworkRequest `json:"requests"`
	Console  []ConsoleMessage `json:"console"`
}

// enableCapture subscribes to Network and Console CDP events on the session.
// Call this BEFORE navigation. Returns a *PageCapture that accumulates data
// as events arrive.
func enableCapture(sess *cdpSession) *PageCapture {
	cap := &PageCapture{}

	// Enable domains
	sess.sendCmd("Network.enable", nil, sess.sessionID)
	sess.sendCmd("Runtime.enable", nil, sess.sessionID)

	// Track requests
	requests := make(map[string]*NetworkRequest) // keyed by requestId
	var reqMu sync.Mutex

	sess.onEvent("Network.requestWillBeSent", func(params json.RawMessage) {
		var ev struct {
			RequestID string `json:"requestId"`
			Request   struct {
				URL    string `json:"url"`
				Method string `json:"method"`
			} `json:"request"`
		}
		if json.Unmarshal(params, &ev) == nil {
			reqMu.Lock()
			requests[ev.RequestID] = &NetworkRequest{
				URL:    ev.Request.URL,
				Method: ev.Request.Method,
			}
			reqMu.Unlock()
		}
	})

	sess.onEvent("Network.responseReceived", func(params json.RawMessage) {
		var ev struct {
			RequestID string `json:"requestId"`
			Response  struct {
				Status   int    `json:"status"`
				MimeType string `json:"mimeType"`
			} `json:"response"`
		}
		if json.Unmarshal(params, &ev) == nil {
			reqMu.Lock()
			if r, ok := requests[ev.RequestID]; ok {
				r.Status = ev.Response.Status
				r.MimeType = ev.Response.MimeType
			}
			reqMu.Unlock()
		}
	})

	sess.onEvent("Network.loadingFinished", func(params json.RawMessage) {
		var ev struct {
			RequestID         string  `json:"requestId"`
			EncodedDataLength float64 `json:"encodedDataLength"`
		}
		if json.Unmarshal(params, &ev) == nil {
			reqMu.Lock()
			if r, ok := requests[ev.RequestID]; ok {
				r.Size = int64(ev.EncodedDataLength)
				cap.mu.Lock()
				cap.Requests = append(cap.Requests, *r)
				cap.mu.Unlock()
				delete(requests, ev.RequestID)
			}
			reqMu.Unlock()
		}
	})

	// Console messages via Runtime.consoleAPICalled
	sess.onEvent("Runtime.consoleAPICalled", func(params json.RawMessage) {
		var ev struct {
			Type string `json:"type"`
			Args []struct {
				Value json.RawMessage `json:"value"`
			} `json:"args"`
		}
		if json.Unmarshal(params, &ev) == nil {
			text := ""
			for _, arg := range ev.Args {
				var s string
				if json.Unmarshal(arg.Value, &s) == nil {
					if text != "" {
						text += " "
					}
					text += s
				}
			}
			if text != "" {
				cap.mu.Lock()
				cap.Console = append(cap.Console, ConsoleMessage{Level: ev.Type, Text: text})
				cap.mu.Unlock()
			}
		}
	})

	return cap
}
