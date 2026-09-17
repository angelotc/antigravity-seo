package crawler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AICrawlerStatus describes access policy for a specific AI bot
type AICrawlerStatus struct {
	UserAgent        string   `json:"user_agent"`
	Purpose          string   `json:"purpose"`
	Status           string   `json:"status"` // Allowed, Disallowed, Partial Restrictions
	EffectiveGroup   string   `json:"effective_group,omitempty"`
	CrawlDelaySec    float64  `json:"crawl_delay_seconds,omitempty"`
	DisallowedPaths  []string `json:"disallowed_paths,omitempty"`
}

// RobotsReport contains a complete breakdown of robots.txt rules
type RobotsReport struct {
	URL            string            `json:"url"`
	Exists         bool              `json:"exists"`
	StatusCode     int               `json:"status_code"`
	Sitemaps       []string          `json:"sitemaps"`
	UserAgents     []string          `json:"user_agents"`
	AICrawlers     []AICrawlerStatus `json:"ai_crawlers"`
	HasGlobalBlock bool              `json:"has_global_block"` // Disallow: / for User-agent: *
	RawRulesCount  int               `json:"raw_rules_count"`
}

var knownAICrawlers = []struct {
	UA      string
	Purpose string
}{
	{"Googlebot", "Google Search Engine Indexing"},
	{"Google-Extended", "Google Gemini & Vertex AI training (Does not affect search)"},
	{"OAI-SearchBot", "ChatGPT Search (OpenAI live search index)"},
	{"GPTBot", "OpenAI Model Training"},
	{"ClaudeBot", "Anthropic Model Training & Search"},
	{"PerplexityBot", "Perplexity AI Answer Search Engine"},
	{"CCBot", "Common Crawl Dataset"},
	{"Bytespider", "ByteDance AI / Search"},
	{"Applebot-Extended", "Apple Intelligence Training"},
}

// accessRule is a single Allow/Disallow directive with its raw pattern
type accessRule struct {
	pattern string
	allow   bool
}

// agentGroup is a robots.txt rule group: one or more User-agent lines
// followed by rules, terminated by the next User-agent line after a rule.
type agentGroup struct {
	agents    []string
	rules     []accessRule
	crawlDelay float64
	hasDelay  bool
}

// parsedRobots is the parsed representation of a robots.txt body
type parsedRobots struct {
	groups        []*agentGroup
	sitemaps      []string
	userAgents    []string // ordered, unique as written
	rawRulesCount int
	globalBlock   bool
}

// parseRobots parses a robots.txt body into groups following the robots
// exclusion protocol with Google's longest-match precedence semantics.
func parseRobots(body string) *parsedRobots {
	parsed := &parsedRobots{
		groups:     []*agentGroup{},
		sitemaps:   []string{},
		userAgents: []string{},
	}

	var current *agentGroup
	seenRule := false

	for _, rawLine := range strings.Split(body, "\n") {
		line := rawLine
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}
		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			if seenRule {
				current = nil // a User-agent after rules starts a new group
				seenRule = false
			}
			if current == nil {
				current = &agentGroup{}
				parsed.groups = append(parsed.groups, current)
			}
			agent := strings.ToLower(value)
			current.agents = append(current.agents, agent)
			if !containsString(parsed.userAgents, value) {
				parsed.userAgents = append(parsed.userAgents, value)
			}

		case "disallow":
			if current == nil {
				continue
			}
			seenRule = true
			if value == "" {
				// Empty Disallow explicitly allows everything for this group
				current.rules = append(current.rules, accessRule{pattern: "", allow: true})
				continue
			}
			current.rules = append(current.rules, accessRule{pattern: value, allow: false})
			parsed.rawRulesCount++
			for _, a := range current.agents {
				if a == "*" && value == "/" {
					parsed.globalBlock = true
				}
			}

		case "allow":
			if current == nil {
				continue
			}
			seenRule = true
			current.rules = append(current.rules, accessRule{pattern: value, allow: true})
			parsed.rawRulesCount++

		case "crawl-delay":
			if current == nil {
				continue
			}
			seenRule = true
			if d, err := strconv.ParseFloat(value, 64); err == nil && d >= 0 {
				current.crawlDelay = d
				current.hasDelay = true
			}

		case "sitemap":
			if value != "" {
				parsed.sitemaps = append(parsed.sitemaps, value)
			}
		}
	}

	return parsed
}

