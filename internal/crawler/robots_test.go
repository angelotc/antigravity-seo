package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseRobotsGroupSeparation(t *testing.T) {
	body := `
User-agent: *
Disallow: /private/

User-agent: Googlebot
Disallow: /admin/
`
	parsed := parseRobots(body)

	if len(parsed.groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(parsed.groups))
	}

	// The wildcard group must NOT contain Googlebot's rules (regression: rule leak across groups)
	star := parsed.groupForAgent("SomeRandomBot")
	if star == nil {
		t.Fatal("expected wildcard group for unknown bot")
	}
	for _, rule := range star.rules {
		if rule.pattern == "/admin/" {
			t.Error("rule leak: /admin/ from Googlebot group appeared in wildcard group")
		}
	}
}

func TestParseRobotsMultiAgentGroup(t *testing.T) {
	body := `
User-agent: GPTBot
User-agent: ClaudeBot
Disallow: /ai-blocked/
`
	parsed := parseRobots(body)
	if len(parsed.groups) != 1 {
		t.Fatalf("consecutive User-agent lines must share one group, got %d", len(parsed.groups))
	}
	if len(parsed.groups[0].agents) != 2 {
		t.Fatalf("expected 2 agents in group, got %d", len(parsed.groups[0].agents))
	}
}

func TestAllowsPathPrecedence(t *testing.T) {
	body := `
User-agent: *
Disallow: /catalog/
Allow: /catalog/public/
`
	parsed := parseRobots(body)

	if allowed, _ := parsed.allowsPath("AnyBot", "/catalog/public/item"); !allowed {
		t.Error("longer Allow rule must beat shorter Disallow")
	}
	if allowed, _ := parsed.allowsPath("AnyBot", "/catalog/secret"); allowed {
		t.Error("Disallow /catalog/ must block unmatched subpaths")
	}
}

func TestAllowsPathWildcardsAndAnchor(t *testing.T) {
	body := `
User-agent: *
Disallow: /*.pdf$
Disallow: /tmp*
Disallow: /exact$
`
	parsed := parseRobots(body)

	cases := []struct {
		path    string
		allowed bool
	}{
		{"/file.pdf", false},
		{"/docs/file.pdf", false},
		{"/file.pdfx", true},   // $ anchors end
		{"/tmp", false},
		{"/tmp/whatever", false},
		{"/exact", false},
		{"/exact/sub", true},   // $ prevents prefix match
		{"/other", true},
	}
	for _, c := range cases {
		if allowed, _ := parsed.allowsPath("Bot", c.path); allowed != c.allowed {
			t.Errorf("path %s: expected allowed=%t", c.path, c.allowed)
		}
	}
}

func TestEmptyDisallowAllowsAll(t *testing.T) {
	body := `
User-agent: *
Disallow:
`
	parsed := parseRobots(body)
	if allowed, _ := parsed.allowsPath("Bot", "/anything"); !allowed {
		t.Error("empty Disallow must allow everything")
	}
}

func TestCrawlDelayParsing(t *testing.T) {
	body := `
User-agent: *
Crawl-delay: 7.5
Disallow: /x/
`
	parsed := parseRobots(body)
	g := parsed.groupForAgent("WhateverBot")
	if g == nil || !g.hasDelay || g.crawlDelay != 7.5 {
		t.Fatalf("expected crawl-delay 7.5 in wildcard group, got %+v", g)
	}
}

func TestGlobalBlockDetection(t *testing.T) {
	parsed := parseRobots("User-agent: *\nDisallow: /")
	if !parsed.globalBlock {
		t.Error("expected global block flag for Disallow: / under *")
	}
	parsed = parseRobots("User-agent: Googlebot\nDisallow: /")
	if parsed.globalBlock {
		t.Error("global block must only trigger for wildcard group")
	}
}

func TestInspectRobotsEndToEnd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Sitemap: https://example.com/sitemap.xml\n\nUser-agent: *\nDisallow: /admin/\n\nUser-agent: GPTBot\nDisallow: /\n"))
	}))
	defer ts.Close()

	client := NewSafeClient(ClientOptions{Timeout: 5 * time.Second, AllowPrivateIPs: true})
	report, err := client.InspectRobots(context.Background(), ts.URL+"/robots.txt")
	if err != nil {
		t.Fatalf("InspectRobots error: %v", err)
	}
	if !report.Exists {
		t.Fatal("expected robots.txt to exist")
	}
	if len(report.Sitemaps) != 1 || report.Sitemaps[0] != "https://example.com/sitemap.xml" {
		t.Errorf("sitemap not parsed: %+v", report.Sitemaps)
	}

	var gpt *AICrawlerStatus
	for i := range report.AICrawlers {
		if report.AICrawlers[i].UserAgent == "GPTBot" {
			gpt = &report.AICrawlers[i]
		}
	}
	if gpt == nil {
		t.Fatal("GPTBot missing from report")
	}
	if gpt.Status != "Disallowed" {
		t.Errorf("GPTBot should be fully disallowed, got %q (effective group %q)", gpt.Status, gpt.EffectiveGroup)
	}

	var google *AICrawlerStatus
	for i := range report.AICrawlers {
		if report.AICrawlers[i].UserAgent == "Googlebot" {
			google = &report.AICrawlers[i]
		}
	}
	if google == nil || google.Status != "Partial Restrictions" {
		t.Errorf("Googlebot should fall back to * group with partial restrictions, got %+v", google)
	}
}
