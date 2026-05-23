package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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

	attachResult, err := sendCmd("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if err != nil {
		return nil, fmt.Errorf("attach target: %w", err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(attachResult, &attached)
	sid := attached.SessionID

	if _, err := sendCmd("Page.navigate", map[string]string{"url": targetURL}, sid); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	select {
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	injectBrowserScripts(sendCmd, sid)

	result, err := sendCmd("Runtime.evaluate", map[string]any{
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