// groupForAgent picks the single most specific group matching an agent token:
// exact (case-insensitive) match first, then longest token-prefix match, then "*".
// Per Google semantics only that one group applies; all others are ignored.
func (p *parsedRobots) groupForAgent(agent string) *agentGroup {
	agentLower := strings.ToLower(agent)
	var prefixBest *agentGroup
	prefixLen := -1
	var wildcard *agentGroup

	for _, g := range p.groups {
		for _, a := range g.agents {
			switch {
			case a == agentLower:
				return g
			case a == "*":
				if wildcard == nil {
					wildcard = g
				}
			case strings.HasPrefix(agentLower, a):
				if len(a) > prefixLen {
					prefixBest = g
					prefixLen = len(a)
				}
			}
		}
	}
	if prefixBest != nil {
		return prefixBest
	}
	return wildcard
}

// ruleMatches applies prefix matching with Google wildcard semantics:
// '*' matches any character sequence and '$' anchors the end of the path.
func ruleMatches(pattern, path string) bool {
	anchorEnd := strings.HasSuffix(pattern, "$")
	if anchorEnd {
		pattern = pattern[:len(pattern)-1]
	}

	var expr strings.Builder
	expr.WriteString("^")
	for i, part := range strings.Split(pattern, "*") {
		if i > 0 {
			expr.WriteString(".*")
		}
		expr.WriteString(regexp.QuoteMeta(part))
	}
	if anchorEnd {
		expr.WriteString("$")
	}

	re, err := regexp.Compile(expr.String())
	if err != nil {
		return false
	}
	return re.MatchString(path)
}

// allowsPath evaluates the most specific matching rule for a path.
// Longest pattern wins; ties go to Allow.
func (p *parsedRobots) allowsPath(agent, path string) (allowed bool, matched string) {
	group := p.groupForAgent(agent)
	if group == nil {
		return true, ""
	}

	allowed = true
	longest := -1
	for _, rule := range group.rules {
		if rule.pattern == "" {
			// Empty pattern only matches the empty path; treat as allow-all marker
			if longest < 0 {
				allowed = true
				longest = 0
				matched = ""
			}
			continue
		}
		if !ruleMatches(rule.pattern, path) {
			continue
		}
		if len(rule.pattern) > longest {
			longest = len(rule.pattern)
			allowed = rule.allow
			matched = rule.pattern
		} else if len(rule.pattern) == longest && rule.allow {
			allowed = true
			matched = rule.pattern
		}
	}
	return allowed, matched
}

// InspectRobots fetches and analyzes a domain's robots.txt
func (c *SafeClient) InspectRobots(ctx context.Context, robotsURL string) (*RobotsReport, error) {
	res, err := c.Fetch(ctx, robotsURL)
	if err != nil {
		return nil, fmt.Errorf("failed fetching robots.txt: %w", err)
	}

	report := &RobotsReport{
		URL:         robotsURL,
		Exists:      res.StatusCode == 200,
		StatusCode:  res.StatusCode,
		Sitemaps:    []string{},
		UserAgents:  []string{},
		AICrawlers:  []AICrawlerStatus{},
	}

	if res.StatusCode != 200 {
		return report, nil
	}

	parsed := parseRobots(string(res.Body))
	report.Sitemaps = parsed.sitemaps
	report.UserAgents = parsed.userAgents
	report.RawRulesCount = parsed.rawRulesCount
	report.HasGlobalBlock = parsed.globalBlock

	for _, bot := range knownAICrawlers {
		group := parsed.groupForAgent(bot.UA)
		status := AICrawlerStatus{
			UserAgent: bot.UA,
			Purpose:   bot.Purpose,
			Status:    "Allowed",
		}

		if group != nil {
			// Effective group label: named group or wildcard fallback
			if containsString(group.agents, strings.ToLower(bot.UA)) {
				status.EffectiveGroup = bot.UA
			} else if containsString(group.agents, "*") {
				status.EffectiveGroup = "*"
			} else {
				status.EffectiveGroup = group.agents[0]
			}
			if group.hasDelay {
				status.CrawlDelaySec = group.crawlDelay
			}

			disallowPatterns := []string{}
			fullyBlocked := false
			partiallyBlocked := false
			for _, rule := range group.rules {
				if rule.allow || rule.pattern == "" {
					continue
				}
				disallowPatterns = append(disallowPatterns, rule.pattern)
				if rule.pattern == "/" {
					fullyBlocked = true
				} else {
					partiallyBlocked = true
				}
			}

			// An Allow rule covering "/" can neutralize a blanket Disallow
			if fullyBlocked {
				if allowed, _ := parsed.allowsPath(bot.UA, "/"); !allowed {
					status.Status = "Disallowed"
				} else {
					fullyBlocked = false
					partiallyBlocked = len(disallowPatterns) > 0
				}
			}
			if !fullyBlocked && partiallyBlocked {
				status.Status = "Partial Restrictions"
			}
			status.DisallowedPaths = disallowPatterns
		}

		report.AICrawlers = append(report.AICrawlers, status)
	}

	return report, nil
}

func containsString(arr []string, target string) bool {
	for _, s := range arr {
		if s == target {
			return true
		}
	}
	return false
}
