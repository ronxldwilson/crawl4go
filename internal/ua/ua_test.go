package ua

import (
	"strings"
	"sync"
	"testing"
)

// TestRandomUANonEmpty verifies that RandomUA always returns a non-empty UserAgent.
func TestRandomUANonEmpty(t *testing.T) {
	result := RandomUA()
	if result.UserAgent == "" {
		t.Error("RandomUA() returned an empty UserAgent string")
	}
}

// TestChromeSecCHUA verifies that a Chrome entry includes "Google Chrome" in SecCHUA.
func TestChromeSecCHUA(t *testing.T) {
	initUAPool()
	for _, entry := range uaPool {
		if entry.brand == "Google Chrome" {
			result := buildUAResult(entry)
			if !strings.Contains(result.SecCHUA, "Google Chrome") {
				t.Errorf("Chrome entry SecCHUA %q does not contain 'Google Chrome'", result.SecCHUA)
			}
			return
		}
	}
	t.Fatal("no Google Chrome entry found in pool")
}

// TestEdgeSecCHUA verifies that an Edge entry includes "Microsoft Edge" in SecCHUA.
func TestEdgeSecCHUA(t *testing.T) {
	initUAPool()
	for _, entry := range uaPool {
		if entry.brand == "Microsoft Edge" {
			result := buildUAResult(entry)
			if !strings.Contains(result.SecCHUA, "Microsoft Edge") {
				t.Errorf("Edge entry SecCHUA %q does not contain 'Microsoft Edge'", result.SecCHUA)
			}
			return
		}
	}
	t.Fatal("no Microsoft Edge entry found in pool")
}

// TestFirefoxSecCHUAEmpty verifies that Firefox entries have an empty SecCHUA.
func TestFirefoxSecCHUAEmpty(t *testing.T) {
	initUAPool()
	for _, entry := range uaPool {
		if entry.brand == "Firefox" {
			result := buildUAResult(entry)
			if result.SecCHUA != "" {
				t.Errorf("Firefox entry SecCHUA = %q, want empty", result.SecCHUA)
			}
			return
		}
	}
	t.Fatal("no Firefox entry found in pool")
}

// TestSafariSecCHUAEmpty verifies that Safari entries have an empty SecCHUA.
func TestSafariSecCHUAEmpty(t *testing.T) {
	initUAPool()
	for _, entry := range uaPool {
		if entry.brand == "Safari" {
			result := buildUAResult(entry)
			if result.SecCHUA != "" {
				t.Errorf("Safari entry SecCHUA = %q, want empty", result.SecCHUA)
			}
			return
		}
	}
	t.Fatal("no Safari entry found in pool")
}

// TestSecCHUAPlatIsKnown verifies that SecCHUAPlat is one of Windows, macOS, or Linux.
func TestSecCHUAPlatIsKnown(t *testing.T) {
	validPlatforms := map[string]bool{
		`"Windows"`: true,
		`"macOS"`:   true,
		`"Linux"`:   true,
	}

	for i := 0; i < 50; i++ {
		result := RandomUA()
		if !validPlatforms[result.SecCHUAPlat] {
			t.Errorf("SecCHUAPlat = %q, want one of Windows/macOS/Linux", result.SecCHUAPlat)
		}
	}
}

// TestRandomUAReturnsDiversity verifies that 200 calls return at least 3 distinct UserAgent strings.
func TestRandomUAReturnsDiversity(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 200; i++ {
		result := RandomUA()
		seen[result.UserAgent] = struct{}{}
	}
	if len(seen) < 3 {
		t.Errorf("only %d distinct UserAgent strings from 200 calls, want at least 3", len(seen))
	}
}

// TestConcurrentRandomUA runs concurrent RandomUA calls to detect data races (-race).
func TestConcurrentRandomUA(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = RandomUA()
		}()
	}
	wg.Wait()
}

// TestPoolContainsBrowserFamilies verifies that the pool has Chrome, Firefox, Edge, and Safari.
func TestPoolContainsBrowserFamilies(t *testing.T) {
	initUAPool()

	families := map[string]bool{
		"Google Chrome": false,
		"Firefox":       false,
		"Microsoft Edge": false,
		"Safari":        false,
	}

	for _, entry := range uaPool {
		if _, ok := families[entry.brand]; ok {
			families[entry.brand] = true
		}
	}

	for family, found := range families {
		if !found {
			t.Errorf("pool missing browser family: %q", family)
		}
	}
}

// buildUAResult replicates the SecCHUA / SecCHUAPlat logic from RandomUA for a given entry,
// allowing tests to inspect specific pool entries without relying on randomness.
func buildUAResult(entry uaEntry) UAResult {
	secCHUA := ""
	switch {
	case strings.Contains(entry.brand, "Chrome"):
		secCHUA = `"Not/A)Brand";v="8", "Chromium";v="` + entry.version + `", "Google Chrome";v="` + entry.version + `"`
	case strings.Contains(entry.brand, "Edge"):
		secCHUA = `"Not/A)Brand";v="8", "Chromium";v="` + entry.version + `", "Microsoft Edge";v="` + entry.version + `"`
	case strings.Contains(entry.brand, "Firefox"):
		secCHUA = ""
	case strings.Contains(entry.brand, "Safari"):
		secCHUA = ""
	}

	return UAResult{
		UserAgent:   entry.ua,
		SecCHUA:     secCHUA,
		SecCHUAPlat: `"` + entry.platform + `"`,
	}
}
