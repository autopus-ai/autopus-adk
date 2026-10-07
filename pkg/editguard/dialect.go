package editguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// Platform ids a registered hook command line passes as --platform. The id is
// static generated text and never comes from the payload (REQ-EG-17).
const (
	PlatformClaudeCode = "claude-code"
	PlatformOpenCode   = "opencode"
	PlatformCodex      = "codex"
	PlatformGemini     = "gemini"
)

// DialectFor returns the codec of a platform id. Each codec comes from the
// probe that verified its host (A1 to A3, T11 for Gemini CLI); a platform
// without one has no enforced lane (REQ-EG-14).
func DialectFor(platform string) (Dialect, bool) {
	switch platform {
	case PlatformClaudeCode:
		return claudeDialect{}, true
	case PlatformOpenCode:
		return openCodeDialect{}, true
	case PlatformCodex:
		return codexDialect{}, true
	case PlatformGemini:
		return geminiDialect{}, true
	default:
		return nil, false
	}
}

var errNotADeny = errors.New("editguard: only a deny with a reason is encoded")

// claudeDialect reads tool_input.file_path of Edit, Write, and MultiEdit, the
// subject rulecond derives for those tools (REQ-EG-02). Claude Code 2.1.289
// offers no MultiEdit tool; the name stays accepted for hosts that still do.
// Every other field the host sends, the content body included, is skipped.
type claudeDialect struct{}

const claudeTargetKey = "file_path"

func (claudeDialect) Decode(payload []byte) (Call, error) {
	doc, err := decodeHookPayload(payload, claudeTargetKey)
	if err != nil {
		return Call{}, err
	}
	call := Call{Cwd: doc.cwd}
	// ConditionSubject also answers for Bash, whose subject is a command.
	if !doc.hasInput || !slices.Contains(strings.Split(rulecond.MatcherEdit, "|"), doc.toolName) {
		return call, nil
	}
	if target, ok := rulecond.ConditionSubject(doc.toolName, map[string]any{claudeTargetKey: doc.input}); ok {
		call.Targets = []string{target}
	}
	return call, nil
}

// EncodeDeny writes the PreToolUse deny of the Decision Output Contract. An
// allow is never encoded: `permissionDecision: allow` would skip the user's
// permission flow (REQ-EG-11).
func (claudeDialect) EncodeDeny(decision Decision) ([]byte, error) {
	return encodeHookDeny(decision)
}

// openCodeDialect reads the payload the generated OpenCode plugin synthesizes,
// {"platform","cwd","tool_name","targets"}, with every target in native
// argument order and move destinations included (REQ-EG-13). The payload's
// platform field is never consulted.
type openCodeDialect struct{}

func (openCodeDialect) Decode(payload []byte) (Call, error) {
	return decodeOpenCodePayload(payload)
}

// EncodeDeny writes {"decision":"deny","reason":...} exactly, without a line
// end; the plugin throws Error(reason) only for it.
func (openCodeDialect) EncodeDeny(decision Decision) ([]byte, error) {
	if !decision.Deny || decision.Reason == "" {
		return nil, errNotADeny
	}
	line, err := encodeJSONLine(decisionDeny{Decision: "deny", Reason: decision.Reason})
	return bytes.TrimSuffix(line, []byte("\n")), err
}

// codexDialect reads the apply_patch call of a Codex PreToolUse payload, whose
// tool_input.command is the whole patch text (probe A3). Every other Codex
// tool is a shell path, which the guard does not cover. The deny is the
// PreToolUse document Claude Code uses, which A3 saw block on exit 0.
type codexDialect struct{}

const codexPatchTool = "apply_patch"

func (codexDialect) Decode(payload []byte) (Call, error) {
	doc, err := decodeHookPayload(payload, "command")
	if err != nil {
		return Call{}, err
	}
	call := Call{Cwd: doc.cwd}
	if doc.toolName != codexPatchTool || !doc.hasInput {
		return call, nil
	}
	targets, displaced := patchTargets(doc.input)
	for _, target := range targets {
		call.add(target)
	}
	call.Displaced = displaced
	return call, nil
}

func (codexDialect) EncodeDeny(decision Decision) ([]byte, error) {
	return encodeHookDeny(decision)
}

// Markers of the apply_patch grammar.
const (
	patchAddFile    = "*** Add File: "
	patchUpdateFile = "*** Update File: "
	patchDeleteFile = "*** Delete File: "
	patchMoveTo     = "*** Move to: "
	patchEndOfFile  = "*** End of File"
)

