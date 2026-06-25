package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const ProtocolVersion = "2025-06-18"

const (
	initializeRequestID = 1
	toolsListRequestID  = 2
	toolsCallRequestID  = 3
)

type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type ToolsListResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type ToolCallResult struct {
	Content           []map[string]any `json:"content,omitempty"`
	StructuredContent any              `json:"structuredContent,omitempty"`
	IsError           bool             `json:"isError,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e RPCError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("mcp rpc error %d", e.Code)
	}
	return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message)
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

func BuildToolsListInput() ([]byte, error) {
	return encodeJSONLines([]rpcMessage{
		initializeRequest(),
		initializedNotification(),
		{
			JSONRPC: "2.0",
			ID:      toolsListRequestID,
			Method:  "tools/list",
			Params:  map[string]any{},
		},
	})
}

func BuildToolCallInput(toolName string, arguments json.RawMessage) ([]byte, error) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil, errors.New("tool name is required")
	}
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return nil, errors.New("tool arguments must be valid JSON")
	}
	return encodeJSONLines([]rpcMessage{
		initializeRequest(),
		initializedNotification(),
		{
			JSONRPC: "2.0",
			ID:      toolsCallRequestID,
			Method:  "tools/call",
			Params: map[string]any{
				"name":      toolName,
				"arguments": json.RawMessage(arguments),
			},
		},
	})
}

func ParseToolsListOutput(output []byte) (ToolsListResult, error) {
	response, err := findResponse(output, toolsListRequestID)
	if err != nil {
		return ToolsListResult{}, err
	}
	if response.Error != nil {
		return ToolsListResult{}, *response.Error
	}
	var result ToolsListResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return ToolsListResult{}, err
	}
	sort.SliceStable(result.Tools, func(i, j int) bool {
		return result.Tools[i].Name < result.Tools[j].Name
	})
	return result, nil
}

func ParseToolCallOutput(output []byte) (ToolCallResult, error) {
	response, err := findResponse(output, toolsCallRequestID)
	if err != nil {
		return ToolCallResult{}, err
	}
	if response.Error != nil {
		return ToolCallResult{}, *response.Error
	}
	var result ToolCallResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return ToolCallResult{}, err
	}
	return result, nil
}

func ToolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != "" {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)
	return names
}

func HasTool(tools []Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func ArgumentSummary(raw json.RawMessage) map[string]any {
	compact := compactJSON(raw)
	return map[string]any{
		"argument_keys":   ArgumentKeys(raw),
		"argument_bytes":  len(compact),
		"argument_digest": ArgumentDigest(raw),
	}
}

func ArgumentKeys(raw json.RawMessage) []string {
	var object map[string]any
	keys := []string{}
	if err := json.Unmarshal(compactJSON(raw), &object); err == nil {
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
	}
	return keys
}

func ArgumentDigest(raw json.RawMessage) string {
	sum := sha256.Sum256(compactJSON(raw))
	return hex.EncodeToString(sum[:])
}

func ResultSummary(result ToolCallResult) map[string]any {
	types := make([]string, 0, len(result.Content))
	for _, item := range result.Content {
		if value, ok := item["type"].(string); ok && value != "" {
			types = append(types, value)
		}
	}
	sort.Strings(types)
	raw, _ := json.Marshal(result)
	return map[string]any{
		"is_error":      result.IsError,
		"content_count": len(result.Content),
		"content_types": types,
		"result_bytes":  len(raw),
	}
}

func compactJSON(raw json.RawMessage) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte(`{}`)
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err == nil {
		if encoded, err := json.Marshal(value); err == nil {
			return encoded
		}
	}
	var out bytes.Buffer
	if err := json.Compact(&out, trimmed); err == nil {
		return out.Bytes()
	}
	return trimmed
}

func initializeRequest() rpcMessage {
	return rpcMessage{
		JSONRPC: "2.0",
		ID:      initializeRequestID,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{},
			"clientInfo": map[string]string{
				"name":    "stola",
				"title":   "Stola Host OS",
				"version": "0.0.0-dev",
			},
		},
	}
}

func initializedNotification() rpcMessage {
	return rpcMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
}

func encodeJSONLines(messages []rpcMessage) ([]byte, error) {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	for _, message := range messages {
		if err := enc.Encode(message); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func findResponse(output []byte, id int) (rpcMessage, error) {
	for _, rawLine := range bytes.Split(output, []byte{'\n'}) {
		line := bytes.TrimSpace(rawLine)
		if len(line) == 0 {
			continue
		}
		var message rpcMessage
		if err := json.Unmarshal(line, &message); err != nil {
			continue
		}
		if idMatches(message.ID, id) {
			return message, nil
		}
	}
	return rpcMessage{}, fmt.Errorf("mcp response id %d not found", id)
}

func idMatches(value any, want int) bool {
	switch typed := value.(type) {
	case float64:
		return int(typed) == want
	case int:
		return typed == want
	case string:
		return typed == fmt.Sprintf("%d", want)
	default:
		return false
	}
}
