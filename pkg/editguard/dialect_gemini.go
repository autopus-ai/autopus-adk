package editguard

import (
	"path/filepath"
	"slices"
)

// geminiEditTools are the file-editing tools of Gemini CLI 0.52.0. Both name
// their target in tool_input.file_path; every other built-in tool reads, or
// is the shell tool, which the guard does not cover (T11 probe).
var geminiEditTools = []string{"write_file", "replace"}

// geminiSearchTool is the one edit tool that hands a relative file_path to
// correctPath; write_file resolves its path literally.
const geminiSearchTool = "replace"

// geminiDialect reads the BeforeTool payload Gemini CLI sends a command hook,
// whose tool_input.file_path is the target as the model sent it, relative to
// the payload cwd or absolute; its targets are every spelling the host may
// write it as (geminiSpellings). The hook sees the path before the host
// resolves it, so a relative replace path is also searched as correctPath
// searches it (searchTargets). The deny is the top-level decision document
// the T11 probe saw block on exit 0.
type geminiDialect struct{}

func (geminiDialect) Decode(payload []byte) (Call, error) {
	doc, err := decodeHookPayload(payload, "file_path")
	if err != nil {
		return Call{}, err
	}
	call := Call{Cwd: doc.cwd}
	if slices.Contains(geminiEditTools, doc.toolName) && doc.hasInput {
		// replace decodes an absolute path as sent and searches a relative one.
		replace, absolute := doc.toolName == geminiSearchTool, filepath.IsAbs(doc.input)
		for _, spelling := range geminiSpellings(doc.cwd, doc.input, replace && absolute) {
			call.add(spelling)
		}
		if replace && !absolute {
			call.Searched = geminiBases(doc.input)
		}
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
