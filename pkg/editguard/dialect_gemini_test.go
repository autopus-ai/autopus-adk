package editguard

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// geminiDeny is the BeforeTool deny Gemini CLI 0.52.0 blocked on in the T11
// probe, plus the line end every dialect writes.
func geminiDeny(reason string) string {
	return `{"decision":"deny","reason":"` + reason + `"}` + "\n"
}

func geminiPayload(cwd, tool string, input any) string {
	data, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": cwd, "hook_event_name": "BeforeTool",
		"tool_name": tool, "tool_input": input})
	return string(data)
}

// T11: the BeforeTool payloads Gemini CLI 0.52.0 sent for write_file and
// replace name tool_input.file_path, relative as the model sent it or
// absolute; a protected one gets the deny encoding the probe saw block.
func TestGeminiDialect_HostBeforeToolPayloads(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformGemini)
	lines := strings.Split(strings.TrimSpace(hostPayload(t, "gemini-cli-0.52.0-before-tool.jsonl", root)), "\n")
	// write_file pkg/foo.go, replace notes.txt, write_file absolute protected,
	// replace relative protected.
	want := []string{"", "", geminiDeny(gsConReason), geminiDeny(gsConReason)}
	if len(lines) != len(want) {
		t.Fatalf("fixture has %d payloads, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		got := runGuard(strings.NewReader(line), dialect, Options{})
		if got.code != 0 || got.stdout != want[i] || got.stderr != "" || want[i] != "" && got.stdoutWrites != 1 {
			t.Errorf("payload %d: got %+v, want stdout %q", i+1, got, want[i])
		}
	}
	call, err := dialect.Decode([]byte(lines[1]))
	if err != nil || call.Cwd != root || !slices.Equal(call.Targets, []string{"notes.txt"}) {
		t.Fatalf("Decode(replace) = %+v, %v", call, err)
	}
}

// Only write_file and replace edit files; the shell tool and every other tool
// have no target, and a file_path that is not a string is no target either.
func TestGeminiDialect_OtherToolsHaveNoTarget(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformGemini)
	for _, payload := range []string{
		geminiPayload(root, "run_shell_command", map[string]any{"command": "echo x > " + skillRel}),
		geminiPayload(root, "read_file", map[string]any{"file_path": skillRel}),
		geminiPayload(root, "Edit", map[string]any{"file_path": skillRel}),
		geminiPayload(root, "write_file", map[string]any{"file_path": 42}),
		geminiPayload(root, "replace", nil),
	} {
		call, err := dialect.Decode([]byte(payload))
		if err != nil || len(call.Targets) != 0 {
			t.Errorf("Decode(%s) = %+v, %v", payload, call, err)
		}
		runGuard(strings.NewReader(payload), dialect, Options{}).allowed(t, payload)
	}
	if _, err := dialect.Decode([]byte(`{"tool_name":"write_file","tool_input":"x"}`)); err == nil {
		t.Error("a tool_input that is not an object must be a decode error")
	}
}
