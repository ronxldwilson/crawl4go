package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func (c *CDPClient) CaptureScreenshot(ctx context.Context, targetURL string, waitMs int, fullPage bool) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
	if err != nil {
		return "", fmt.Errorf("ws connect: %w", err)
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

	createResult, err := sendCmd("Target.createTarget", map[string]string{"url": "about:blank"}, "")
	if err != nil {
		return "", fmt.Errorf("create target: %w", err)
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	json.Unmarshal(createResult, &created)
	targetID := created.TargetID
	defer sendCmd("Target.closeTarget", map[string]string{"targetId": targetID}, "")

	attachResult, err := sendCmd("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if err != nil {
		return "", fmt.Errorf("attach target: %w", err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(attachResult, &attached)
	sid := attached.SessionID

	if _, err := sendCmd("Page.navigate", map[string]string{"url": targetURL}, sid); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}

	select {
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	injectBrowserScripts(sendCmd, sid)

	params := map[string]any{
		"format":  "png",
		"quality": 80,
	}

	if fullPage {
		result, err := sendCmd("Runtime.evaluate", map[string]any{
			"expression":    "[document.documentElement.scrollWidth, document.documentElement.scrollHeight, window.devicePixelRatio || 1]",
			"returnByValue": true,
		}, sid)
		if err == nil {
			var evalResult struct {
				Result struct {
					Value []float64 `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(result, &evalResult) == nil && len(evalResult.Result.Value) == 3 {
				w := evalResult.Result.Value[0]
				h := evalResult.Result.Value[1]
				params["clip"] = map[string]any{
					"x": 0, "y": 0, "width": w, "height": h, "scale": 1,
				}
			}
		}
	}

	result, err := sendCmd("Page.captureScreenshot", params, sid)
	if err != nil {
		return "", fmt.Errorf("screenshot: %w", err)
	}

	var screenshotResult struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(result, &screenshotResult); err != nil {
		return "", err
	}

	return screenshotResult.Data, nil
}
