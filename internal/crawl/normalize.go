package crawl

import (
	"net/url"
	"sort"
	"strings"
)

// Common tracking parameters that waste crawl budget when they create
// duplicate URLs. These are stripped during normalization.
var trackingParams = map[string]bool{
	// Google Analytics / Ads
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "utm_id": true,
	"gclid": true, "gclsrc": true, "dclid": true, "gbraid": true, "wbraid": true,
	// Facebook
	"fbclid": true, "fb_action_ids": true, "fb_action_types": true,
	"fb_source": true, "fb_ref": true,
	// Microsoft
	"msclkid": true,
	// HubSpot
	"hsa_acc": true, "hsa_cam": true, "hsa_grp": true, "hsa_ad": true,
	"hsa_src": true, "hsa_tgt": true, "hsa_kw": true, "hsa_mt": true,
	"hsa_net": true, "hsa_ver": true, "_hsenc": true, "_hsmi": true,
	"__hstc": true, "__hssc": true, "__hsfp": true,
	// Mailchimp
	"mc_cid": true, "mc_eid": true,
	// Generic tracking
	"ref": true, "referrer": true, "source": true,
	"click_id": true, "campaign_id": true, "ad_id": true,
	"_ga": true, "_gl": true, "_ke": true,
	"trk": true, "trkCampaign": true, "sc_campaign": true,
	"s_kwcid": true, "ef_id": true, "s_cid": true,
}

// NormalizeURL normalizes a URL by:
// 1. Lowercasing the scheme and host
// 2. Removing default ports (80 for http, 443 for https)
// 3. Removing trailing slashes from paths (except root "/")
// 4. Removing tracking/analytics query parameters
// 5. Sorting remaining query parameters for consistent dedup
// 6. Removing fragments
func NormalizeURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}

	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove default ports
	host := u.Hostname()
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		u.Host = host
	}

	// Remove trailing slash (except root)
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	if u.Path == "" {
		u.Path = "/"
	}

	// Strip tracking params and sort remaining
	query := u.Query()
	cleaned := make(url.Values)
	for key, vals := range query {
		if !trackingParams[strings.ToLower(key)] {
			cleaned[key] = vals
		}
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(cleaned))
	for k := range cleaned {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if len(keys) > 0 {
		var buf strings.Builder
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte('&')
			}
			vals := cleaned[k]
			sort.Strings(vals)
			for j, v := range vals {
				if j > 0 {
					buf.WriteByte('&')
				}
				buf.WriteString(url.QueryEscape(k))
				buf.WriteByte('=')
				buf.WriteString(url.QueryEscape(v))
			}
		}
		u.RawQuery = buf.String()
	} else {
		u.RawQuery = ""
	}

	// Remove fragment
	u.Fragment = ""

	return u.String()
}
