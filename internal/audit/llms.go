package audit

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"antigravity-seo/internal/crawler"
)

// LLMSTxtReport audits /llms.txt and /llms-full.txt availability and shape
type LLMSTxtReport struct {
	Domain         string       `json:"domain"`
	LLMSTxtExists  bool         `json:"llms_txt_exists"`
	LLMSTxtStatus  int          `json:"llms_txt_status"`
	LLMSTxtBytes   int          `json:"llms_txt_bytes"`
	LLMSTxtLinks   int          `json:"llms_txt_links"`
	LLMSTxtSections int         `json:"llms_txt_sections"`
	FullTxtExists  bool         `json:"full_txt_exists"`
	FullTxtStatus  int          `json:"full_txt_status"`
	FullTxtBytes   int          `json:"full_txt_bytes"`
	Score          int          `json:"score"`
	Issues         []AuditIssue `json:"issues"`
}

var mdLinkRe = regexp.MustCompile(`\[[^\]]+\]\((https?://[^)\s]+)\)`)

// InspectLLMSTxt fetches /llms.txt and /llms-full.txt for a page's origin
func InspectLLMSTxt(ctx context.Context, client *crawler.SafeClient, pageURL string) (*LLMSTxtReport, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)

	report := &LLMSTxtReport{
		Domain: parsed.Host,
		Score:  100,
		Issues: []AuditIssue{},
	}

	// /llms.txt
	if res, err := client.Fetch(ctx, origin+"/llms.txt"); err == nil {
		report.LLMSTxtStatus = res.StatusCode
		if res.StatusCode == 200 {
			body := string(res.Body)
			report.LLMSTxtExists = true
			report.LLMSTxtBytes = len(res.Body)
			report.LLMSTxtLinks = len(mdLinkRe.FindAllStringIndex(body, -1))
			report.LLMSTxtSections = strings.Count(body, "\n#") + strings.Count(body, "\n##")
			if !strings.HasPrefix(strings.TrimSpace(body), "#") {
				report.Issues = append(report.Issues, AuditIssue{
					Severity: SeverityWarning, Category: "LLMs.txt",
					Message: "/llms.txt does not start with a Markdown H1 title",
				})
				report.Score -= 10
			}
			if report.LLMSTxtLinks == 0 {
				report.Issues = append(report.Issues, AuditIssue{
					Severity: SeverityWarning, Category: "LLMs.txt",
					Message: "/llms.txt contains no Markdown links to site content",
				})
				report.Score -= 15
			}
		}
	}

	// /llms-full.txt
	if res, err := client.Fetch(ctx, origin+"/llms-full.txt"); err == nil {
		report.FullTxtStatus = res.StatusCode
		if res.StatusCode == 200 {
			report.FullTxtExists = true
			report.FullTxtBytes = len(res.Body)
		}
	}

	if !report.LLMSTxtExists {
		report.Score = 0
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "LLMs.txt",
			Message: "No /llms.txt found — AI assistants have no curated site map",
			Details: "Generate a Markdown llms.txt listing key pages with one-line descriptions; serve it at the domain root. Treat it as a discovery aid, not a citation lever.",
		})
	} else if !report.FullTxtExists {
		report.Issues = append(report.Issues, AuditIssue{
			Severity: SeverityInfo, Category: "LLMs.txt",
			Message: "No /llms-full.txt — consider an expanded variant with full page content for deep crawls",
		})
		report.Score -= 5
	}

	return report, nil
}
