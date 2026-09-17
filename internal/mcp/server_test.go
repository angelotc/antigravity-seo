package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runProtocol(t *testing.T, input string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(strings.NewReader(input), &out, "test"); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	return out.String()
}

func TestMCPProtocolHandshake(t *testing.T) {
	out := runProtocol(t, strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"bogus/method"}`,
		`not json at all`,
	}, "\n"))

	if !strings.Contains(out, `"protocolVersion":"2024-11-05"`) {
		t.Error("initialize response missing protocolVersion")
	}
	if !strings.Contains(out, `"serverInfo"`) || !strings.Contains(out, `"antigravity-seo-engine"`) {
		t.Error("serverInfo missing")
	}
	if !strings.Contains(out, `-32601`) {
		t.Error("unknown method should return -32601")
	}
	if !strings.Contains(out, `-32700`) {
		t.Error("malformed line should return -32700 parse error")
	}

	// tools/list must advertise the full toolset including the v2 additions
	for _, name := range []string{
		"seo_inspect_headers", "seo_audit_page", "seo_inspect_sitemap", "seo_inspect_robots",
		"seo_inspect_schema", "seo_audit_images", "seo_audit_content", "seo_audit_hreflang",
	} {
		if !strings.Contains(out, `"name":"`+name+`"`) {
			t.Errorf("tools/list missing %s", name)
		}
	}
}

func TestMCPMissingURL(t *testing.T) {
	out := runProtocol(t, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"seo_audit_page","arguments":{}}}`)
	if !strings.Contains(out, "-32602") {
		t.Errorf("missing url should return -32602, got: %s", out)
	}
}

func TestMCPUnknownTool(t *testing.T) {
	out := runProtocol(t, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"nope","arguments":{"url":"https://example.com"}}}`)
	if !strings.Contains(out, "Unknown tool") {
		t.Errorf("unknown tool should be rejected, got: %s", out)
	}
}

func TestResponseJSONShape(t *testing.T) {
	out := runProtocol(t, `{"jsonrpc":"2.0","id":77,"method":"ping"}`)
	var resp struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      json.Number `json:"id"`
		Result  struct{}    `json:"result"`
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		t.Fatal("no response")
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &resp); err != nil {
		t.Fatalf("response is not valid JSON-RPC: %v (%s)", err, out)
	}
	if resp.JSONRPC != "2.0" || resp.ID.String() != "77" {
		t.Errorf("wrong envelope: %+v", resp)
	}
}
