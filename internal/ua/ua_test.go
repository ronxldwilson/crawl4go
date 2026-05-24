package ua

import (
	"strings"
	"testing"
)

func TestRandomUA_ReturnsNonEmpty(t *testing.T) {
	result := RandomUA()
	if result.UserAgent == "" {
		t.Fatal("RandomUA() returned empty UserAgent")
	}
	if result.SecCHUAPlat == "" {
		t.Fatal("RandomUA() returned empty SecCHUAPlat")
	}
}

func TestRandomUA_ValidUserAgentFormat(t *testing.T) {
	for i := 0; i < 50; i++ {
		result := RandomUA()
		if !strings.HasPrefix(result.UserAgent, "Mozilla/5.0") {
			t.Fatalf("UserAgent should start with Mozilla/5.0, got: %s", result.UserAgent)
		}
	}
}

func TestRandomUA_BrowserTypes(t *testing.T) {
	// Sample enough to see all browser types.
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		ua := RandomUA().UserAgent
		switch {
		case strings.Contains(ua, "Firefox/"):
			seen["Firefox"] = true
		case strings.Contains(ua, "Edg/"):
			seen["Edge"] = true
		case strings.Contains(ua, "Chrome/") && strings.Contains(ua, "Safari/") && !strings.Contains(ua, "Edg/"):
			seen["Chrome"] = true
		case strings.Contains(ua, "Version/") && strings.Contains(ua, "Safari/") && !strings.Contains(ua, "Chrome/"):
			seen["Safari"] = true
		}
	}

	for _, browser := range []string{"Chrome", "Firefox", "Edge", "Safari"} {
		if !seen[browser] {
			t.Errorf("expected to see %s in random UA pool, but never encountered it in 500 samples", browser)
		}
	}
}

func TestRandomUA_SecCHUA(t *testing.T) {
	tests := []struct {
		name       string
		brand      string
		wantCHUA   bool
		checkBrand string
	}{
		{"Chrome has Sec-CH-UA", "Chrome/", true, "Google Chrome"},
		{"Edge has Sec-CH-UA", "Edg/", true, "Microsoft Edge"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Find a UA matching the brand.
			initUAPool()
			var found UAResult
			for i := 0; i < 1000; i++ {
				r := RandomUA()
				if strings.Contains(r.UserAgent, tc.brand) {
					found = r
					break
				}
			}
			if found.UserAgent == "" {
				t.Skipf("could not find UA containing %s", tc.brand)
			}
			if tc.wantCHUA && found.SecCHUA == "" {
				t.Errorf("expected non-empty SecCHUA for %s UA", tc.brand)
			}
			if tc.wantCHUA && !strings.Contains(found.SecCHUA, tc.checkBrand) {
				t.Errorf("SecCHUA %q should contain %q", found.SecCHUA, tc.checkBrand)
			}
		})
	}
}

func TestRandomUA_SecCHUAPlatform(t *testing.T) {
	validPlatforms := map[string]bool{
		`"Windows"`: true,
		`"macOS"`:   true,
		`"Linux"`:   true,
	}

	for i := 0; i < 100; i++ {
		r := RandomUA()
		if !validPlatforms[r.SecCHUAPlat] {
			t.Fatalf("unexpected SecCHUAPlat: %s", r.SecCHUAPlat)
		}
	}
}

func TestRandomUA_FirefoxAndSafariHaveEmptySecCHUA(t *testing.T) {
	initUAPool()
	for i := 0; i < 1000; i++ {
		r := RandomUA()
		isFirefox := strings.Contains(r.UserAgent, "Firefox/")
		isSafari := strings.Contains(r.UserAgent, "Version/") && strings.Contains(r.UserAgent, "Safari/") && !strings.Contains(r.UserAgent, "Chrome/")
		if (isFirefox || isSafari) && r.SecCHUA != "" {
			t.Fatalf("Firefox/Safari should have empty SecCHUA, got: %s (UA: %s)", r.SecCHUA, r.UserAgent)
		}
	}
}

func TestPoolSize(t *testing.T) {
	initUAPool()
	// 9 chrome * 5 platforms + 8 firefox * 5 + 7 edge * 5 + 4 safari = 45 + 40 + 35 + 4 = 124
	if len(uaPool) == 0 {
		t.Fatal("uaPool should not be empty after init")
	}
	if len(uaPool) < 50 {
		t.Errorf("expected at least 50 entries in uaPool, got %d", len(uaPool))
	}
}
