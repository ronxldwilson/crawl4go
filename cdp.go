package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe = regexp.MustCompile(`\s+`)
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style|noscript)[^>]*>.*?</\1>`)
)

type CDPClient struct {
	zenPandaURL string
	httpClient  *http.Client
	sem         chan struct{}
}

func NewCDPClient(zenPandaURL string, maxConcurrent int) *CDPClient {
	return &CDPClient{
		zenPandaURL: zenPandaURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		sem:         make(chan struct{}, maxConcurrent),
	}
}

type cdpMessage struct {
	ID        int             `json:"id"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *CDPClient) browserWSURL() string {
	u := strings.TrimPrefix(c.zenPandaURL, "http://")
	u = strings.TrimPrefix(u, "https://")
	return "ws://" + u + "/"
}

func (c *CDPClient) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *CDPClient) release() {
	<-c.sem
}

// FetchHTML navigates to a URL via CDP and returns the rendered HTML.
func (c *CDPClient) FetchHTML(ctx context.Context, targetURL string, waitMs int, scroll bool, maxScrollSteps int) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
	if err != nil {
		return "", fmt.Errorf("ws connect: %w", err)
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
		return "", fmt.Errorf("create target: %w", err)
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	json.Unmarshal(createResult, &created)
	targetID := created.TargetID

	defer sendCmd("Target.closeTarget", map[string]string{"targetId": targetID}, "")

	attachResult, err := sendCmd("Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, "")
	if err != nil {
		return "", fmt.Errorf("attach target: %w", err)
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(attachResult, &attached)
	sid := attached.SessionID

	if _, err := sendCmd("Page.navigate", map[string]string{"url": targetURL}, sid); err != nil {
		return "", fmt.Errorf("navigate: %w", err)
	}

	select {
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	injectBrowserScripts(sendCmd, sid)

	if scroll {
		scrollPage(sendCmd, sid, maxScrollSteps)
	}

	result, err := sendCmd("Runtime.evaluate", map[string]any{
		"expression":    "document.documentElement ? document.documentElement.outerHTML : ''",
		"returnByValue": true,
	}, sid)
	if err != nil {
		return "", fmt.Errorf("evaluate: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evalResult); err != nil {
		return "", err
	}

	return evalResult.Result.Value, nil
}

// httpFetchHTML fetches a page via plain HTTP and returns the raw HTML.
func httpFetchHTML(ctx context.Context, client *http.Client, pageURL string, proxyURL string) (string, int, error) {
	transport := http.DefaultTransport
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err == nil {
			transport = &http.Transport{Proxy: http.ProxyURL(proxy)}
		}
	}
	c := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", 0, err
	}
	ua := RandomUA()
	req.Header.Set("User-Agent", ua.UserAgent)
	if ua.SecCHUA != "" {
		req.Header.Set("Sec-CH-UA", ua.SecCHUA)
		req.Header.Set("Sec-CH-UA-Platform", ua.SecCHUAPlat)
		req.Header.Set("Sec-CH-UA-Mobile", "?0")
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := c.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return "", resp.StatusCode, err
	}

	return string(body), resp.StatusCode, nil
}

// RenderPage races HTTP fetch against CDP render and returns the best result.
func RenderPage(ctx context.Context, cdpClient *CDPClient, httpClient *http.Client, pageURL string, waitMs int, scroll bool, maxScrollSteps int, proxyURL string) (html string, statusCode int, source string, err error) {
	type result struct {
		html       string
		statusCode int
		source     string
		err        error
	}

	ch := make(chan result, 2)

	// HTTP fetch
	go func() {
		h, code, e := httpFetchHTML(ctx, httpClient, pageURL, proxyURL)
		if e != nil || len(strings.TrimSpace(h)) < 200 {
			ch <- result{err: fmt.Errorf("http fetch insufficient")}
			return
		}
		if len(h) > 500_000 {
			h = h[:500_000]
		}
		ch <- result{html: h, statusCode: code, source: "fetch"}
	}()

	// CDP render
	go func() {
		h, e := cdpClient.FetchHTML(ctx, pageURL, waitMs, scroll, maxScrollSteps)
		if e != nil || len(strings.TrimSpace(h)) < 100 {
			ch <- result{err: fmt.Errorf("cdp render insufficient")}
			return
		}
		if len(h) > 500_000 {
			h = h[:500_000]
		}
		ch <- result{html: h, statusCode: 200, source: "cdp"}
	}()

	var best result
	for received := 0; received < 2; received++ {
		select {
		case r := <-ch:
			if r.err != nil {
				continue
			}
			if best.html == "" {
				best = r
				// If HTTP fetch won with enough content, use it immediately
				if r.source == "fetch" && len(r.html) >= 1000 {
					return best.html, best.statusCode, best.source, nil
				}
			} else if r.source == "cdp" && len(r.html) > len(best.html) {
				best = r
			}
		case <-ctx.Done():
			if best.html != "" {
				return best.html, best.statusCode, best.source, nil
			}
			return "", 0, "", ctx.Err()
		}
	}

	if best.html == "" {
		return "", 0, "", fmt.Errorf("no usable content from fetch or render")
	}
	return best.html, best.statusCode, best.source, nil
}

// HTMLToText strips HTML tags and normalizes whitespace.
func HTMLToText(htmlContent string) string {
	text := scriptStyleRe.ReplaceAllString(htmlContent, " ")
	text = htmlTagRe.ReplaceAllString(text, " ")
	text = whitespaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// Healthy checks if ZenPanda is reachable.
func (c *CDPClient) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.zenPandaURL+"/json/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("zenpanda unhealthy", "status", resp.StatusCode)
		return false
	}
	return true
}
