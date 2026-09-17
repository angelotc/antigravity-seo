package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MaxLintFileBytes caps files the linter will read (10MB, matching upstream gate)
const MaxLintFileBytes = 10 * 1024 * 1024

// deprecatedSchemaTypes no longer earn Google rich results and should not be emitted
var deprecatedSchemaTypes = map[string]bool{
	"HowTo":             true,
	"SpecialAnnouncement": true,
	"ClaimReview":       true,
	"VehicleListing":    true,
	"EstimatedSalary":   true,
	"LearningVideo":     true,
	"CourseInfo":        true,
}

var placeholderPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\[business name\]`),
	regexp.MustCompile(`(?i)\[your [^\]]+\]`),
	regexp.MustCompile(`(?i)replace[_-]`),
	regexp.MustCompile(`(?i)\btodo\b`),
	regexp.MustCompile(`(?i)\bfixme\b`),
}

// LintFinding is a single schema quality-gate violation
type LintFinding struct {
	Severity string `json:"severity"` // "block" or "warn"
	Block    int    `json:"block"`    // 1-based JSON-LD block index
	Message  string `json:"message"`
}

// LintReport is the outcome of linting one file for JSON-LD quality
type LintReport struct {
	File       string        `json:"file"`
	Checked    bool          `json:"checked"`
	SkipReason string        `json:"skip_reason,omitempty"`
	Blocks     int           `json:"blocks"`
	Findings   []LintFinding `json:"findings"`
	Blocked    bool          `json:"blocked"`
}

// IsHTMLLikeFile reports whether a path could contain HTML with embedded JSON-LD
func IsHTMLLikeFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".php", ".erb", ".hbs", ".ejs", ".liquid", ".astro", ".vue", ".jsx", ".tsx", ".md", ".mdx", ".json", ".jsonld":
		return true
	}
	return false
}

// LintSchemaFile reads a file and applies the JSON-LD quality gate.
func LintSchemaFile(path string) (*LintReport, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot stat %s: %w", path, err)
	}
	if info.Size() > MaxLintFileBytes {
		return &LintReport{File: path, Checked: false, SkipReason: "file exceeds 10MB lint cap"}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	if !IsHTMLLikeFile(path) && !looksLikeJSONLD(raw) {
		return &LintReport{File: path, Checked: false, SkipReason: "not an HTML-like or JSON-LD file"}, nil
	}

	return LintSchemaSource(path, raw), nil
}

// LintSchemaSource applies the JSON-LD quality gate to raw HTML or raw JSON-LD content.
func LintSchemaSource(name string, raw []byte) *LintReport {
	report := &LintReport{File: name, Checked: true, Findings: []LintFinding{}}

	var blocks []string
	if looksLikeJSONLD(raw) && !strings.Contains(strings.ToLower(string(raw)), "<script") {
		blocks = []string{strings.TrimSpace(string(raw))}
	} else {
		blocks, _ = ExtractJSONLDBlocks(raw)
	}
	report.Blocks = len(blocks)
	if len(blocks) == 0 {
		return report
	}

	for i, rawBlock := range blocks {
		var parsed interface{}
		if err := json.Unmarshal([]byte(rawBlock), &parsed); err != nil {
			report.Findings = append(report.Findings, LintFinding{
				Severity: "warn", Block: i + 1,
				Message: fmt.Sprintf("invalid JSON syntax: %v", err),
			})
			continue
		}

		for _, obj := range flattenSchemaObjects(parsed) {
			lintSchemaObject(obj, i+1, report)
		}
	}

	for _, f := range report.Findings {
		if f.Severity == "block" {
			report.Blocked = true
			break
		}
	}
	return report
}

// flattenSchemaObjects expands arrays and @graph containers into individual objects
func flattenSchemaObjects(v interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	switch val := v.(type) {
	case map[string]interface{}:
		out = append(out, val)
		if graph, ok := val["@graph"].([]interface{}); ok {
			for _, g := range graph {
				out = append(out, flattenSchemaObjects(g)...)
			}
		}
	case []interface{}:
		for _, item := range val {
			out = append(out, flattenSchemaObjects(item)...)
		}
	}
	return out
}

func lintSchemaObject(obj map[string]interface{}, blockNum int, report *LintReport) {
	typeVal, hasType := obj["@type"]
	if !hasType {
		report.Findings = append(report.Findings, LintFinding{
			Severity: "warn", Block: blockNum,
			Message: "JSON-LD object is missing '@type'",
		})
	} else {
		for _, t := range coerceStringSlice(typeVal) {
			if deprecatedSchemaTypes[t] {
				report.Findings = append(report.Findings, LintFinding{
					Severity: "block", Block: blockNum,
					Message: fmt.Sprintf("deprecated schema type '%s' no longer earns Google rich results", t),
				})
			}
		}
	}

	if _, hasContext := obj["@context"]; !hasTypeOnlyGraph(obj) && !hasContext {
		report.Findings = append(report.Findings, LintFinding{
			Severity: "warn", Block: blockNum,
			Message: "JSON-LD object is missing '@context'",
		})
	}

	for _, ph := range findPlaceholders(obj, "") {
		report.Findings = append(report.Findings, LintFinding{
			Severity: "block", Block: blockNum,
			Message: fmt.Sprintf("placeholder value '%s' left in schema property '%s'", ph.value, ph.path),
		})
	}
}

// hasTypeOnlyGraph allows @context to live only on the wrapping @graph node
func hasTypeOnlyGraph(obj map[string]interface{}) bool {
	_, hasGraph := obj["@graph"]
	_, hasType := obj["@type"]
	return hasGraph && !hasType
}

type placeholderHit struct {
	path  string
	value string
}

func findPlaceholders(v interface{}, path string) []placeholderHit {
	var hits []placeholderHit
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			hits = append(hits, findPlaceholders(child, k)...)
		}
	case []interface{}:
		for _, child := range val {
			hits = append(hits, findPlaceholders(child, path)...)
		}
	case string:
		for _, pat := range placeholderPatterns {
			if pat.MatchString(val) {
				hits = append(hits, placeholderHit{path: path, value: truncate(val, 60)})
				break
			}
		}
	}
	return hits
}

func looksLikeJSONLD(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func coerceStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
