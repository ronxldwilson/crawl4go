package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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

// errSessionClosed is returned when the background reader has stopped.
var errSessionClosed = errors.New("cdp session closed")

// cdpSession holds an active CDP connection with a created and attached target.
type cdpSession struct {
	conn      *websocket.Conn
	targetID  string
	sessionID string
	sendCmd   sendCmdFunc

	writeMu sync.Mutex // serialises conn.WriteJSON calls

	pendingMu sync.Mutex
	pending   map[int]chan cdpMessage

	handlerMu sync.Mutex
	handlers  map[string][]func(json.RawMessage)

	// closeFn, when non-nil, overrides the default close behaviour.
	// Used by cdpPool to dispose BrowserContexts without closing the
	// shared WebSocket connection.
	closeFn func()
}

// onEvent registers a handler that will be called whenever a CDP event with the
// given method name is received. Multiple handlers per method are supported.
func (s *cdpSession) onEvent(method string, handler func(json.RawMessage)) {
	s.handlerMu.Lock()
	defer s.handlerMu.Unlock()
	s.handlers[method] = append(s.handlers[method], handler)
}

// waitForEvent blocks until a CDP event with the given method arrives or the
// timeout elapses. It registers a temporary handler, so callers don't need to
// clean up.
func (s *cdpSession) waitForEvent(method string, timeout time.Duration) (json.RawMessage, error) {
	ch := make(chan json.RawMessage, 1)
	var once sync.Once
	s.onEvent(method, func(params json.RawMessage) {
		once.Do(func() {
			ch <- params
		})
	})
	select {
	case params := <-ch:
		return params, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for event %s", method)
	}
}

// newCDPSession takes ownership of an already-dialled WebSocket connection,
// starts the background reader goroutine, builds the sendCmd closure, creates
// a blank browser target, and attaches to it.
//
// The caller must call session.close() when done.
func newCDPSession(ctx context.Context, conn *websocket.Conn) (*cdpSession, error) {
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	sess := &cdpSession{
		conn:     conn,
		pending:  make(map[int]chan cdpMessage),
		handlers: make(map[string][]func(json.RawMessage)),
	}

	var msgID atomic.Int64

	// Background reader goroutine: reads all messages from the WebSocket and
	// routes them to pending command callers or event handlers.
	go func() {
		for {
			var msg cdpMessage
			if err := conn.ReadJSON(&msg); err != nil {
				// Connection closed or context cancelled — wake all pending callers.
				sess.pendingMu.Lock()
				for id, ch := range sess.pending {
					ch <- cdpMessage{Error: &cdpError{Code: -1, Message: errSessionClosed.Error()}}
					delete(sess.pending, id)
				}
				sess.pendingMu.Unlock()
				return
			}

			if msg.ID != 0 {
				// Response to a command.
				sess.pendingMu.Lock()
				ch, ok := sess.pending[msg.ID]
				if ok {
					delete(sess.pending, msg.ID)
				}
				sess.pendingMu.Unlock()
				if ok {
					ch <- msg
				}
			} else if msg.Method != "" {
				// CDP event notification.
				sess.handlerMu.Lock()
				handlers := make([]func(json.RawMessage), len(sess.handlers[msg.Method]))
				copy(handlers, sess.handlers[msg.Method])
				sess.handlerMu.Unlock()
				for _, h := range handlers {
					h(msg.Params)
				}
			}
		}
	}()

	sendCmd := sendCmdFunc(func(method string, params any, sessionID string) (json.RawMessage, error) {
		id := int(msgID.Add(1))
		p, _ := json.Marshal(params)
		cdpMsg := cdpMessage{ID: id, Method: method, Params: p, SessionID: sessionID}

		// Register the pending channel before writing so the reader can never
		// deliver before we're listening.
		ch := make(chan cdpMessage, 1)
		sess.pendingMu.Lock()
		sess.pending[id] = ch
		sess.pendingMu.Unlock()

		sess.writeMu.Lock()
		err := conn.WriteJSON(cdpMsg)
		sess.writeMu.Unlock()
		if err != nil {
			sess.pendingMu.Lock()
			delete(sess.pending, id)
			sess.pendingMu.Unlock()
			return nil, err
		}

		select {
		case resp := <-ch:
			if resp.Error != nil {
				return nil, fmt.Errorf("cdp error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		case <-ctx.Done():
			sess.pendingMu.Lock()
			delete(sess.pending, id)
			sess.pendingMu.Unlock()
			return nil, ctx.Err()
		}
	})

	sess.sendCmd = sendCmd

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

	sess.targetID = created.TargetID
	sess.sessionID = attached.SessionID

	return sess, nil
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
// If closeFn is set (e.g. by cdpPool), it is called instead so the shared
// connection is not torn down.
func (s *cdpSession) close() {
	if s.closeFn != nil {
		s.closeFn()
		return
	}
	s.sendCmd("Target.closeTarget", map[string]string{"targetId": s.targetID}, "")
	s.conn.Close()
}
