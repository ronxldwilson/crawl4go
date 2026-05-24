package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// cdpMessage is the wire format for Chrome DevTools Protocol messages.
type cdpMessage struct {
	ID        int             `json:"id"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// cdpError is the error sub-object returned by CDP when a command fails.
type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// cdpSession holds an active CDP connection with a created and attached target.
type cdpSession struct {
	conn      *websocket.Conn
	targetID  string
	sessionID string
	sendCmd   sendCmdFunc
}

// newCDPSession takes ownership of an already-dialled WebSocket connection,
// starts the context-cancellation closer goroutine, builds the sendCmd
// closure, creates a blank browser target, and attaches to it.
//
// The caller must call session.close() when done.
func newCDPSession(ctx context.Context, conn *websocket.Conn) (*cdpSession, error) {
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	var msgID atomic.Int64

	sendCmd := sendCmdFunc(func(method string, params any, sessionID string) (json.RawMessage, error) {
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
	})

	createResult, err := sendCmd("Target.createTarget", map[string]string{"url": "about:blank"}, "")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create target: %w", err)
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	json.Unmarshal(createResult, &created)

	attachResult, err := sendCmd("Target.attachToTarget", map[string]any{"targetId": created.TargetID, "flatten": true}, "")
	if err != nil {
		sendCmd("Target.closeTarget", map[string]string{"targetId": created.TargetID}, "")
		conn.Close()
		return nil, fmt.Errorf("attach target: %w", err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(attachResult, &attached)

	return &cdpSession{
		conn:      conn,
		targetID:  created.TargetID,
		sessionID: attached.SessionID,
		sendCmd:   sendCmd,
	}, nil
}

// openSession dials wsURL, creates a blank browser target, and attaches to it.
// The caller must call session.close() when done.
func openSession(ctx context.Context, wsURL string) (*cdpSession, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ws connect: %w", err)
	}
	return newCDPSession(ctx, conn)
}

// close sends Target.closeTarget and closes the underlying WebSocket connection.
func (s *cdpSession) close() {
	s.sendCmd("Target.closeTarget", map[string]string{"targetId": s.targetID}, "")
	s.conn.Close()
}
