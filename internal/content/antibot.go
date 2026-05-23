package content

import (
	"regexp"
	"strings"
)

const (
	tier2MaxSize = 10_000
	tier3MaxSize = 50_000
)

type antibotPattern struct {
	re   *regexp.Regexp
	name string
}

var tier1Patterns = []antibotPattern{
	{regexp.MustCompile(`Reference #\d+\.[0-9a-f]+\.\d+\.[0-9a-f]+`), "Akamai reference"},
	{regexp.MustCompile(`(?i)Pardon Our Interruption`), "Akamai interruption"},
	{regexp.MustCompile(`(?i)challenge-form.*__cf_chl_f_tk=`), "Cloudflare challenge"},
	{regexp.MustCompile(`<span class="cf-error-code">\d{4}</span>`), "Cloudflare error"},
	{regexp.MustCompile(`/cdn-cgi/challenge-platform/\S+orchestrate`), "Cloudflare orchestrate"},
	{regexp.MustCompile(`window\._pxAppId\s*=`), "PerimeterX"},
	{regexp.MustCompile(`(?i)captcha\.px-cdn\.net`), "PerimeterX captcha"},
	{regexp.MustCompile(`(?i)captcha-delivery\.com`), "DataDome"},
	{regexp.MustCompile(`_Incapsula_Resource`), "Imperva/Incapsula"},
	{regexp.MustCompile(`(?i)Incapsula incident ID`), "Imperva incident"},
	{regexp.MustCompile(`(?i)Sucuri WebSite Firewall`), "Sucuri"},
	{regexp.MustCompile(`KPSDK\.scriptStart\s*=\s*KPSDK\.now\(\)`), "Kasada"},
	{regexp.MustCompile(`(?i)blocked by network security`), "generic network block"},
}

var tier2Patterns = []antibotPattern{
	{regexp.MustCompile(`(?i)Access Denied`), "access denied"},
	{regexp.MustCompile(`(?i)Checking your browser`), "browser check"},
	{regexp.MustCompile(`(?i)Just a moment`), "Cloudflare wait"},
	{regexp.MustCompile(`(?i)class="g-recaptcha"`), "reCAPTCHA"},
	{regexp.MustCompile(`(?i)class="h-captcha"`), "hCaptcha"},
	{regexp.MustCompile(`(?i)Access to This Page Has Been Blocked`), "PerimeterX block"},
	{regexp.MustCompile(`(?i)blocked by security`), "security block"},
	{regexp.MustCompile(`(?i)Request unsuccessful`), "request failed"},
}

var (
	bodyTagRe     = regexp.MustCompile(`(?i)<body`)
	contentTagsRe = regexp.MustCompile(`(?i)<(p|h[1-6]|article|section|li|td|a|pre)[\s>]`)
	scriptTagsRe  = regexp.MustCompile(`(?i)<script[\s>]`)
)

func IsBlocked(statusCode int, htmlContent string) (bool, string) {
	size := len(htmlContent)

	if statusCode == 429 {
		return true, "rate limited (429)"
	}
	if statusCode == 403 || statusCode == 503 {
		if !looksLikeData(htmlContent) {
			return true, "blocked (" + statusCodeStr(statusCode) + ")"
		}
	}

	checkContent := htmlContent
	if size > 15_000 {
		checkContent = htmlContent[:15_000]
	}
	for _, p := range tier1Patterns {
		if p.re.MatchString(checkContent) {
			return true, p.name
		}
	}
	if size > 15_000 {
		stripped := stripScriptsStyles(htmlContent)
		for _, p := range tier1Patterns {
			if p.re.MatchString(stripped) {
				return true, p.name
			}
		}
	}

	if size < tier2MaxSize {
		for _, p := range tier2Patterns {
			if p.re.MatchString(htmlContent) {
				return true, p.name
			}
		}
	}

	if statusCode >= 400 && size < tier2MaxSize {
		for _, p := range tier2Patterns {
			if p.re.MatchString(htmlContent) {
				return true, p.name + " (" + statusCodeStr(statusCode) + ")"
			}
		}
	}

	if statusCode == 200 && size < 100 {
		return true, "near-empty response"
	}

	if size < tier3MaxSize {
		signals := structuralIntegrityCheck(htmlContent)
		if signals >= 2 {
			return true, "structural integrity failure"
		}
		if signals >= 1 && size < 5000 {
			return true, "structural integrity failure (small page)"
		}
	}

	return false, ""
}

func structuralIntegrityCheck(htmlContent string) int {
	signals := 0

	if !bodyTagRe.MatchString(htmlContent) {
		signals++
	}

	visibleText := stripScriptsStyles(htmlContent)
	visibleText = HtmlTagRe.ReplaceAllString(visibleText, "")
	visibleText = strings.TrimSpace(WhitespaceRe.ReplaceAllString(visibleText, " "))
	if len(visibleText) < 50 {
		signals++
	}

	contentElements := contentTagsRe.FindAllStringIndex(htmlContent, -1)
	if len(contentElements) == 0 {
		signals++
	}

	scripts := scriptTagsRe.FindAllStringIndex(htmlContent, -1)
	if len(scripts) > 0 && len(contentElements) == 0 && len(visibleText) < 100 {
		signals++
	}

	return signals
}

func looksLikeData(content string) bool {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '[' || strings.HasPrefix(trimmed, "<?xml")
}

func stripScriptsStyles(htmlContent string) string {
	return ScriptStyleRe.ReplaceAllString(htmlContent, " ")
}

func statusCodeStr(code int) string {
	switch code {
	case 403:
		return "403"
	case 429:
		return "429"
	case 503:
		return "503"
	default:
		return string(rune('0'+code/100)) + string(rune('0'+(code/10)%10)) + string(rune('0'+code%10))
	}
}
