package browser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

type sendCmdFunc func(method string, params any, sessionID string) (json.RawMessage, error)

func scrollPage(sendCmd sendCmdFunc, sessionID string, maxSteps int) {
	if maxSteps <= 0 {
		maxSteps = 10
	}

	evalJS := func(expr string) (float64, error) {
		result, err := sendCmd("Runtime.evaluate", map[string]any{
			"expression":    expr,
			"returnByValue": true,
		}, sessionID)
		if err != nil {
			return 0, err
		}
		var r struct {
			Result struct {
				Value float64 `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(result, &r); err != nil {
			return 0, err
		}
		return r.Result.Value, nil
	}

	viewportHeight, err := evalJS("window.innerHeight")
	if err != nil || viewportHeight <= 0 {
		return
	}

	prevHeight := 0.0
	noChangeCount := 0

	for step := 0; step < maxSteps; step++ {
		scrollHeight, err := evalJS("document.body.scrollHeight")
		if err != nil {
			break
		}

		if scrollHeight == prevHeight {
			noChangeCount++
			if noChangeCount >= 2 {
				break
			}
		} else {
			noChangeCount = 0
		}
		prevHeight = scrollHeight

		scrollY := float64(step+1) * viewportHeight
		if scrollY > scrollHeight {
			scrollY = scrollHeight
		}

		_, err = sendCmd("Runtime.evaluate", map[string]any{
			"expression":    fmt.Sprintf("window.scrollTo(0, %f)", scrollY),
			"returnByValue": true,
		}, sessionID)
		if err != nil {
			break
		}

		time.Sleep(300 * time.Millisecond)
	}

	sendCmd("Runtime.evaluate", map[string]any{
		"expression":    "Array.from(document.images).every(img => img.complete)",
		"returnByValue": true,
	}, sessionID)

	sendCmd("Runtime.evaluate", map[string]any{
		"expression":    "window.scrollTo(0, 0)",
		"returnByValue": true,
	}, sessionID)

	slog.Debug("scroll completed", "max_steps", maxSteps)
}
