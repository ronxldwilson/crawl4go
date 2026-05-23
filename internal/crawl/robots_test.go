package crawl

import (
	"testing"
)

func TestParseRobots(t *testing.T) {
	rc := NewRobotsChecker()

	content := `
# Example robots.txt
User-agent: Googlebot
Disallow: /private/
Allow: /private/public/

User-agent: *
Disallow: /secret/
Disallow: /tmp/
Allow: /tmp/ok/
`

	entry := rc.parseRobots(content)

	if len(entry.rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(entry.rules))
	}

	// Googlebot rule.
	if entry.rules[0].userAgent != "googlebot" {
		t.Errorf("rule 0 user-agent = %q, want %q", entry.rules[0].userAgent, "googlebot")
	}
	if len(entry.rules[0].disallow) != 1 || entry.rules[0].disallow[0] != "/private/" {
		t.Errorf("rule 0 disallow = %v, want [/private/]", entry.rules[0].disallow)
	}
	if len(entry.rules[0].allow) != 1 || entry.rules[0].allow[0] != "/private/public/" {
		t.Errorf("rule 0 allow = %v, want [/private/public/]", entry.rules[0].allow)
	}

	// Wildcard rule.
	if entry.rules[1].userAgent != "*" {
		t.Errorf("rule 1 user-agent = %q, want %q", entry.rules[1].userAgent, "*")
	}
	if len(entry.rules[1].disallow) != 2 {
		t.Errorf("rule 1 disallow count = %d, want 2", len(entry.rules[1].disallow))
	}
}

func TestParseRobotsComments(t *testing.T) {
	rc := NewRobotsChecker()

	content := `
User-agent: * # all bots
Disallow: /admin/ # admin pages
# This is a full-line comment
Allow: /admin/public/
`

	entry := rc.parseRobots(content)
	if len(entry.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(entry.rules))
	}
	if len(entry.rules[0].disallow) != 1 {
		t.Errorf("disallow count = %d, want 1", len(entry.rules[0].disallow))
	}
	if len(entry.rules[0].allow) != 1 {
		t.Errorf("allow count = %d, want 1", len(entry.rules[0].allow))
	}
}

func TestParseRobotsEmptyDisallow(t *testing.T) {
	rc := NewRobotsChecker()

	content := `
User-agent: *
Disallow:
`

	entry := rc.parseRobots(content)
	if len(entry.rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(entry.rules))
	}
	// Empty Disallow value should not be added.
	if len(entry.rules[0].disallow) != 0 {
		t.Errorf("disallow count = %d, want 0", len(entry.rules[0].disallow))
	}
}

func TestPathMatches(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pattern string
		want    bool
	}{
		{
			name:    "prefix match",
			path:    "/admin/settings",
			pattern: "/admin/",
			want:    true,
		},
		{
			name:    "no prefix match",
			path:    "/public/page",
			pattern: "/admin/",
			want:    false,
		},
		{
			name:    "exact match",
			path:    "/admin/settings",
			pattern: "/admin",
			want:    true,
		},
		{
			name:    "wildcard suffix",
			path:    "/images/photo.jpg",
			pattern: "/images/*",
			want:    true,
		},
		{
			name:    "dollar sign exact match",
			path:    "/page",
			pattern: "/page$",
			want:    true,
		},
		{
			name:    "dollar sign no match",
			path:    "/page/sub",
			pattern: "/page$",
			want:    false,
		},
		{
			name:    "empty pattern no match",
			path:    "/anything",
			pattern: "",
			want:    false,
		},
		{
			name:    "root path",
			path:    "/",
			pattern: "/",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathMatches(tt.path, tt.pattern)
			if got != tt.want {
				t.Errorf("pathMatches(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestIsAllowed(t *testing.T) {
	rc := NewRobotsChecker()

	content := `
User-agent: Googlebot
Disallow: /private/
Allow: /private/public/

User-agent: *
Disallow: /secret/
Disallow: /tmp/
`

	entry := rc.parseRobots(content)

	tests := []struct {
		name      string
		userAgent string
		path      string
		want      bool
	}{
		{
			name:      "Googlebot blocked from /private/",
			userAgent: "Googlebot",
			path:      "/private/page",
			want:      false,
		},
		{
			name:      "Googlebot allowed in /private/public/",
			userAgent: "Googlebot",
			path:      "/private/public/page",
			want:      true,
		},
		{
			name:      "Googlebot allowed elsewhere",
			userAgent: "Googlebot",
			path:      "/public/page",
			want:      true,
		},
		{
			name:      "random bot blocked from /secret/",
			userAgent: "RandomBot",
			path:      "/secret/data",
			want:      false,
		},
		{
			name:      "random bot allowed on public pages",
			userAgent: "RandomBot",
			path:      "/public/page",
			want:      true,
		},
		{
			name:      "unknown agent with no matching rule allowed",
			userAgent: "UnknownBot",
			path:      "/secret/data",
			want:      false, // matches wildcard *
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rc.isAllowed(entry, tt.userAgent, tt.path)
			if got != tt.want {
				t.Errorf("isAllowed(%q, %q) = %v, want %v", tt.userAgent, tt.path, got, tt.want)
			}
		})
	}
}

func TestIsAllowedNoRules(t *testing.T) {
	rc := NewRobotsChecker()
	entry := rc.parseRobots("")

	got := rc.isAllowed(entry, "AnyBot", "/any/path")
	if !got {
		t.Error("expected allowed when no rules exist")
	}
}
