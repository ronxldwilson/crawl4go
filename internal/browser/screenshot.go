package browser

import (
	"context"
	"encoding/json"
	"fmt"

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

	waitForPageReady(sess, waitMs)

	injectBrowserScripts(sess.sendCmd, sid)

	params := map[string]any{
		"format": "png",
	}

	if fullPage {
		metricsResult, err := sess.sendCmd("Page.getLayoutMetrics", nil, sid)
		if err == nil {
			var metrics struct {
				ContentSize struct {
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"contentSize"`
			}
			if json.Unmarshal(metricsResult, &metrics) == nil && metrics.ContentSize.Width > 0 {
				params["clip"] = map[string]any{
					"x": 0, "y": 0,
					"width":  metrics.ContentSize.Width,
					"height": metrics.ContentSize.Height,
					"scale":  1,
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
