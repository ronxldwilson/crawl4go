package browser

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ronxldwilson/crawl4go/internal/ua"
)

type JSResult struct {
	Value any    `json:"value"`
	Type  string `json:"type"`
}

func (c *CDPClient) ExecuteJS(ctx context.Context, targetURL string, waitMs int, expression string, awaitPromise bool) (*JSResult, error) {
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

	injectBrowserScripts(sess.sendCmd, sid)

	result, err := sess.sendCmd("Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  awaitPromise,
	}, sid)
	if err != nil {
		return nil, fmt.Errorf("evaluate: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value json.RawMessage `json:"value"`
			Type  string          `json:"type"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return nil, err
	}
	if evalResult.ExceptionDetails != nil {
		return nil, fmt.Errorf("js exception: %s", evalResult.ExceptionDetails.Text)
	}

	var value any
	if evalResult.Result.Value != nil {
		json.Unmarshal(evalResult.Result.Value, &value)
	}

	return &JSResult{
		Value: value,
		Type:  evalResult.Result.Type,
	}, nil
}
