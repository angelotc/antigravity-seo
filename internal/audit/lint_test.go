package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintBlocksPlaceholderValues(t *testing.T) {
	html := `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"LocalBusiness","name":"[Business Name]","url":"https://example.com"}
</script></head></html>`

	report := LintSchemaSource("test.html", []byte(html))
	if !report.Blocked {
		t.Fatalf("expected placeholder to block, findings: %+v", report.Findings)
	}
	if len(report.Findings) != 1 || !strings.Contains(report.Findings[0].Message, "placeholder") {
		t.Errorf("unexpected findings: %+v", report.Findings)
	}
}

func TestLintBlocksDeprecatedTypes(t *testing.T) {
	for _, typ := range []string{"HowTo", "SpecialAnnouncement", "ClaimReview", "VehicleListing", "EstimatedSalary", "LearningVideo", "CourseInfo"} {
		html := `<script type="application/ld+json">{"@context":"https://schema.org","@type":"` + typ + `","name":"x"}</script>`
		report := LintSchemaSource("t.html", []byte(html))
		if !report.Blocked {
			t.Errorf("deprecated type %s should block; findings: %+v", typ, report.Findings)
		}
	}
}

func TestLintWarnsOnInvalidJSON(t *testing.T) {
	report := LintSchemaSource("t.html", []byte(`<script type="application/ld+json">{broken</script>`))
	if report.Blocked {
		t.Error("invalid JSON should warn, not block")
	}
	if len(report.Findings) == 0 || report.Findings[0].Severity != "warn" {
		t.Errorf("expected warn finding, got %+v", report.Findings)
	}
}

func TestLintCleanPass(t *testing.T) {
	html := `<script type="application/ld+json">{"@context":"https://schema.org","@type":"Organization","name":"Acme","url":"https://acme.com"}</script>`
	report := LintSchemaSource("t.html", []byte(html))
	if report.Blocked || len(report.Findings) != 0 {
		t.Errorf("clean schema should have zero findings, got %+v", report.Findings)
	}
}

func TestLintSchemaFileSkipsNonHTML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "styles.css")
	if err := os.WriteFile(p, []byte("body { color: red }"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := LintSchemaFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if report.Checked {
		t.Error("css file must be skipped")
	}
}

func TestLintSchemaFileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "page.html")
	body := []byte(`<html><script type="application/ld+json">{"@context":"https://schema.org","@type":"Product","name":"REPLACE_ME_PRODUCT"}</script></html>`)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := LintSchemaFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Checked || !report.Blocked {
		t.Errorf("expected checked+blocked, got %+v", report)
	}
}

func TestLintRawJSONLDInput(t *testing.T) {
	report := LintSchemaSource("block.jsonld", []byte(`{"@context":"https://schema.org","@type":"Event","name":"Ok"}`))
	if report.Blocks != 1 {
		t.Fatalf("raw JSON-LD input should be treated as one block, got %d", report.Blocks)
	}
	if report.Blocked {
		t.Errorf("valid Event should not block: %+v", report.Findings)
	}
}
