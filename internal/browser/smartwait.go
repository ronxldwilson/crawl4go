package browser

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// smartWait waits for a CSS selector to appear in the page, falling back to
// JS evaluation if the selector syntax is invalid CSS. Returns true if the
// element was found within the timeout.
func smartWait(sendCmd sendCmdFunc, sessionID string, selectorOrJS string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		found, err := checkSelector(sendCmd, sessionID, selectorOrJS)
		if err != nil {
			// CSS selector failed — try as JS expression fallback (#152)
			found, _ = checkJS(sendCmd, sessionID, selectorOrJS)
		}
		if found {
			return true
		}
		time.Sleep(pollInterval)
	}
	return false
}

func checkSelector(sendCmd sendCmdFunc, sessionID string, selector string) (bool, error) {
	// Escape the selector for JS string
	escaped := strings.ReplaceAll(selector, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)

	result, err := sendCmd("Runtime.evaluate", map[string]any{
		"expression":    fmt.Sprintf("!!document.querySelector('%s')", escaped),
		"returnByValue": true,
	}, sessionID)
	if err != nil {
		return false, err
	}
	var evalResult struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct{} `json:"exceptionDetails"`
	}
	if json.Unmarshal(result, &evalResult) != nil {
		return false, fmt.Errorf("unmarshal failed")
	}
	if evalResult.ExceptionDetails != nil {
		return false, fmt.Errorf("selector error")
	}
	return evalResult.Result.Value, nil
}

func checkJS(sendCmd sendCmdFunc, sessionID string, expression string) (bool, error) {
	result, err := sendCmd("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	}, sessionID)
	if err != nil {
		return false, err
	}
	var evalResult struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(result, &evalResult) != nil {
		return false, fmt.Errorf("unmarshal failed")
	}
	// Truthy check
	switch v := evalResult.Result.Value.(type) {
	case bool:
		return v, nil
	case float64:
		return v != 0, nil
	case string:
		return v != "", nil
	case nil:
		return false, nil
	default:
		return true, nil
	}
}
