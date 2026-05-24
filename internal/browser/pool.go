package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// cdpPool maintains a single persistent WebSocket connection to a CDP endpoint
// (e.g. ZenPanda) and multiplexes multiple sessions over it via BrowserContexts.
//
// Each call to getSession creates an isolated BrowserContext (#160) with its own
// cookies/storage, then creates and attaches a target within that context. When
// the returned session is closed, both the target and the BrowserContext are
// disposed.
//
// The pool reconnects transparently if the underlying connection drops.
type cdpPool struct {
	wsURL string

	mu   sync.Mutex
	conn *websocket.Conn

	// writeMu serialises WebSocket writes across all sessions sharing this
	// connection. Reads are handled by a dedicated goroutine per connection.
	writeMu sync.Mutex

	msgID atomic.Int64

	// pending tracks in-flight command responses keyed by message ID.
	pendingMu sync.Mutex
	pending   map[int]chan cdpMessage

	// ctx/cancel govern the lifetime of the current connection's reader goroutine.
	ctx    context.Context
	cancel context.CancelFunc
}

// newCDPPool creates a pool for the given WebSocket URL. It does NOT connect
// eagerly; the first call to getSession will establish the connection.
func newCDPPool(wsURL string) *cdpPool {
	return &cdpPool{
		wsURL:   wsURL,
		pending: make(map[int]chan cdpMessage),
	}
}

// ensureConnected dials the WebSocket if no live connection exists.
// Caller must hold p.mu.
func (p *cdpPool) ensureConnected(ctx context.Context) error {
	if p.conn != nil {
		// Ping to verify liveness; if it fails we reconnect below.
		p.writeMu.Lock()
		err := p.conn.WriteMessage(websocket.PingMessage, nil)
		p.writeMu.Unlock()
		if err == nil {
			return nil
		}
		// Connection dead — tear down and reconnect.
		p.teardownLocked()
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, p.wsURL, nil)
	if err != nil {
		return fmt.Errorf("cdpPool dial: %w", err)
	}

	p.conn = conn
	p.pending = make(map[int]chan cdpMessage)

	rctx, cancel := context.WithCancel(context.Background())
	p.ctx = rctx
	p.cancel = cancel

	// Background reader: routes responses to pending callers. Events on the
	// browser-level connection (no sessionID) are currently discarded because
	// the pool only uses the connection for setup commands; session-scoped
	// events are handled by cdpSession's own reader once the session is wired.
	//
	// NOTE: This pool uses a simpler inline-read pattern for setup commands
	// rather than the full event dispatcher planned in #139.
	go p.readLoop(rctx, conn)

	return nil
}

// readLoop reads CDP messages from conn and dispatches responses.
func (p *cdpPool) readLoop(_ context.Context, conn *websocket.Conn) {
	for {
		var msg cdpMessage
		if err := conn.ReadJSON(&msg); err != nil {
			// Connection lost — wake all pending callers so they fail fast.
			p.pendingMu.Lock()
			for id, ch := range p.pending {
				ch <- cdpMessage{Error: &cdpError{Code: -1, Message: "pool connection lost"}}
				delete(p.pending, id)
			}
			p.pendingMu.Unlock()
			return
		}

		if msg.ID != 0 {
			p.pendingMu.Lock()
			ch, ok := p.pending[msg.ID]
			if ok {
				delete(p.pending, msg.ID)
			}
			p.pendingMu.Unlock()
			if ok {
				ch <- msg
			}
		}
		// Non-response messages (events) on the top-level connection are
		// intentionally ignored; per-session events go through the session's
		// own reader once the caller attaches via Target.attachToTarget with
		// flatten:true.
	}
}

