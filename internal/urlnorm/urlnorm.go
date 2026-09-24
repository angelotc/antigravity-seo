// Package urlnorm normalizes URLs so that equivalent spellings of the same
// resource compare equal (canonical/hreflang self-reference, crawl dedupe).
package urlnorm

import (
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/idna"
)

// trackingParams are query parameters that never change page content.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "msclkid": true, "yclid": true,
	"mc_cid": true, "mc_eid": true, "_ga": true, "_gl": true,
}

// Normalize resolves raw against base (which may be nil) and returns a
// canonical absolute form: lowercase scheme and host, IDN host in punycode,
// default port removed, empty path as "/", canonical percent-encoding and no
// fragment. The query string is preserved.
func Normalize(raw string, base *url.URL) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if ascii, err := idna.Lookup.ToASCII(host); err == nil {
		host = ascii
	}
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = host + ":" + port
	} else if strings.Contains(host, ":") { // bare IPv6 literal
		host = "[" + host + "]"
	}
	u.Host = host
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawPath = "" // re-encode Path canonically: /物件 == /%E7%89%A9%E4%BB%B6
	u.Fragment = ""
	u.RawFragment = ""
	return u, nil
}

// Key returns a dedupe key: Normalize, minus tracking parameters (utm_* and
// click IDs), with remaining query parameters sorted and the trailing slash
// trimmed from non-root paths. It returns raw unchanged if it cannot be parsed.
func Key(raw string, base *url.URL) string {
	u, err := Normalize(raw, base)
	if err != nil {
		return raw
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			if strings.HasPrefix(strings.ToLower(k), "utm_") || trackingParams[strings.ToLower(k)] {
				q.Del(k)
			}
		}
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for _, k := range keys {
			vals := q[k]
			sort.Strings(vals)
			for _, v := range vals {
				if b.Len() > 0 {
					b.WriteByte('&')
				}
				b.WriteString(url.QueryEscape(k) + "=" + url.QueryEscape(v))
			}
		}
		u.RawQuery = b.String()
	}
	u.ForceQuery = false
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Path == "" {
			u.Path = "/"
		}
	}
	return u.String()
}

// Same reports whether a and b (each resolved against base) identify the
// same resource under Key normalization.
func Same(a, b string, base *url.URL) bool {
	return Key(a, base) == Key(b, base)
}
