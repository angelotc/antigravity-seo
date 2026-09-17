package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
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
	client := crawler.NewSafeClient(crawler.ClientOptions{
		Timeout: 20 * time.Second,
	})

	scanner := bufio.NewScanner(os.Stdin)
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
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(os.Stdout, nil, -32700, "Parse error")
			continue
		}

		switch req.Method {
		case "initialize":
			sendResponse(os.Stdout, req.ID, map[string]interface{}{
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
			sendResponse(os.Stdout, req.ID, map[string]interface{}{})

		case "tools/list":
			sendResponse(os.Stdout, req.ID, map[string]interface{}{
				"tools": tools,
			})

		case "tools/call":
			handleToolCall(context.Background(), client, req.ID, req.Params)

		default:
			sendError(os.Stdout, req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
		}
	}

	return scanner.Err()
}

func handleToolCall(ctx context.Context, client *crawler.SafeClient, id interface{}, paramsRaw json.RawMessage) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		sendError(os.Stdout, id, -32602, "Invalid params")
		return
	}

	urlStr, _ := params.Arguments["url"].(string)
	if urlStr == "" {
		sendError(os.Stdout, id, -32602, "Missing required argument 'url'")
		return
	}

	var resultText string

	switch params.Name {
	case "seo_inspect_headers":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(id, err.Error())
			return
		}
		hdrAudit := audit.InspectHeaders(res)
		b, _ := json.MarshalIndent(hdrAudit, "", "  ")
		resultText = string(b)

	case "seo_audit_page":
		res, err := client.Fetch(ctx, urlStr)
		if err != nil {
			sendToolResultError(id, err.Error())
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
		report, err := client.InspectSitemap(ctx, urlStr, checkLimit)
		if err != nil {
			sendToolResultError(id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	case "seo_inspect_robots":
		report, err := client.InspectRobots(ctx, urlStr)
		if err != nil {
			sendToolResultError(id, err.Error())
			return
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		resultText = string(b)

	default:
		sendError(os.Stdout, id, -32601, fmt.Sprintf("Unknown tool: %s", params.Name))
		return
	}

	sendResponse(os.Stdout, id, map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": resultText,
			},
		},
	})
}

func sendToolResultError(id interface{}, errMsg string) {
	sendResponse(os.Stdout, id, map[string]interface{}{
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
