package report

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
)

// IssueSeverity indicates the impact of an audit finding
type IssueSeverity string

const (
	SeverityCritical IssueSeverity = "CRITICAL"
	SeverityWarning  IssueSeverity = "WARNING"
	SeverityPass     IssueSeverity = "PASS"
	SeverityInfo     IssueSeverity = "INFO"
)

// Issue represents a discrete SEO audit finding
type Issue struct {
	Severity       IssueSeverity `json:"severity"`
	Category       string        `json:"category"`
	Message        string        `json:"message"`
	Recommendation string        `json:"recommendation,omitempty"`
}

// ScoreCard holds the category and overall SEO health scores (0-100)
type ScoreCard struct {
	Overall   int `json:"overall"`
	Technical int `json:"technical"`
	Schema    int `json:"schema"`
	Content   int `json:"content"`
	Media     int `json:"media"`
}

// AuditData aggregates all page audit results for report rendering
type AuditData struct {
	URL         string                      `json:"url"`
	Host        string                      `json:"host"`
	GeneratedAt string                      `json:"generated_at"`
	Scores      ScoreCard                   `json:"scores"`
	Issues      []Issue                     `json:"issues"`
	Headers     *audit.HeaderAuditResult    `json:"headers"`
	Technical   *audit.TechnicalAuditReport `json:"technical"`
	Schema      *audit.SchemaAuditReport    `json:"schema"`
	Images      *audit.ImageAuditReport     `json:"images"`
	Content     *audit.ContentAuditReport   `json:"content"`
	Hreflang    *audit.HreflangAuditReport  `json:"hreflang"`
	LLMS        *audit.LLMSTxtReport        `json:"llms,omitempty"`
}

// BuildAuditData conducts on-page audits and computes category scorecards
func BuildAuditData(ctx context.Context, client *crawler.SafeClient, targetURL string) (*AuditData, error) {
	res, err := client.Fetch(ctx, targetURL)
	if err != nil {
		return nil, fmt.Errorf("fetching %s failed: %w", targetURL, err)
	}

	hdrAudit := audit.InspectHeaders(res)

	techAudit, err := audit.InspectHTML(targetURL, res.Body)
	if err != nil {
		return nil, fmt.Errorf("technical audit failed: %w", err)
	}

	schemaAudit := audit.InspectSchema(targetURL, res.Body)

	imagesAudit, err := audit.InspectImages(targetURL, res.Body)
	if err != nil {
		return nil, fmt.Errorf("images audit failed: %w", err)
	}

	contentAudit, err := audit.InspectContent(targetURL, res.Body, "")
	if err != nil {
		return nil, fmt.Errorf("content audit failed: %w", err)
	}

	hreflangAudit, err := audit.InspectHreflang(targetURL, res.Body)
	if err != nil {
		return nil, fmt.Errorf("hreflang audit failed: %w", err)
	}

	llmsReport, _ := audit.InspectLLMSTxt(ctx, client, targetURL)

	data := &AuditData{
		URL:         targetURL,
		Host:        extractHostSimple(targetURL),
		GeneratedAt: time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		Headers:     hdrAudit,
		Technical:   techAudit,
		Schema:      schemaAudit,
		Images:      imagesAudit,
		Content:     contentAudit,
		Hreflang:    hreflangAudit,
		LLMS:        llmsReport,
		Issues:      make([]Issue, 0),
	}

	computeScoresAndIssues(data)
	return data, nil
}

func extractHostSimple(u string) string {
	clean := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if idx := strings.IndexByte(clean, '/'); idx != -1 {
		return clean[:idx]
	}
	return clean
}

