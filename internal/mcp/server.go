package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"antigravity-seo/internal/api"
	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
	"antigravity-seo/internal/report"
)

// JSONRPCRequest represents an incoming MCP request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing MCP response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError captures JSON-RPC error payload
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool represents an MCP tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// StartMCPServer runs the stdio JSON-RPC server for Antigravity, Claude Code, and Codex
func StartMCPServer(version string) error {
	return Serve(os.Stdin, os.Stdout, version)
}

// Serve answers MCP JSON-RPC requests from r, writing responses to w.
// Split from StartMCPServer so the protocol loop is testable.
func Serve(r io.Reader, w io.Writer, version string) error {
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: 20 * time.Second,
	})

	scanner := bufio.NewScanner(r)
	// Allow large payloads (up to 10MB)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	tools := []Tool{
		{
			Name:        "seo_inspect_headers",
			Description: "Examine HTTP response headers, 301/302 redirect chains, X-Robots-Tag, canonical headers, and TTFB for a URL.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The target website URL to inspect.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_audit_page",
			Description: "Comprehensive on-page technical SEO audit: titles, meta tags, headings, canonicals, schema JSON-LD, and image alt text.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL of the webpage to audit.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_inspect_sitemap",
			Description: "Stream, parse, and validate an XML sitemap or sitemapindex. Detects broken URLs, size limits, and cross-origin links.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The full URL of the XML sitemap (e.g. https://example.com/sitemap.xml).",
					},
					"check_limit": map[string]interface{}{
						"type":        "integer",
						"description": "Number of sample URLs to live-verify HTTP status for (default: 10, max: 100).",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_inspect_robots",
			Description: "Audit robots.txt directives and evaluate access status for search engines and AI crawlers (Google-Extended, GPTBot, ClaudeBot).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The robots.txt URL or domain homepage (e.g. https://example.com/robots.txt).",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_inspect_schema",
			Description: "Extract and validate JSON-LD structured data against Google Rich Results requirements (required properties, deprecated types, placeholders).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL of the page to validate structured data on.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_audit_images",
			Description: "Image optimization audit: alt coverage and quality, width/height (CLS risk), lazy-loading, modern formats, insecure and oversized inline images.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL of the page whose images should be audited.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_audit_content",
			Description: "Content quality and E-E-A-T audit: word count, reading time, heading hierarchy, author byline and date signals, GEO answer blocks, optional keyword density.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL of the page to analyze.",
					},
					"keyword": map[string]interface{}{
						"type":        "string",
						"description": "Optional target keyword for density and placement analysis.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_audit_hreflang",
			Description: "International hreflang audit: BCP47 code validation, x-default presence, self-reference, and duplicate declarations.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL of the page to audit hreflang annotations on.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_generate_report",
			Description: "Generate an executive SEO & GEO audit report data structure with scores, categorized findings, and issue breakdowns.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{
						"type":        "string",
						"description": "The URL of the webpage to audit and report on.",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "seo_query_backlinks",
			Description: "Query open Common Crawl web graph and CDX index for domain captures, MIME distribution, and indexed pages (keyless).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"domain": map[string]interface{}{
						"type":        "string",
						"description": "The domain or URL to inspect in Common Crawl (e.g. example.com).",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of records to return (default: 50).",
					},
				},
				"required": []string{"domain"},
			},
		},
		{
			Name:        "seo_gsc_query",
			Description: "Query Google Search Console Search Analytics for clicks, impressions, CTR, and average position.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"site_url": map[string]interface{}{
						"type":        "string",
						"description": "The GSC site property URL (e.g. sc-domain:example.com or https://example.com/).",
					},
					"dimensions": map[string]interface{}{
						"type":        "string",
						"description": "Comma-separated dimensions: query, page, device, country, date (default: query).",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of rows to return (default: 25).",
					},
				},
				"required": []string{"site_url"},
			},
		},
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(w, nil, -32700, "Parse error")
			continue
		}

		switch req.Method {
		case "initialize":
			sendResponse(w, req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]interface{}{
					"name":    "antigravity-seo-engine",
					"version": version,
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
			})

		case "notifications/initialized":
			// Acknowledged by client

		case "ping":
			sendResponse(w, req.ID, map[string]interface{}{})

		case "tools/list":
			sendResponse(w, req.ID, map[string]interface{}{
				"tools": tools,
			})

		case "tools/call":
			handleToolCall(context.Background(), client, w, req.ID, req.Params)

		default:
			sendError(w, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
		}
	}

	return scanner.Err()
}