// sendCmd sends a CDP command over the shared connection and waits for the
// response. It is safe for concurrent use.
func (p *cdpPool) sendCmd(ctx context.Context, method string, params any, sessionID string) (json.RawMessage, error) {
	id := int(p.msgID.Add(1))
	raw, _ := json.Marshal(params)
	msg := cdpMessage{ID: id, Method: method, Params: raw, SessionID: sessionID}

	ch := make(chan cdpMessage, 1)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()

	p.writeMu.Lock()
	err := p.conn.WriteJSON(msg)
	p.writeMu.Unlock()
	if err != nil {
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return nil, fmt.Errorf("pool write: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("cdp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// teardownLocked closes the current connection and cancels its reader.
// Caller must hold p.mu.
func (p *cdpPool) teardownLocked() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
}

// getSession creates an isolated BrowserContext (#160), creates a target within
// it, attaches to the target, and returns a fully wired cdpSession.
//
// The returned session's close() disposes both the target and the
// BrowserContext so cookies/storage never leak between crawl jobs.
//
// If the underlying connection is dead, getSession reconnects transparently.
func (p *cdpPool) getSession(ctx context.Context) (*cdpSession, error) {
	p.mu.Lock()
	if err := p.ensureConnected(ctx); err != nil {
		p.mu.Unlock()
		return nil, err
	}
	p.mu.Unlock()

	// 1. Create an isolated BrowserContext (cookie + storage isolation).
	bcResult, err := p.sendCmd(ctx, "Target.createBrowserContext", map[string]any{
		"disposeOnDetach": true,
	}, "")
	if err != nil {
		// Connection may have died between ensureConnected and sendCmd.
		// Try once more with a fresh connection.
		p.mu.Lock()
		p.teardownLocked()
		if err2 := p.ensureConnected(ctx); err2 != nil {
			p.mu.Unlock()
			return nil, fmt.Errorf("reconnect after createBrowserContext failure: %w", err2)
		}
		p.mu.Unlock()

		bcResult, err = p.sendCmd(ctx, "Target.createBrowserContext", map[string]any{
			"disposeOnDetach": true,
		}, "")
		if err != nil {
			return nil, fmt.Errorf("create browser context: %w", err)
		}
	}

	var bc struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(bcResult, &bc); err != nil {
		return nil, fmt.Errorf("unmarshal browser context: %w", err)
	}

	// 2. Create a target inside the BrowserContext.
	createResult, err := p.sendCmd(ctx, "Target.createTarget", map[string]any{
		"url":              "about:blank",
		"browserContextId": bc.BrowserContextID,
	}, "")
	if err != nil {
		// Best-effort cleanup.
		p.sendCmd(ctx, "Target.disposeBrowserContext", map[string]string{
			"browserContextId": bc.BrowserContextID,
		}, "")
		return nil, fmt.Errorf("create target: %w", err)
	}

	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(createResult, &created); err != nil {
		p.sendCmd(ctx, "Target.disposeBrowserContext", map[string]string{
			"browserContextId": bc.BrowserContextID,
		}, "")
		return nil, fmt.Errorf("unmarshal target: %w", err)
	}

	// 3. Attach to the target with flatten:true so CDP routes session messages
	//    over the same WebSocket connection using a sessionId field.
	attachResult, err := p.sendCmd(ctx, "Target.attachToTarget", map[string]any{
		"targetId": created.TargetID,
		"flatten":  true,
	}, "")
	if err != nil {
		p.sendCmd(ctx, "Target.closeTarget", map[string]string{"targetId": created.TargetID}, "")
		p.sendCmd(ctx, "Target.disposeBrowserContext", map[string]string{
			"browserContextId": bc.BrowserContextID,
		}, "")
		return nil, fmt.Errorf("attach target: %w", err)
	}

	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachResult, &attached); err != nil {
		p.sendCmd(ctx, "Target.closeTarget", map[string]string{"targetId": created.TargetID}, "")
		p.sendCmd(ctx, "Target.disposeBrowserContext", map[string]string{
			"browserContextId": bc.BrowserContextID,
		}, "")
		return nil, fmt.Errorf("unmarshal attach: %w", err)
	}

	// 4. Build a cdpSession that reuses the pool's shared connection.
	//    The session gets its own sendCmd that scopes commands to its sessionID
	//    and serialises writes via the pool's writeMu.
	sess := &cdpSession{
		conn:      p.conn,
		targetID:  created.TargetID,
		sessionID: attached.SessionID,
		pending:   make(map[int]chan cdpMessage),
		handlers:  make(map[string][]func(json.RawMessage)),
	}

	// Wire sendCmd through the pool's shared connection. The pool's readLoop
	// only handles top-level (no sessionID) responses. For session-scoped
	// messages, the session needs its own pending map — but since flatten:true
	// means ALL messages go over the same WebSocket, we route through the
	// pool's pending map which is already keyed by unique msgID.
	sessSendCmd := sendCmdFunc(func(method string, params any, sessionID string) (json.RawMessage, error) {
		return p.sendCmd(ctx, method, params, sessionID)
	})
	sess.sendCmd = sessSendCmd

	// Override close to dispose BrowserContext for full isolation cleanup.
	browserContextID := bc.BrowserContextID
	sess.closeFn = func() {
		// Close the target.
		p.sendCmd(context.Background(), "Target.closeTarget", map[string]string{
			"targetId": created.TargetID,
		}, "")
		// Dispose the BrowserContext so cookies/storage are garbage-collected.
		p.sendCmd(context.Background(), "Target.disposeBrowserContext", map[string]string{
			"browserContextId": browserContextID,
		}, "")
		// Do NOT close the WebSocket — it's shared by the pool.
	}

	return sess, nil
}

// close tears down the persistent connection. After close, getSession must not
// be called.
func (p *cdpPool) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.teardownLocked()
}
