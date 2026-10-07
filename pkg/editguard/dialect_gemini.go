package editguard

import (
	"encoding/json"
	"slices"
)

// geminiEditTools are the file-editing tools of Gemini CLI 0.52.0. Both name
// their target in tool_input.file_path; every other built-in tool reads, or
// is the shell tool, which the guard does not cover (T11 probe).
var geminiEditTools = []string{"write_file", "replace"}

// geminiDialect reads the BeforeTool payload Gemini CLI sends a command hook,
// whose tool_input.file_path is the target as the model sent it, relative to
// the payload cwd or absolute. The deny is the top-level decision document the
// T11 probe saw block on exit 0.
type geminiDialect struct{}

func (geminiDialect) Decode(payload []byte) (Call, error) {
	var doc toolPayload
	if err := json.Unmarshal(payload, &doc); err != nil {
		return Call{}, err
	}
	call := Call{Cwd: doc.Cwd}
	if !slices.Contains(geminiEditTools, doc.ToolName) {
		return call, nil
	}
	if target, ok := doc.ToolInput["file_path"].(string); ok {
		call.add(target)
	}
	return call, nil
}

func (geminiDialect) EncodeDeny(decision Decision) ([]byte, error) {
	if !decision.Deny || decision.Reason == "" {
		return nil, errNotADeny
	}
	return encodeJSONLine(decisionDeny{Decision: "deny", Reason: decision.Reason})
}

// decisionDeny is the top-level {"decision","reason"} deny that the OpenCode
// plugin and Gemini CLI read, fields in contract order.
type decisionDeny struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