func handleToolCall(ctx context.Context, client *crawler.SafeClient, w io.Writer, id interface{}, paramsRaw json.RawMessage) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		sendError(w, id, -32602, "Invalid params")
		return
	}

	urlStr, _ := params.Arguments["url"].(string)
	if urlStr == "" {
		if u, ok := params.Arguments["domain"].(string); ok {
			urlStr = u
		} else if u, ok := params.Arguments["site_url"].(string); ok {
			urlStr = u
		}
	}
	if urlStr == "" {
		sendError(w, id, -32602, "Missing required argument ('url', 'domain', or 'site_url')")
		return
	}

	var resultText string

	switch params.Name {
	case "seo_inspect_headers":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		hdrAudit := audit.InspectHeaders(res)
		b, _ := json.MarshalIndent(hdrAudit, "", "  ")
		resultText = string(b)

	case "seo_audit_page":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		hdrAudit := audit.InspectHeaders(res)
		htmlAudit, _ := audit.InspectHTML(urlStr, res.Body)
		schemaAudit := audit.InspectSchema(urlStr, res.Body)

		fullReport := map[string]interface{}{
			"headers":   hdrAudit,
			"technical": htmlAudit,
			"schema":    schemaAudit,
		}
		b, _ := json.MarshalIndent(fullReport, "", "  ")
		resultText = string(b)

	case "seo_inspect_sitemap":
		checkLimit := 10
		if cl, ok := params.Arguments["check_limit"].(float64); ok && cl > 0 {
			checkLimit = int(cl)
		}
		if checkLimit > 100 {
			checkLimit = 100
		}
		report, err := client.InspectSitemap(ctx, urlStr, checkLimit)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_inspect_robots":
		report, err := client.InspectRobots(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_inspect_schema":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		report := audit.InspectSchema(urlStr, res.Body)
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_audit_images":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		report, err := audit.InspectImages(urlStr, res.Body)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_audit_content":
		keyword, _ := params.Arguments["keyword"].(string)
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		report, err := audit.InspectContent(urlStr, res.Body, keyword)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_audit_hreflang":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		report, err := audit.InspectHreflang(urlStr, res.Body)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_generate_report":
		data, err := report.BuildAuditData(ctx, client, urlStr)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(data, "", "  ")
		resultText = string(b)

	case "seo_query_backlinks":
		limit := 50
		if lim, ok := params.Arguments["limit"].(float64); ok && lim > 0 {
			limit = int(lim)
		}
		rep, err := api.QueryCommonCrawlBacklinks(ctx, urlStr, limit, "")
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(rep, "", "  ")
		resultText = string(b)

	case "seo_gsc_query":
		token, err := api.ResolveGSCToken(ctx)
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		dims := []string{"query"}
		if dStr, ok := params.Arguments["dimensions"].(string); ok && dStr != "" {
			dims = strings.Split(dStr, ",")
		}
		limit := 25
		if lim, ok := params.Arguments["limit"].(float64); ok && lim > 0 {
			limit = int(lim)
		}
		rep, err := api.QuerySearchAnalytics(ctx, token, api.GSCQueryOptions{
			SiteURL:    urlStr,
			Dimensions: dims,
			RowLimit:   limit,
		})
		if err != nil {
			sendToolResultError(w, id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(rep, "", "  ")
		resultText = string(b)

	default:
		sendError(w, id, -32601, fmt.Sprintf("Unknown tool: %s", params.Name))
		return
	}

	sendResponse(w, id, map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": resultText,
			},
		},
	})
}

func sendToolResultError(w io.Writer, id interface{}, errMsg string) {
	sendResponse(w, id, map[string]interface{}{
		"isError": true,
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("Tool error: %s", errMsg),
			},
		},
	})
}

func sendResponse(w io.Writer, id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	b, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", b)
}

func sendError(w io.Writer, id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	b, _ := json.Marshal(resp)
	fmt.Fprintf(w, "%s\n", b)
}
