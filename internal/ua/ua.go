package ua

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
)

type UAResult struct {
	UserAgent   string
	SecCHUA     string
	SecCHUAPlat string
}

var (
	uaOnce sync.Once
	uaPool []uaEntry
)

type uaEntry struct {
	ua       string
	brand    string
	version  string
	platform string
}

func initUAPool() {
	uaOnce.Do(func() {
		chromeVersions := []string{"120", "121", "122", "123", "124", "125", "126", "127", "128"}
		firefoxVersions := []string{"121", "122", "123", "124", "125", "126", "127", "128"}
		edgeVersions := []string{"120", "121", "122", "123", "124", "125", "126"}

		platforms := []struct {
			os       string
			platform string
		}{
			{"Windows NT 10.0; Win64; x64", "Windows"},
			{"Windows NT 10.0; Win64; x64", "Windows"},
			{"Macintosh; Intel Mac OS X 10_15_7", "macOS"},
			{"Macintosh; Intel Mac OS X 14_5", "macOS"},
			{"X11; Linux x86_64", "Linux"},
		}

		for _, v := range chromeVersions {
			for _, p := range platforms {
				uaPool = append(uaPool, uaEntry{
					ua:       fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36", p.os, v),
					brand:    "Google Chrome",
					version:  v,
					platform: p.platform,
				})
			}
		}

		for _, v := range firefoxVersions {
			for _, p := range platforms {
				uaPool = append(uaPool, uaEntry{
					ua:       fmt.Sprintf("Mozilla/5.0 (%s; rv:%s.0) Gecko/20100101 Firefox/%s.0", p.os, v, v),
					brand:    "Firefox",
					version:  v,
					platform: p.platform,
				})
			}
		}

		for _, v := range edgeVersions {
			for _, p := range platforms {
				uaPool = append(uaPool, uaEntry{
					ua:       fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s.0.0.0 Safari/537.36 Edg/%s.0.0.0", p.os, v, v),
					brand:    "Microsoft Edge",
					version:  v,
					platform: p.platform,
				})
			}
		}

		safariMacVersions := []struct {
			osVer     string
			safariVer string
			webkitVer string
		}{
			{"10_15_7", "17.2", "605.1.15"},
			{"14_0", "17.3", "605.1.15"},
			{"14_5", "17.5", "605.1.15"},
			{"14_4", "17.4", "605.1.15"},
		}
		for _, sv := range safariMacVersions {
			uaPool = append(uaPool, uaEntry{
				ua:       fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X %s) AppleWebKit/%s (KHTML, like Gecko) Version/%s Safari/%s", sv.osVer, sv.webkitVer, sv.safariVer, sv.webkitVer),
				brand:    "Safari",
				version:  strings.Split(sv.safariVer, ".")[0],
				platform: "macOS",
			})
		}
	})
}

func RandomUA() UAResult {
	initUAPool()
	entry := uaPool[rand.Intn(len(uaPool))]

	secCHUA := ""
	switch {
	case strings.Contains(entry.brand, "Chrome"):
		secCHUA = fmt.Sprintf(`"Not/A)Brand";v="8", "Chromium";v="%s", "Google Chrome";v="%s"`, entry.version, entry.version)
	case strings.Contains(entry.brand, "Edge"):
		secCHUA = fmt.Sprintf(`"Not/A)Brand";v="8", "Chromium";v="%s", "Microsoft Edge";v="%s"`, entry.version, entry.version)
	case strings.Contains(entry.brand, "Firefox"):
		secCHUA = ""
	case strings.Contains(entry.brand, "Safari"):
		secCHUA = ""
	}

	return UAResult{
		UserAgent:   entry.ua,
		SecCHUA:     secCHUA,
		SecCHUAPlat: fmt.Sprintf(`"%s"`, entry.platform),
	}
}
