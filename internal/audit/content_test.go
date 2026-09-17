package audit

import (
	"strings"
	"testing"
)

var contentHTML = `
<html><head>
<meta name="author" content="Yuki Tanaka">
<meta property="article:published_time" content="2026-01-15T09:00:00Z">
</head><body>
<h1>Tokyo Property Guide</h1>
<p>` + strings.Repeat("Tokyo property investment offers stable returns and low holding taxes. ", 2) + `</p>
<h2>Overview</h2>
<p>` + strings.Repeat("word ", 50) + `</p>
<h4>Skipped level</h4>
<p>` + strings.Repeat("answer ", 12) + `</p>
<ul><li>point one</li></ul>
</body></html>`

func TestInspectContentSignals(t *testing.T) {
	report, err := InspectContent("https://example.com/guide", []byte(contentHTML), "")
	if err != nil {
		t.Fatal(err)
	}

	if !report.HasAuthorByline {
		t.Error("meta author should set HasAuthorByline")
	}
	if !report.HasPublishDate {
		t.Error("article:published_time should set HasPublishDate")
	}
	if report.ListCount != 1 {
		t.Errorf("expected 1 list, got %d", report.ListCount)
	}
	if len(report.Headings) == 0 || report.Headings[0].Level != 1 {
		t.Errorf("expected h1 first, got %+v", report.Headings)
	}
	foundSkip := false
	for i := 1; i < len(report.Headings); i++ {
		if report.Headings[i].Level-report.Headings[i-1].Level > 1 {
			foundSkip = true
		}
	}
	if !foundSkip || report.SkippedLevels == 0 {
		t.Error("h2 -> h4 skip should be detected")
	}
	if report.AnswerBlocks == 0 {
		t.Error("60-word paragraph should count as answer block")
	}
	if report.WordCount < 80 {
		t.Errorf("word count too low: %d", report.WordCount)
	}
}

func TestInspectContentKeyword(t *testing.T) {
	html := `<html><body><h1>t</h1><p>` + strings.Repeat("tokyo property ", 10) + strings.Repeat("filler ", 90) + `</p></body></html>`
	report, err := InspectContent("https://example.com", []byte(html), "tokyo property")
	if err != nil {
		t.Fatal(err)
	}
	if report.KeywordCount == 0 || report.KeywordDensity <= 0 {
		t.Errorf("keyword analysis failed: count=%d density=%.2f", report.KeywordCount, report.KeywordDensity)
	}
	if !report.KeywordInLede {
		t.Error("keyword appears in first 100 words, KeywordInLede should be true")
	}
}

func TestInspectContentThinPage(t *testing.T) {
	report, err := InspectContent("https://example.com", []byte("<html><body><p>hi</p></body></html>"), "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Score > 60 {
		t.Errorf("thin page with no author/date/headings should score low, got %d", report.Score)
	}
}
