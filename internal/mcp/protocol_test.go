package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildAndParseToolsList(t *testing.T) {
	input, err := BuildToolsListInput()
	if err != nil {
		t.Fatalf("BuildToolsListInput returned error: %v", err)
	}
	text := string(input)
	if !strings.Contains(text, `"method":"initialize"`) || !strings.Contains(text, `"method":"tools/list"`) {
		t.Fatalf("unexpected tools/list input: %s", text)
	}

	output := []byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}}}}
{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"write_file","description":"Write"},{"name":"read_file","description":"Read"}]}}
`)
	result, err := ParseToolsListOutput(output)
	if err != nil {
		t.Fatalf("ParseToolsListOutput returned error: %v", err)
	}
	if got := ToolNames(result.Tools); strings.Join(got, ",") != "read_file,write_file" {
		t.Fatalf("unexpected tools: %#v", got)
	}
	if !HasTool(result.Tools, "read_file") || HasTool(result.Tools, "missing") {
		t.Fatalf("unexpected HasTool result: %#v", result.Tools)
	}
}

func TestBuildAndParseToolCall(t *testing.T) {
	input, err := BuildToolCallInput("read_file", json.RawMessage(`{"path":"/workspace/README.md"}`))
	if err != nil {
		t.Fatalf("BuildToolCallInput returned error: %v", err)
	}
	text := string(input)
	if !strings.Contains(text, `"method":"tools/call"`) || !strings.Contains(text, `"name":"read_file"`) {
		t.Fatalf("unexpected tools/call input: %s", text)
	}

	output := []byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}}}}
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"hello"}],"isError":false}}
`)
	result, err := ParseToolCallOutput(output)
	if err != nil {
		t.Fatalf("ParseToolCallOutput returned error: %v", err)
	}
	if result.IsError || len(result.Content) != 1 || result.Content[0]["text"] != "hello" {
		t.Fatalf("unexpected result: %#v", result)
	}
	summary := ResultSummary(result)
	if summary["content_count"] != 1 || summary["result_bytes"] == 0 {
		t.Fatalf("unexpected result summary: %#v", summary)
	}
}

func TestParseToolCallReturnsRPCError(t *testing.T) {
	output := []byte(`{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"bad args"}}` + "\n")
	_, err := ParseToolCallOutput(output)
	if err == nil || !strings.Contains(err.Error(), "bad args") {
		t.Fatalf("expected RPC error, got %v", err)
	}
}

func TestArgumentSummaryDoesNotExposeValues(t *testing.T) {
	summary := ArgumentSummary(json.RawMessage(`{"path":"/workspace/a","secret":"value"}`))
	keys := strings.Join(summary["argument_keys"].([]string), ",")
	if keys != "path,secret" {
		t.Fatalf("unexpected argument keys: %s", keys)
	}
	if _, ok := summary["secret"]; ok {
		t.Fatalf("summary exposed secret value: %#v", summary)
	}
	if summary["argument_digest"] == "" {
		t.Fatalf("summary did not include argument digest: %#v", summary)
	}
}

func TestArgumentDigestIsCanonicalAndValueSensitive(t *testing.T) {
	first := ArgumentDigest(json.RawMessage(`{"b":2,"a":1}`))
	second := ArgumentDigest(json.RawMessage(`{ "a" : 1, "b" : 2 }`))
	if first != second {
		t.Fatalf("expected equivalent JSON to have same digest: %s != %s", first, second)
	}
	changed := ArgumentDigest(json.RawMessage(`{"a":1,"b":3}`))
	if changed == first {
		t.Fatalf("expected changed JSON to have a different digest")
	}
}
