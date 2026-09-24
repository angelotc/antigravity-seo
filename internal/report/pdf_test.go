package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"antigravity-seo/internal/audit"
)

// japaneseAuditData builds a synthetic AuditData with Japanese text in the
// title, meta description, and an issue message/recommendation, exercising
// every fpdf.MultiCell/Cell call path that previously used the core "Arial"
// (Latin-1) font and would have produced mojibake or a rendering error.
func japaneseAuditData() *AuditData {
	return &AuditData{
		URL:         "https://example.co.jp/物件/東京",
		FinalURL:    "https://www.example.co.jp/物件/東京",
		Host:        "www.example.co.jp",
		GeneratedAt: "2026-09-24 00:00:00 UTC",
		Scores: ScoreCard{
			Overall: 72, Technical: 70, Schema: 80, Content: 65, Media: 75,
		},
		Issues: []Issue{
			{
				Severity:       SeverityCritical,
				Category:       "Indexability",
				Message:        "X-Robots-Tag に「noindex」が含まれています",
				Recommendation: "検索エンジンがこのページをインデックスできるように、noindexディレクティブを削除してください。",
			},
			{
				Severity: SeverityPass,
				Category: "Technical",
				Message:  "ページは200 OKを返しました",
			},
		},
		Headers: &audit.HeaderAuditResult{
			StatusCode: 200,
			Server:     "nginx",
			TTFBMS:     120,
		},
		Technical: &audit.TechnicalAuditReport{
			Title:           "東京の不動産ガイド — 高級住宅を見つける",
			MetaDescription: "東京の不動産情報、価格動向、海外バイヤー向けの地域ガイドをご覧ください。",
			Canonical:       "https://www.example.co.jp/物件/東京",
			MetaRobots:      "index, follow",
			H1Count:         1, H2Count: 3, H3Count: 2,
		},
		Content: &audit.ContentAuditReport{
			WordCount: 850, ReadingTimeMin: 4, ParagraphCount: 12,
		},
		Schema: &audit.SchemaAuditReport{
			BlocksCount: 1,
			Blocks: []audit.SchemaBlock{
				{Types: []string{"RealEstateListing"}, IsValid: true},
			},
		},
		Images: &audit.ImageAuditReport{
			TotalImages: 8, MissingAlt: 1,
		},
		Hreflang: &audit.HreflangAuditReport{
			Count: 2,
		},
	}
}

func TestRenderNativePDFJapanese(t *testing.T) {
	data := japaneseAuditData()

	pdfBytes, err := RenderNativePDFBytes(data)
	if err != nil {
		t.Fatalf("RenderNativePDFBytes with Japanese content failed: %v", err)
	}
	if len(pdfBytes) < 2048 {
		t.Fatalf("expected PDF bytes >= 2KB, got %d", len(pdfBytes))
	}
	if !strings.HasPrefix(string(pdfBytes[:5]), "%PDF-") {
		t.Errorf("expected %%PDF- magic header, got %q", string(pdfBytes[:5]))
	}

	tmpDir := t.TempDir()
	outPDF := filepath.Join(tmpDir, "sample-ja.pdf")
	if err := os.WriteFile(outPDF, pdfBytes, 0o644); err != nil {
		t.Fatalf("writing sample PDF: %v", err)
	}

	pdftotext, lookErr := exec.LookPath("pdftotext")
	if lookErr != nil {
		t.Skip("pdftotext not installed, skipping Japanese round-trip text extraction check")
	}

	outTxt := filepath.Join(tmpDir, "sample-ja.txt")
	cmd := exec.Command(pdftotext, "-enc", "UTF-8", outPDF, outTxt)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pdftotext failed: %v (output: %s)", err, out)
	}

	extracted, err := os.ReadFile(outTxt)
	if err != nil {
		t.Fatalf("reading extracted text: %v", err)
	}
	text := string(extracted)

	for _, want := range []string{"東京", "不動産", "noindex", "インデックス"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected extracted PDF text to contain %q, got:\n%s", want, text)
		}
	}
}

func TestTruncateStringJapanese(t *testing.T) {
	s := "東京の不動産ガイド：高級住宅を見つける完全なガイドです"
	runeCount := utf8.RuneCountInString(s)

	got := truncateString(s, 10)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateString produced invalid UTF-8: %q", got)
	}
	gotRunes := utf8.RuneCountInString(got)
	if gotRunes != 10 {
		t.Errorf("expected truncated result to be exactly 10 runes, got %d (%q)", gotRunes, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated string to end with '...', got %q", got)
	}

	// String shorter than maxLen is returned unchanged.
	short := "東京"
	if out := truncateString(short, 10); out != short {
		t.Errorf("expected short string unchanged, got %q", out)
	}

	// Sanity: the byte length differs from the rune length for Japanese text,
	// so a byte-slicing truncateString would have split a multi-byte rune.
	if len(s) == runeCount {
		t.Fatalf("test fixture must contain multi-byte runes")
	}
}
