package content

import (
	"strings"
	"testing"
)

func TestIsBlocked(t *testing.T) {
	normalPage := `<html><body><p>This is a normal page with real content.</p>
		<article><h1>Welcome</h1><p>Some interesting article text that is long enough to pass checks.</p></article>
		<p>More content here for structural integrity.</p>
		<a href="/about">About us</a></body></html>`

	tests := []struct {
		name       string
		statusCode int
		html       string
		wantBlock  bool
		wantReason string // substring match on reason
	}{
		{
			name:       "normal page 200",
			statusCode: 200,
			html:       normalPage,
			wantBlock:  false,
		},
		{
			name:       "rate limited 429",
			statusCode: 429,
			html:       normalPage,
			wantBlock:  true,
			wantReason: "rate limited",
		},
		{
			name:       "403 with non-data content",
			statusCode: 403,
			html:       "<html><body>Forbidden</body></html>",
			wantBlock:  true,
			wantReason: "blocked",
		},
		{
			name:       "403 with JSON data skips status block but hits structural",
			statusCode: 403,
			html:       `{"error": "forbidden", "message": "You do not have access to this resource", "details": "Contact admin"}`,
			wantBlock:  true,
			wantReason: "structural integrity",
		},
		{
			name:       "503 with non-data content",
			statusCode: 503,
			html:       "<html><body>Service Unavailable</body></html>",
			wantBlock:  true,
			wantReason: "blocked",
		},
		{
			name:       "403 with HTML body blocked",
			statusCode: 403,
			html:       `<html><body><p>Forbidden</p></body></html>`,
			wantBlock:  true,
			wantReason: "blocked",
		},
		{
			name:       "503 with XML data skips status block but hits structural",
			statusCode: 503,
			html:       `<?xml version="1.0"?><error>Service Unavailable</error>` + strings.Repeat(" data ", 50),
			wantBlock:  true,
			wantReason: "structural integrity",
		},
		{
			name:       "Cloudflare challenge page",
			statusCode: 200,
			html:       `<html><body><form class="challenge-form" action="?__cf_chl_f_tk=abc123">Please wait</form></body></html>`,
			wantBlock:  true,
			wantReason: "Cloudflare challenge",
		},
		{
			name:       "Cloudflare error code",
			statusCode: 200,
			html:       `<html><body><span class="cf-error-code">1020</span></body></html>`,
			wantBlock:  true,
			wantReason: "Cloudflare error",
		},
		{
			name:       "Cloudflare orchestrate",
			statusCode: 200,
			html:       `<html><body><script src="/cdn-cgi/challenge-platform/h/g/orchestrate/chl_page/v1"></script></body></html>`,
			wantBlock:  true,
			wantReason: "Cloudflare orchestrate",
		},
		{
			name:       "PerimeterX detected",
			statusCode: 200,
			html:       `<html><body><script>window._pxAppId = 'PX12345';</script></body></html>`,
			wantBlock:  true,
			wantReason: "PerimeterX",
		},
		{
			name:       "DataDome captcha",
			statusCode: 200,
			html:       `<html><body><script src="https://captcha-delivery.com/123"></script></body></html>`,
			wantBlock:  true,
			wantReason: "DataDome",
		},
		{
			name:       "Imperva Incapsula",
			statusCode: 200,
			html:       `<html><body><script>_Incapsula_Resource = 'abc';</script></body></html>`,
			wantBlock:  true,
			wantReason: "Imperva/Incapsula",
		},
		{
			name:       "Akamai reference",
			statusCode: 200,
			html:       `<html><body>Reference #18.abc123.1234567890.1a2b3c4d</body></html>`,
			wantBlock:  true,
			wantReason: "Akamai reference",
		},
		{
			name:       "small page with Access Denied (tier2)",
			statusCode: 200,
			html:       `<html><body>Access Denied</body></html>`,
			wantBlock:  true,
			wantReason: "access denied",
		},
		{
			name:       "small page with browser check (tier2)",
			statusCode: 200,
			html:       `<html><body>Checking your browser before accessing</body></html>`,
			wantBlock:  true,
			wantReason: "browser check",
		},
		{
			name:       "reCAPTCHA (tier2)",
			statusCode: 200,
			html:       `<html><body><div class="g-recaptcha" data-sitekey="abc"></div></body></html>`,
			wantBlock:  true,
			wantReason: "reCAPTCHA",
		},
		{
			name:       "near-empty 200 response",
			statusCode: 200,
			html:       "OK",
			wantBlock:  true,
			wantReason: "near-empty",
		},
		{
			name:       "200 with empty body",
			statusCode: 200,
			html:       "",
			wantBlock:  true,
			wantReason: "near-empty",
		},
		{
			name:       "near-empty script-only page",
			statusCode: 200,
			html:       `<script>redirect()</script>`,
			wantBlock:  true,
			wantReason: "near-empty",
		},
		{
			name:       "structural integrity failure - no body, scripts only, larger page",
			statusCode: 200,
			html:       `<script>` + strings.Repeat("var x=1;", 20) + `</script>`,
			wantBlock:  true,
			wantReason: "structural integrity",
		},
		{
			name:       "large normal page not blocked by tier2",
			statusCode: 200,
			html:       normalPage + strings.Repeat(" content ", 2000),
			wantBlock:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason := IsBlocked(tt.statusCode, tt.html)
			if blocked != tt.wantBlock {
				t.Errorf("IsBlocked(%d, ...) blocked = %v, want %v (reason: %q)", tt.statusCode, blocked, tt.wantBlock, reason)
			}
			if tt.wantBlock && tt.wantReason != "" {
				if !strings.Contains(reason, tt.wantReason) {
					t.Errorf("reason = %q, want to contain %q", reason, tt.wantReason)
				}
			}
		})
	}
}

func TestLooksLikeData(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"JSON object", `{"key": "value"}`, true},
		{"JSON array", `[1, 2, 3]`, true},
		{"XML", `<?xml version="1.0"?><root/>`, true},
		{"HTML", `<html><body>Hello</body></html>`, false},
		{"empty", "", false},
		{"whitespace then JSON", `  {"key": "value"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeData(tt.content)
			if got != tt.want {
				t.Errorf("looksLikeData(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestStatusCodeStr(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{403, "403"},
		{429, "429"},
		{503, "503"},
		{200, "200"},
		{404, "404"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := statusCodeStr(tt.code)
			if got != tt.want {
				t.Errorf("statusCodeStr(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}
