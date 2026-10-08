//go:build unix

package cli

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The stream-json reading of the T10 live check: the system init event, the
// last result event, and every tool call with its result, so the evidence
// can show what a Grep or Glob outside the band worktree got back.

// lpitLiveInit and lpitLiveResult are the stream-json fields of the check.
type lpitLiveInit struct {
	Model          string            `json:"model"`
	PermissionMode string            `json:"permissionMode"`
	APIKeySource   string            `json:"apiKeySource"`
	Tools          []string          `json:"tools"`
	MCPServers     []json.RawMessage `json:"mcp_servers"`
}

type lpitLiveResult struct {
	Subtype           string `json:"subtype"`
	IsError           bool   `json:"is_error"`
	NumTurns          int    `json:"num_turns"`
	PermissionDenials []struct {
		Tool  string         `json:"tool_name"`
		ID    string         `json:"tool_use_id"`
		Input map[string]any `json:"tool_input"`
	} `json:"permission_denials"`
}

// lpitLiveTool is one tool call of a stream and what came back for it.
type lpitLiveTool struct {
	ID, Name    string
	Input       map[string]any
	ResultError bool
	Result      string // the tool_result text; "" when none came back
	Answered    bool
}

// target is the path-like input of a tool call: file_path, path, or, for
// a Glob without a path, its pattern.
func (tool lpitLiveTool) target() string {
	for _, key := range []string{"file_path", "path", "pattern"} {
		if value, ok := tool.Input[key].(string); ok && (key != "pattern" || tool.Name == "Glob") {
			return value
		}
	}
	return ""
}

// outside reports a call whose target is an absolute path outside worktree.
func (tool lpitLiveTool) outside(worktree string) bool {
	target := tool.target()
	if !filepath.IsAbs(target) {
		return false
	}
	rel, err := filepath.Rel(worktree, target)
	return err != nil || rel == ".." || strings.HasPrefix(rel, "../")
}

type lpitLiveStreamEvents struct {
	init   lpitLiveInit
	result lpitLiveResult
	tools  []*lpitLiveTool
}

// lpitLiveStream reads the system init event, the last result event, and
// the tool calls in stream order.
func lpitLiveStream(t *testing.T, stream string) lpitLiveStreamEvents {
	t.Helper()
	var events lpitLiveStreamEvents
	byID := map[string]*lpitLiveTool{}
	found := false
	scanner := bufio.NewScanner(strings.NewReader(stream))
	scanner.Buffer(nil, 16<<20)
	for scanner.Scan() {
		var line struct {
			Type, Subtype string
			Message       struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			continue
		}
		switch {
		case line.Type == "system" && line.Subtype == "init" && !found:
			require.NoError(t, json.Unmarshal(scanner.Bytes(), &events.init))
			found = true
		case line.Type == "result":
			require.NoError(t, json.Unmarshal(scanner.Bytes(), &events.result))
		case line.Type == "assistant" || line.Type == "user":
			events.tools = lpitLiveToolBlocks(line.Message.Content, byID, events.tools)
		}
	}
	require.NoError(t, scanner.Err())
	require.True(t, found, "a system init event")
	return events
}

// lpitLiveToolBlocks adds the tool_use blocks of one message and attaches
// its tool_result blocks to their calls.
func lpitLiveToolBlocks(raw json.RawMessage, byID map[string]*lpitLiveTool, tools []*lpitLiveTool) []*lpitLiveTool {
	var blocks []struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     map[string]any  `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
		IsError   bool            `json:"is_error"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return tools
	}
	for _, block := range blocks {
		switch block.Type {
		case "tool_use":
			tool := &lpitLiveTool{ID: block.ID, Name: block.Name, Input: block.Input}
			byID[block.ID] = tool
			tools = append(tools, tool)
		case "tool_result":
			if tool := byID[block.ToolUseID]; tool != nil {
				tool.Answered, tool.ResultError, tool.Result = true, block.IsError, lpitLiveText(block.Content)
			}
		}
	}
	return tools
}

// lpitLiveText is the text of a tool_result content: a string, or the text
// of its text blocks.
func lpitLiveText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(raw, &blocks)
	var out strings.Builder
	for _, block := range blocks {
		out.WriteString(block.Text)
	}
	return out.String()
}
