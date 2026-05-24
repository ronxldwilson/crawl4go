package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ronxldwilson/crawl4go/internal/ua"
)

type CDPClient struct {
	zenPandaURL string
	httpClient  *http.Client
	sem         chan struct{}
}

// ZenPanda's default cdp_max_connections is 16. The semaphore must not
// exceed that or ZenPanda will reject connections with MaxThreadsReached.
const maxZenPandaConnections = 16

func NewCDPClient(zenPandaURL string, maxConcurrent int) *CDPClient {
	if maxConcurrent > maxZenPandaConnections {
		maxConcurrent = maxZenPandaConnections
	}
	return &CDPClient{
		zenPandaURL: zenPandaURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		sem:         make(chan struct{}, maxConcurrent),
	}
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

func (c *CDPClient) FetchHTML(ctx context.Context, targetURL string, waitMs int, scroll bool, maxScrollSteps int) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	// Dial with a retry if ZenPanda is temporarily unavailable.
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
	if err != nil {
		if rerr := c.waitForReady(ctx); rerr != nil {
			return "", fmt.Errorf("ws connect: %w", err)
		}
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
		if err != nil {
			return "", fmt.Errorf("ws connect after recovery: %w", err)
		}
	}

	sess, err := newCDPSession(ctx, conn)
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

	if scroll {
		scrollPage(sess.sendCmd, sid, maxScrollSteps)
	}

	result, err := sess.sendCmd("Runtime.evaluate", map[string]any{
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

// FetchMarkdown navigates to targetURL and returns page content as markdown.
// It first tries ZenPanda's native LP.getMarkdown CDP command which returns
// clean markdown directly from the DOM. If that command is unavailable (e.g.
// non-ZenPanda endpoint) or returns empty content, it falls back to FetchHTML.
func (c *CDPClient) FetchMarkdown(ctx context.Context, targetURL string, waitMs int, scroll bool, maxScrollSteps int) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
	if err != nil {
		if rerr := c.waitForReady(ctx); rerr != nil {
			return "", fmt.Errorf("ws connect: %w", err)
		}
		conn, _, err = websocket.DefaultDialer.DialContext(ctx, c.browserWSURL(), nil)
		if err != nil {
			return "", fmt.Errorf("ws connect after recovery: %w", err)
		}
	}

	sess, err := newCDPSession(ctx, conn)
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

	if scroll {
		scrollPage(sess.sendCmd, sid, maxScrollSteps)
	}

	// Try ZenPanda's native LP.getMarkdown first — no JS eval overhead.
	lpResult, lpErr := sess.sendCmd("LP.getMarkdown", nil, sid)
	if lpErr == nil {
		var md struct {
			Result string `json:"result"`
		}
		if json.Unmarshal(lpResult, &md) == nil && len(md.Result) > 0 {
			return md.Result, nil
		}
	}

	// Fall back to standard outerHTML extraction.
	result, err := sess.sendCmd("Runtime.evaluate", map[string]any{
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

func HTTPFetchHTML(ctx context.Context, client *http.Client, pageURL string, proxyURL string) (string, int, error) {
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
	uaResult := ua.RandomUA()
	req.Header.Set("User-Agent", uaResult.UserAgent)
	if uaResult.SecCHUA != "" {
		req.Header.Set("Sec-CH-UA", uaResult.SecCHUA)
		req.Header.Set("Sec-CH-UA-Platform", uaResult.SecCHUAPlat)
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

func RenderPage(ctx context.Context, cdpClient *CDPClient, httpClient *http.Client, pageURL string, waitMs int, scroll bool, maxScrollSteps int, proxyURL string) (html string, statusCode int, source string, err error) {
	type result struct {
		html       string
		statusCode int
		source     string
		err        error
	}

	ch := make(chan result, 2)

	go func() {
		h, code, e := HTTPFetchHTML(ctx, httpClient, pageURL, proxyURL)
		if e != nil || len(strings.TrimSpace(h)) < 200 {
			ch <- result{err: fmt.Errorf("http fetch insufficient")}
			return
		}
		if len(h) > 500_000 {
			h = h[:500_000]
		}
		ch <- result{html: h, statusCode: code, source: "fetch"}
	}()

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

func (c *CDPClient) waitForReady(ctx context.Context) error {
	backoff := 100 * time.Millisecond
	const maxBackoff = 10 * time.Second
	for {
		if c.Healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *CDPClient) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.zenPandaURL+"/json/version", nil)
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

