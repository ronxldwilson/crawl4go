package content

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

type Link struct {
	Href string `json:"href"`
	Text string `json:"text"`
}

type LinkSet struct {
	Internal []Link `json:"internal"`
	External []Link `json:"external"`
}

var skipPrefixes = []string{"javascript:", "mailto:", "tel:", "data:", "ftp:"}

var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "utm_id": true,
	"fbclid": true, "gclid": true, "ref": true, "msclkid": true,
}

func ExtractLinks(htmlContent string, baseURL string) LinkSet {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return LinkSet{}
	}

	base, _ := url.Parse(baseURL)
	baseDomain := getBaseDomain(baseURL)

	var links LinkSet
	seen := make(map[string]bool)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := GetAttr(n, "href")
			if href == "" || href == "#" {
				goto children
			}
			for _, prefix := range skipPrefixes {
				if strings.HasPrefix(strings.ToLower(href), prefix) {
					goto children
				}
			}

			{
				normalized := NormalizeURL(href, base)
				if normalized == "" || seen[normalized] {
					goto children
				}
				seen[normalized] = true

				text := ExtractText(n)
				link := Link{Href: normalized, Text: text}

				if isExternalURL(normalized, baseDomain) {
					links.External = append(links.External, link)
				} else {
					links.Internal = append(links.Internal, link)
				}
			}
		}
	children:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if links.Internal == nil {
		links.Internal = []Link{}
	}
	if links.External == nil {
		links.External = []Link{}
	}
	return links
}

func NormalizeURL(href string, base *url.URL) string {
	parsed, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}

	resolved := base.ResolveReference(parsed)

	resolved.Host = strings.ToLower(resolved.Host)
	resolved.Fragment = ""

	q := resolved.Query()
	changed := false
	for key := range q {
		if trackingParams[strings.ToLower(key)] {
			q.Del(key)
			changed = true
		}
	}
	if changed {
		resolved.RawQuery = q.Encode()
	}

	if resolved.Path != "/" {
		resolved.Path = strings.TrimRight(resolved.Path, "/")
	}
	if resolved.Path == "" {
		resolved.Path = "/"
	}

	return resolved.String()
}

func getBaseDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

func isExternalURL(href string, baseDomain string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")

	if host == baseDomain {
		return false
	}
	if strings.HasSuffix(host, "."+baseDomain) {
		return false
	}
	return true
}

func GetAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func ExtractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(ExtractText(c))
	}
	return strings.TrimSpace(sb.String())
}
