package browser

import (
	"encoding/json"
	"strings"
	"time"
)

// evaluateWithRecovery executes a JS expression, retrying up to maxRetries
// times if the execution context was destroyed (common after navigations).
func evaluateWithRecovery(sendCmd sendCmdFunc, sessionID string, expression string, returnByValue bool, maxRetries int) (json.RawMessage, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := sendCmd("Runtime.evaluate", map[string]any{
			"expression":    expression,
			"returnByValue": returnByValue,
		}, sessionID)
		if err == nil {
			return result, nil
		}
		lastErr = err
		// Only retry on execution context destroyed errors
		if !strings.Contains(err.Error(), "Cannot find context") &&
			!strings.Contains(err.Error(), "Execution context was destroyed") &&
			!strings.Contains(err.Error(), "context was destroyed") {
			return nil, err
		}
		if attempt < maxRetries {
			time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
		}
	}
	return nil, lastErr
}