func computeScoresAndIssues(data *AuditData) {
	// Transfer underlying audit scores
	data.Scores.Technical = data.Technical.Score
	data.Scores.Schema = data.Schema.Score
	data.Scores.Content = data.Content.Score
	data.Scores.Media = data.Images.Score

	// 1. Technical Issues
	if data.Headers.StatusCode != 200 {
		data.Scores.Technical = clamp(data.Scores.Technical-30, 0, 100)
		data.Issues = append(data.Issues, Issue{
			Severity:       SeverityCritical,
			Category:       "Technical",
			Message:        fmt.Sprintf("HTTP status %d", data.Headers.StatusCode),
			Recommendation: "Ensure the page returns HTTP 200 OK for crawlers.",
		})
	} else {
		data.Issues = append(data.Issues, Issue{
			Severity: SeverityPass,
			Category: "Technical",
			Message:  "Page returned HTTP 200 OK",
		})
	}

	for _, iss := range data.Technical.Issues {
		sev := SeverityWarning
		if iss.Severity == audit.SeverityCritical {
			sev = SeverityCritical
		}
		data.Issues = append(data.Issues, Issue{
			Severity:       sev,
			Category:       "Technical",
			Message:        iss.Message,
			Recommendation: iss.Details,
		})
	}

	// 2. Schema Issues
	if data.Schema.BlocksCount == 0 {
		data.Issues = append(data.Issues, Issue{
			Severity:       SeverityCritical,
			Category:       "Schema",
			Message:        "No JSON-LD structured data detected",
			Recommendation: "Implement Schema.org JSON-LD (e.g. Organization, WebSite, Article, Product).",
		})
	} else {
		hasErrors := false
		for _, b := range data.Schema.Blocks {
			for _, errStr := range b.Errors {
				hasErrors = true
				data.Issues = append(data.Issues, Issue{
					Severity:       SeverityCritical,
					Category:       "Schema",
					Message:        fmt.Sprintf("%s: %s", strings.Join(b.Types, ","), errStr),
					Recommendation: "Resolve required schema fields for rich result eligibility.",
				})
			}
			for _, warnStr := range b.Warnings {
				data.Issues = append(data.Issues, Issue{
					Severity:       SeverityWarning,
					Category:       "Schema",
					Message:        fmt.Sprintf("%s: %s", strings.Join(b.Types, ","), warnStr),
					Recommendation: "Address recommended schema fields to avoid validation warnings.",
				})
			}
		}
		if !hasErrors {
			data.Issues = append(data.Issues, Issue{
				Severity: SeverityPass,
				Category: "Schema",
				Message:  fmt.Sprintf("Found %d valid structured data block(s)", data.Schema.BlocksCount),
			})
		}
	}

	// 3. Content Issues
	for _, iss := range data.Content.Issues {
		sev := SeverityWarning
		if iss.Severity == audit.SeverityCritical {
			sev = SeverityCritical
		}
		data.Issues = append(data.Issues, Issue{
			Severity:       sev,
			Category:       "Content",
			Message:        iss.Message,
			Recommendation: iss.Details,
		})
	}

	// 4. Media Issues
	for _, iss := range data.Images.Issues {
		sev := SeverityWarning
		if iss.Severity == audit.SeverityCritical {
			sev = SeverityCritical
		}
		data.Issues = append(data.Issues, Issue{
			Severity:       sev,
			Category:       "Media",
			Message:        iss.Message,
			Recommendation: iss.Details,
		})
	}

	// 5. Hreflang Issues
	for _, iss := range data.Hreflang.Issues {
		data.Issues = append(data.Issues, Issue{
			Severity:       SeverityWarning,
			Category:       "International",
			Message:        iss.Message,
			Recommendation: iss.Details,
		})
	}

	overall := int(float64(data.Scores.Technical)*0.30 +
		float64(data.Scores.Schema)*0.25 +
		float64(data.Scores.Content)*0.25 +
		float64(data.Scores.Media)*0.20)
	data.Scores.Overall = clamp(overall, 0, 100)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// RenderHTML generates a clean, executive print-ready HTML document
func RenderHTML(data *AuditData) (string, error) {
	funcMap := template.FuncMap{
		"scoreColor": func(s int) string {
			if s >= 80 {
				return "#10b981" // emerald
			}
			if s >= 50 {
				return "#f59e0b" // amber
			}
			return "#ef4444" // red
		},
		"severityBadge": func(sev IssueSeverity) template.HTML {
			switch sev {
			case SeverityCritical:
				return template.HTML(`<span class="badge badge-critical">CRITICAL</span>`)
			case SeverityWarning:
				return template.HTML(`<span class="badge badge-warning">WARNING</span>`)
			case SeverityPass:
				return template.HTML(`<span class="badge badge-pass">PASS</span>`)
			default:
				return template.HTML(`<span class="badge badge-info">INFO</span>`)
			}
		},
		"dashDash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		"join": strings.Join,
		"htmlSafe": func(s string) template.HTML {
			return template.HTML(html.EscapeString(s))
		},
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(reportHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// PDFRendererInfo contains details of the detected PDF tool
type PDFRendererInfo struct {
	Command string
	Type    string // "weasyprint", "chrome", "wkhtmltopdf"
}

// DetectPDFRenderer checks PATH for available PDF conversion engines
func DetectPDFRenderer() *PDFRendererInfo {
	if path, err := exec.LookPath("weasyprint"); err == nil {
		return &PDFRendererInfo{Command: path, Type: "weasyprint"}
	}
	for _, bin := range []string{"google-chrome", "chromium", "chromium-browser", "chrome", "msedge"} {
		if path, err := exec.LookPath(bin); err == nil {
			return &PDFRendererInfo{Command: path, Type: "chrome"}
		}
	}
	if path, err := exec.LookPath("wkhtmltopdf"); err == nil {
		return &PDFRendererInfo{Command: path, Type: "wkhtmltopdf"}
	}
	return nil
}

// ExportPDF renders HTML content to a PDF file using the best available renderer
func ExportPDF(ctx context.Context, htmlContent, outputPath string) (*PDFRendererInfo, error) {
	renderer := DetectPDFRenderer()
	if renderer == nil {
		return nil, fmt.Errorf("no PDF engine detected (install weasyprint or chromium)")
	}

	// Create a temp file for HTML input
	tmpDir := os.TempDir()
	tmpHTML := filepath.Join(tmpDir, fmt.Sprintf("seo_report_%d.html", time.Now().UnixNano()))
	if err := os.WriteFile(tmpHTML, []byte(htmlContent), 0o644); err != nil {
		return nil, fmt.Errorf("writing temporary HTML: %w", err)
	}
	defer os.Remove(tmpHTML)

	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		absOut = outputPath
	}

	var cmd *exec.Cmd
	switch renderer.Type {
	case "weasyprint":
		cmd = exec.CommandContext(ctx, renderer.Command, tmpHTML, absOut)
	case "chrome":
		cmd = exec.CommandContext(ctx, renderer.Command,
			"--headless=new",
			"--disable-gpu",
			"--no-sandbox",
			"--no-pdf-header-footer",
			fmt.Sprintf("--print-to-pdf=%s", absOut),
			tmpHTML,
		)
	case "wkhtmltopdf":
		cmd = exec.CommandContext(ctx, renderer.Command, "--enable-local-file-access", tmpHTML, absOut)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w (output: %s)", renderer.Type, err, string(out))
	}

	return renderer, nil
}

const reportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>SEO & GEO Audit Report — {{ .Host }}</title>
<style>
  :root {
    --bg-primary: #f8fafc;
    --bg-card: #ffffff;
    --text-primary: #0f172a;
    --text-secondary: #475569;
    --text-muted: #94a3b8;
    --border: #e2e8f0;
    --brand: #2563eb;
    --success: #10b981;
    --warning: #f59e0b;
    --danger: #ef4444;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    line-height: 1.5;
    padding: 32px 24px;
  }
  .container { max-width: 1080px; margin: 0 auto; }
  
  header {
    background: #0f172a;
    color: #ffffff;
    padding: 32px;
    border-radius: 12px;
    margin-bottom: 24px;
    box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1);
  }
  .header-top { display: flex; justify-content: space-between; align-items: baseline; flex-wrap: wrap; gap: 12px; }
  .badge-tag { background: #334155; padding: 4px 10px; border-radius: 9999px; font-size: 12px; font-weight: 600; letter-spacing: 0.5px; }
  h1 { font-size: 28px; font-weight: 700; margin-top: 8px; }
  .meta-url { color: #94a3b8; font-size: 14px; word-break: break-all; margin-top: 4px; }
  
  .score-banner {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 16px;
    margin-bottom: 24px;
  }
  .score-card {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    text-align: center;
    box-shadow: 0 1px 3px rgba(0,0,0,0.05);
  }
  .score-circle {
    width: 80px;
    height: 80px;
    border-radius: 50%;
    margin: 0 auto 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 28px;
    font-weight: 800;
    color: #ffffff;
  }
  .score-card.overall .score-circle { width: 96px; height: 96px; font-size: 34px; }
  .score-title { font-size: 13px; font-weight: 600; text-transform: uppercase; color: var(--text-secondary); }

  .section {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 24px;
    margin-bottom: 24px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.05);
  }
  .section-title {
    font-size: 18px;
    font-weight: 700;
    margin-bottom: 16px;
    padding-bottom: 10px;
    border-bottom: 2px solid var(--border);
    display: flex;
    align-items: center;
    gap: 8px;
  }

  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th { text-align: left; padding: 10px 12px; background: #f1f5f9; color: var(--text-secondary); font-weight: 600; border-bottom: 1px solid var(--border); }
  td { padding: 10px 12px; border-bottom: 1px solid var(--border); vertical-align: top; }
  tr:last-child td { border-bottom: none; }

  .badge {
    display: inline-block;
    padding: 3px 8px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
  }
  .badge-critical { background: #fee2e2; color: #991b1b; }
  .badge-warning  { background: #fef3c7; color: #92400e; }
  .badge-pass     { background: #d1fae5; color: #065f46; }
  .badge-info     { background: #e0f2fe; color: #075985; }

  .prop-grid {
    display: grid;
    grid-template-columns: 160px 1fr;
    gap: 8px 16px;
    font-size: 14px;
  }
  .prop-label { font-weight: 600; color: var(--text-secondary); }
  .prop-val { word-break: break-all; }

  footer {
    text-align: center;
    color: var(--text-muted);
    font-size: 12px;
    margin-top: 32px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }

  @media print {
    body { background: #ffffff; padding: 0; font-size: 12pt; }
    .container { max-width: 100%; }
    .section, .score-card { page-break-inside: avoid; border: 1px solid #cbd5e1; box-shadow: none; }
    header { background: #1e293b !important; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    .score-circle { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    @page { margin: 1.5cm; size: A4 portrait; }
  }
</style>
</head>
<body>
<div class="container">
  <header>
    <div class="header-top">
      <span class="badge-tag">EXECUTIVE AUDIT</span>
      <span style="font-size: 13px; color: #94a3b8;">Generated {{ .GeneratedAt }}</span>
    </div>
    <h1>{{ .Host }}</h1>
    <div class="meta-url">{{ .URL }}</div>
  </header>

  <!-- Score Cards -->
  <div class="score-banner">
    <div class="score-card overall">
      <div class="score-circle" style="background: {{ scoreColor .Scores.Overall }}">{{ .Scores.Overall }}</div>
      <div class="score-title">Overall Score</div>
    </div>
    <div class="score-card">
      <div class="score-circle" style="background: {{ scoreColor .Scores.Technical }}">{{ .Scores.Technical }}</div>
      <div class="score-title">Technical</div>
    </div>
    <div class="score-card">
      <div class="score-circle" style="background: {{ scoreColor .Scores.Schema }}">{{ .Scores.Schema }}</div>
      <div class="score-title">Schema & Rich</div>
    </div>
    <div class="score-card">
      <div class="score-circle" style="background: {{ scoreColor .Scores.Content }}">{{ .Scores.Content }}</div>
      <div class="score-title">Content & E-E-A-T</div>
    </div>
    <div class="score-card">
      <div class="score-circle" style="background: {{ scoreColor .Scores.Media }}">{{ .Scores.Media }}</div>
      <div class="score-title">Media & Images</div>
    </div>
  </div>

  <!-- Key Findings & Recommendations -->
  <div class="section">
    <h2 class="section-title">Audit Action Items & Findings</h2>
    <table>
      <thead>
        <tr>
          <th style="width: 100px;">Status</th>
          <th style="width: 120px;">Category</th>
          <th>Finding</th>
          <th>Recommended Action</th>
        </tr>
      </thead>
      <tbody>
        {{ range .Issues }}
        <tr>
          <td>{{ severityBadge .Severity }}</td>
          <td><strong>{{ .Category }}</strong></td>
          <td>{{ .Message }}</td>
          <td>{{ dashDash .Recommendation }}</td>
        </tr>
        {{ end }}
      </tbody>
    </table>
  </div>

  <!-- Technical SEO Breakdown -->
  <div class="section">
    <h2 class="section-title">Technical & Meta Signals</h2>
    <div class="prop-grid">
      <div class="prop-label">HTTP Status</div>
      <div class="prop-val">{{ .Headers.StatusCode }}</div>

      <div class="prop-label">Redirects</div>
      <div class="prop-val">{{ if .Headers.RedirectHops }}{{ len .Headers.RedirectChain }} hops{{ else }}None (Direct){{ end }}</div>

      <div class="prop-label">Title Tag</div>
      <div class="prop-val"><strong>{{ dashDash .Technical.Title }}</strong> ({{ .Technical.TitleLength }} chars)</div>

      <div class="prop-label">Meta Description</div>
      <div class="prop-val">{{ dashDash .Technical.MetaDescription }} ({{ .Technical.MetaDescLength }} chars)</div>

      <div class="prop-label">Canonical URL</div>
      <div class="prop-val">{{ dashDash .Technical.Canonical }}</div>

      <div class="prop-label">Robots Tag</div>
      <div class="prop-val">{{ dashDash .Technical.MetaRobots }}</div>

      <div class="prop-label">OpenGraph Title</div>
      <div class="prop-val">{{ dashDash .Technical.OGTitle }}</div>

      <div class="prop-label">OpenGraph Image</div>
      <div class="prop-val">{{ dashDash .Technical.OGImage }}</div>
    </div>
  </div>

  <!-- Structured Data Schema -->
  <div class="section">
    <h2 class="section-title">Structured Data (JSON-LD)</h2>
    {{ if .Schema.Blocks }}
    <table>
      <thead>
        <tr>
          <th>Types</th>
          <th>Context</th>
          <th>Status</th>
          <th>Validation Notes</th>
        </tr>
      </thead>
      <tbody>
        {{ range .Schema.Blocks }}
        <tr>
          <td><strong>{{ join .Types ", " }}</strong></td>
          <td>{{ dashDash .Context }}</td>
          <td>{{ if .IsValid }}<span class="badge badge-pass">Valid</span>{{ else }}<span class="badge badge-critical">Invalid</span>{{ end }}</td>
          <td>
            {{ if .Errors }}
              {{ range .Errors }}<div style="color: #ef4444;">• {{ . }}</div>{{ end }}
            {{ else if .Warnings }}
              {{ range .Warnings }}<div style="color: #f59e0b;">• {{ . }}</div>{{ end }}
            {{ else }}
              <span style="color: #10b981; font-weight: 600;">Clean Pass</span>
            {{ end }}
          </td>
        </tr>
        {{ end }}
      </tbody>
    </table>
    {{ else }}
    <p style="color: #94a3b8; font-style: italic;">No JSON-LD structured data detected on this page.</p>
    {{ end }}
  </div>

  <!-- Content & Headings -->
  <div class="section">
    <h2 class="section-title">Content & Heading Structure</h2>
    <div class="prop-grid" style="margin-bottom: 16px;">
      <div class="prop-label">Word Count</div>
      <div class="prop-val">{{ .Content.WordCount }} words (~{{ .Content.ReadingTimeMin }} min read)</div>

      <div class="prop-label">E-E-A-T Author</div>
      <div class="prop-val">{{ if .Content.HasAuthorByline }}Detected{{ if .Content.AuthorSignals }} ({{ join .Content.AuthorSignals ", " }}){{ end }}{{ else }}Not detected{{ end }}</div>

      <div class="prop-label">Publication Date</div>
      <div class="prop-val">{{ if .Content.HasPublishDate }}Detected{{ if .Content.DateSignals }} ({{ join .Content.DateSignals ", " }}){{ end }}{{ else }}Not detected{{ end }}</div>
    </div>

    <h3 style="font-size: 14px; font-weight: 700; margin-bottom: 8px; color: var(--text-secondary);">Heading Outline</h3>
    <ul style="padding-left: 20px; font-size: 14px;">
      {{ range .Content.Headings }}
      <li><strong>H{{ .Level }}:</strong> {{ .Text }}</li>
      {{ end }}
    </ul>
  </div>

  <!-- Images Audit -->
  <div class="section">
    <h2 class="section-title">Images & Visual Assets</h2>
    <div class="prop-grid" style="margin-bottom: 16px;">
      <div class="prop-label">Total Images</div>
      <div class="prop-val">{{ .Images.TotalImages }}</div>

      <div class="prop-label">Missing Alt</div>
      <div class="prop-val">{{ .Images.MissingAlt }} images missing alt text</div>

      <div class="prop-label">Missing Dimensions</div>
      <div class="prop-val">{{ .Images.MissingDimensions }} images missing width/height (CLS risk)</div>

      <div class="prop-label">Modern Formats</div>
      <div class="prop-val">{{ .Images.ModernFormats }} WebP/AVIF vs {{ .Images.LegacyFormats }} legacy</div>
    </div>
  </div>

  <!-- AI Discovery & LLMS -->
  {{ if .LLMS }}
  <div class="section">
    <h2 class="section-title">AI Discovery & llms.txt</h2>
    <div class="prop-grid">
      <div class="prop-label">llms.txt Score</div>
      <div class="prop-val"><strong>{{ .LLMS.Score }}/100</strong></div>

      <div class="prop-label">/llms.txt Status</div>
      <div class="prop-val">{{ if .LLMS.LLMSTxtExists }}Present ({{ .LLMS.LLMSTxtBytes }} bytes, {{ .LLMS.LLMSTxtLinks }} links){{ else }}Not found (HTTP {{ .LLMS.LLMSTxtStatus }}){{ end }}</div>

      <div class="prop-label">/llms-full.txt Status</div>
      <div class="prop-val">{{ if .LLMS.FullTxtExists }}Present ({{ .LLMS.FullTxtBytes }} bytes){{ else }}Not found (HTTP {{ .LLMS.FullTxtStatus }}){{ end }}</div>
    </div>
  </div>
  {{ end }}

  <footer>
    Antigravity SEO Engine — Standalone Audit Report
  </footer>
</div>
</body>
</html>
`
