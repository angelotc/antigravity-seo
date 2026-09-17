package crawler

import (
	"bufio"
	"context"
	"fmt"
	"strings"
)

// AICrawlerStatus describes access policy for a specific AI bot
type AICrawlerStatus struct {
	UserAgent   string `json:"user_agent"`
	Purpose     string `json:"purpose"`
	Status      string `json:"status"` // Allowed, Disallowed, Partial
	DisallowedPaths []string `json:"disallowed_paths,omitempty"`
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

// InspectRobots fetches and analyzes a domain's robots.txt
func (c *SafeClient) InspectRobots(ctx context.Context, robotsURL string) (*RobotsReport, error) {
	res, err := c.Fetch(ctx, robotsURL)
	if err != nil {
		return nil, fmt.Errorf("failed fetching robots.txt: %w", err)
	}

	report := &RobotsReport{
		URL:        robotsURL,
		Exists:     res.StatusCode == 200,
		StatusCode: res.StatusCode,
		Sitemaps:   make([]string, 0),
		UserAgents: make([]string, 0),
		AICrawlers: make([]AICrawlerStatus, 0),
	}

	if res.StatusCode != 200 {
		return report, nil
	}

	// Parse lines
	scanner := bufio.NewScanner(strings.NewReader(string(res.Body)))
	rules := make(map[string][]string) // agent -> disallowed paths
	currentAgents := make([]string, 0)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "sitemap":
			report.Sitemaps = append(report.Sitemaps, value)
		case "user-agent":
			agent := strings.ToLower(value)
			currentAgents = append(currentAgents, agent)
			if !containsString(report.UserAgents, value) {
				report.UserAgents = append(report.UserAgents, value)
			}
		case "disallow":
			for _, a := range currentAgents {
				rules[a] = append(rules[a], value)
				if a == "*" && value == "/" {
					report.HasGlobalBlock = true
				}
			}
		}
	}

	report.RawRulesCount = len(rules)

	// Check status for each known AI Crawler
	for _, bot := range knownAICrawlers {
		botLower := strings.ToLower(bot.UA)
		disallows, hasSpecific := rules[botLower]
		if !hasSpecific {
			// Fall back to wildcard *
			disallows = rules["*"]
		}

		status := "Allowed"
		if len(disallows) > 0 {
			if containsString(disallows, "/") {
				status = "Disallowed"
			} else {
				status = "Partial Restrictions"
			}
		}

		report.AICrawlers = append(report.AICrawlers, AICrawlerStatus{
			UserAgent:       bot.UA,
			Purpose:         bot.Purpose,
			Status:          status,
			DisallowedPaths: disallows,
		})
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
