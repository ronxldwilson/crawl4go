package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ronxldwilson/crawl4go/internal/ua"
)

func (c *CDPClient) CaptureScreenshot(ctx context.Context, targetURL string, waitMs int, fullPage bool) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	sess, err := openSession(ctx, c.browserWSURL())
	if err != nil {
		return "", err
	}
	defer sess.close()

	sid := sess.sessionID

	configureStealthSession(sess.sendCmd, sid, ua.RandomUA().UserAgent)

	if _, err := sess.sendCmd("Page.navigate", map[string]string{"url": targetURL}, sid); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}

	select {
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	injectBrowserScripts(sess.sendCmd, sid)

	params := map[string]any{
		"format":  "png",
		"quality": 80,
	}

	if fullPage {
		result, err := sess.sendCmd("Runtime.evaluate", map[string]any{
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

	result, err := sess.sendCmd("Page.captureScreenshot", params, sid)
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