// patchState says how the next patch line is read.
type patchState int

const (
	patchAtHeader    patchState = iota // the line is a hunk header candidate
	patchInAdd                         // "+" lines are added content
	patchAfterUpdate                   // an unpadded Move to line may follow
	patchInUpdate                      // chunk lines until a line starting "***"
)

var patchHeaders = [...]struct {
	prefix string
	next   patchState
}{
	{patchAddFile, patchInAdd},
	{patchUpdateFile, patchAfterUpdate},
	{patchDeleteFile, patchAtHeader},
}

// patchTargets returns every path a patch adds, updates, deletes, or moves
// onto, in patch order. It reads header positions the way Codex 0.160.0 does:
// a header line is trimmed and matched case-sensitively, an Add body runs over
// "+" lines, an Update hunk runs until a line starting "***" other than End of
// File, and a Move to line counts only unpadded right below its Update header,
// with trailing space trimmed. Each path comes in every spelling of
// patchPathSpellings. Where Codex would reject the patch, reading more targets
// is harmless: nothing is written. displaced lists the targets whose directory
// entry the patch removes or replaces: each Delete File, and the source and
// destination of each Move to.
func patchTargets(patch string) (targets, displaced []string) {
	state := patchAtHeader
	var updated []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if state == patchAfterUpdate {
			state = patchInUpdate
			if dest, ok := strings.CutPrefix(line, patchMoveTo); ok {
				dests := patchPathSpellings(strings.TrimRightFunc(dest, unicode.IsSpace))
				targets = append(targets, dests...)
				displaced = append(append(displaced, updated...), dests...)
				continue
			}
		}
		if state == patchInAdd && strings.HasPrefix(line, "+") ||
			state == patchInUpdate && (!strings.HasPrefix(line, "***") || line == patchEndOfFile) {
			continue
		}
		state = patchAtHeader
		header := strings.TrimSpace(line)
		for _, marker := range patchHeaders {
			if target, ok := strings.CutPrefix(header, marker.prefix); ok {
				spellings := patchPathSpellings(target)
				targets = append(targets, spellings...)
				if marker.prefix == patchDeleteFile {
					displaced = append(displaced, spellings...)
				}
				updated, state = spellings, marker.next
				break
			}
		}
	}
	return targets, displaced
}

// patchWritten removes the characters Codex drops from a header path.
var patchWritten = strings.NewReplacer("\t", "", "\r", "")

// patchPathSpellings returns a header path as the patch names it and, when it
// holds a TAB or CR, as Codex 0.160.0 writes it: once a line has matched its
// marker, Codex removes every TAB and CR from the path and keeps every other
// character (a differential run of `codex --codex-run-as-apply-patch`). Both
// are judged, so a host that keeps the characters is covered as well. Codex
// decides relative or absolute on the path as sent, so a relative path whose
// stripped form starts with `/` is also judged below the cwd.
func patchPathSpellings(path string) []string {
	if !strings.ContainsAny(path, "\t\r") {
		return []string{path}
	}
	written := patchWritten.Replace(path)
	spellings := []string{path, written}
	if !strings.HasPrefix(path, "/") && strings.HasPrefix(written, "/") {
		if relative := strings.TrimLeft(written, "/"); relative != "" {
			spellings = append(spellings, relative)
		}
	}
	return spellings
}

// add appends one decoded target; an empty or non-string one is a malformed
// entry the decision drops (REQ-EG-18).
func (c *Call) add(target string) {
	if target == "" {
		c.Dropped++
		return
	}
	c.Targets = append(c.Targets, target)
}

// hookDeny is the PreToolUse deny document, fields in contract order.
type hookDeny struct {
	Output hookDecision `json:"hookSpecificOutput"`
}

type hookDecision struct {
	Event    string `json:"hookEventName"`
	Decision string `json:"permissionDecision"`
	Reason   string `json:"permissionDecisionReason"`
}

func encodeHookDeny(decision Decision) ([]byte, error) {
	if !decision.Deny || decision.Reason == "" {
		return nil, errNotADeny
	}
	return encodeJSONLine(hookDeny{hookDecision{Event: rulecond.EventPreToolUse, Decision: "deny", Reason: decision.Reason}})
}

// encodeJSONLine marshals v as one newline-terminated line. HTML escaping is
// off, so the "&&" of a GS-SRC reason reaches the host byte for byte.
func encodeJSONLine(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
