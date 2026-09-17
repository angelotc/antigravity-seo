package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// SchemaBlock represents a single JSON-LD block extracted from a page
type SchemaBlock struct {
	RawJSON   string                 `json:"raw_json"`
	IsValid   bool                   `json:"is_valid"`
	ParseErr  string                 `json:"parse_error,omitempty"`
	Context   string                 `json:"context,omitempty"`
	Types     []string               `json:"types"`
	ParsedData map[string]interface{} `json:"parsed_data,omitempty"`
	Warnings  []string               `json:"warnings,omitempty"`
	Errors    []string               `json:"errors,omitempty"`
}

// SchemaAuditReport provides a complete evaluation of structured data
type SchemaAuditReport struct {
	URL         string        `json:"url"`
	BlocksCount int           `json:"blocks_count"`
	TypesFound  []string      `json:"types_found"`
	Blocks      []SchemaBlock `json:"blocks"`
	Score       int           `json:"score"` // 0 - 100
	Summary     string        `json:"summary"`
}

// ExtractJSONLDBlocks extracts all application/ld+json scripts from HTML
func ExtractJSONLDBlocks(rawHTML []byte) ([]string, error) {
	doc, err := html.Parse(bytes.NewReader(rawHTML))
	if err != nil {
		return nil, fmt.Errorf("failed parsing HTML for JSON-LD: %w", err)
	}

	var blocks []string
	var crawler func(*html.Node)

	crawler = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, attr := range n.Attr {
				if strings.ToLower(attr.Key) == "type" && strings.ToLower(strings.TrimSpace(attr.Val)) == "application/ld+json" {
					var sb strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == html.TextNode || c.Type == html.RawNode {
							sb.WriteString(c.Data)
						}
					}
					content := strings.TrimSpace(sb.String())
					if content != "" {
						blocks = append(blocks, content)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			crawler(c)
		}
	}

	crawler(doc)
	return blocks, nil
}

// InspectSchema parses and audits JSON-LD blocks against Google Rich Results standards
func InspectSchema(url string, rawHTML []byte) *SchemaAuditReport {
	rawBlocks, _ := ExtractJSONLDBlocks(rawHTML)

	report := &SchemaAuditReport{
		URL:         url,
		BlocksCount: len(rawBlocks),
		TypesFound:  make([]string, 0),
		Blocks:      make([]SchemaBlock, 0),
		Score:       100,
	}

	if len(rawBlocks) == 0 {
		report.Score = 0
		report.Summary = "No structured data (JSON-LD) detected on the page."
		return report
	}

	for _, raw := range rawBlocks {
		block := SchemaBlock{
			RawJSON:  raw,
			Types:    make([]string, 0),
			Warnings: make([]string, 0),
			Errors:   make([]string, 0),
		}

		// Try parsing as object
		var obj map[string]interface{}
		err := json.Unmarshal([]byte(raw), &obj)

		if err != nil {
			// Try parsing as array
			var arr []map[string]interface{}
			if arrErr := json.Unmarshal([]byte(raw), &arr); arrErr != nil {
				block.IsValid = false
				block.ParseErr = fmt.Sprintf("Invalid JSON syntax: %v", err)
				block.Errors = append(block.Errors, block.ParseErr)
				report.Score -= 30
				report.Blocks = append(report.Blocks, block)
				continue
			}
			block.IsValid = true
			for _, item := range arr {
				extractTypesAndValidate(item, &block, report)
			}
		} else {
			block.IsValid = true
			block.ParsedData = obj
			if ctx, ok := obj["@context"].(string); ok {
				block.Context = ctx
			}

			// Check for @graph
			if graph, ok := obj["@graph"].([]interface{}); ok {
				for _, item := range graph {
					if itemMap, ok := item.(map[string]interface{}); ok {
						extractTypesAndValidate(itemMap, &block, report)
					}
				}
			} else {
				extractTypesAndValidate(obj, &block, report)
			}
		}

		report.Blocks = append(report.Blocks, block)
	}

	if report.Score < 0 {
		report.Score = 0
	}

	report.Summary = fmt.Sprintf("Found %d JSON-LD block(s) with types: %s. Score: %d/100",
		report.BlocksCount, strings.Join(report.TypesFound, ", "), report.Score)

	return report
}

func extractTypesAndValidate(item map[string]interface{}, block *SchemaBlock, report *SchemaAuditReport) {
	typeVal, hasType := item["@type"]
	if !hasType {
		block.Warnings = append(block.Warnings, "Object missing '@type' property")
		return
	}

	var types []string
	switch v := typeVal.(type) {
	case string:
		types = append(types, v)
	case []interface{}:
		for _, t := range v {
			if ts, ok := t.(string); ok {
				types = append(types, ts)
			}
		}
	}

	for _, t := range types {
		if !containsString(block.Types, t) {
			block.Types = append(block.Types, t)
		}
		if !containsString(report.TypesFound, t) {
			report.TypesFound = append(report.TypesFound, t)
		}

		// Validate specific Google Rich Result types
		validateRichResultType(t, item, block, report)
	}
}

func validateRichResultType(schemaType string, data map[string]interface{}, block *SchemaBlock, report *SchemaAuditReport) {
	switch schemaType {
	case "Article", "NewsArticle", "BlogPosting":
		checkRequiredField(data, "headline", schemaType, block, report)
		checkRequiredField(data, "datePublished", schemaType, block, report)
		checkRequiredField(data, "author", schemaType, block, report)
		checkRequiredField(data, "image", schemaType, block, report)

	case "Product":
		checkRequiredField(data, "name", schemaType, block, report)
		checkRequiredField(data, "image", schemaType, block, report)
		checkRequiredField(data, "offers", schemaType, block, report)

	case "BreadcrumbList":
		checkRequiredField(data, "itemListElement", schemaType, block, report)

	case "FAQPage":
		checkRequiredField(data, "mainEntity", schemaType, block, report)

	case "Organization", "LocalBusiness":
		checkRequiredField(data, "name", schemaType, block, report)
		checkRequiredField(data, "url", schemaType, block, report)
	}
}

func checkRequiredField(data map[string]interface{}, field string, schemaType string, block *SchemaBlock, report *SchemaAuditReport) {
	val, ok := data[field]
	if !ok || val == nil || val == "" {
		msg := fmt.Sprintf("%s is missing recommended Google Rich Results property: '%s'", schemaType, field)
		block.Warnings = append(block.Warnings, msg)
		report.Score -= 5
	}
}

func containsString(arr []string, target string) bool {
	for _, s := range arr {
		if s == target {
			return true
		}
	}
	return false
}

