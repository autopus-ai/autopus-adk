package editguard

import (
	"encoding/json"
	"strings"
	"testing"
)

// writePayload is a Claude Code Write of body to path.
func writePayload(t *testing.T, cwd, path, body string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"session_id": "s1", "cwd": cwd, "hook_event_name": "PreToolUse",
		"tool_name": "Write", "tool_input": map[string]any{"file_path": path, "content": body}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// M2: a content body past the old 1 MiB bound no longer allows the call; only
// the target fields are decoded, and the body is skipped.
func TestRun_LargeContentBody_IsStillDecided(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	body := strings.Repeat("\x01 weakened line\n", (2<<20)/16)
	for _, platform := range []string{PlatformClaudeCode, PlatformGemini} {
		payload := writePayload(t, root, skillRel, body)
		if platform == PlatformGemini {
			payload = strings.Replace(payload, `"tool_name":"Write"`, `"tool_name":"write_file"`, 1)
		}
		if len(payload) <= 2<<20 {
			t.Fatalf("payload is only %d bytes", len(payload))
		}
		got := runGuard(strings.NewReader(payload), dialectOf(t, platform), Options{})
		if got.code != 0 || got.stdoutWrites != 1 || !strings.Contains(got.stdout, `[generated_surface]`) || got.stderr != "" {
			t.Errorf("%s: a %d-byte Write of the generated skill = %+v, want the GS-CON deny", platform, len(payload), got)
		}
	}
	patch := "*** Begin Patch\n*** Add File: pkg/new.go\n+" + strings.Repeat("x", 2<<20) +
		"\n*** Update File: " + skillRel + "\n@@\n-x\n+y\n*** End Patch\n"
	codex, _ := json.Marshal(map[string]any{"cwd": root, "tool_name": "apply_patch", "tool_input": map[string]any{"command": patch}})
	if got := runGuard(strings.NewReader(string(codex)), dialectOf(t, PlatformCodex), Options{}); got.stdoutWrites != 1 {
		t.Errorf("codex: a %d-byte patch whose second target is generated = %+v, want the deny", len(codex), got)
	}
}

// M2: the hard cap still allows a payload past it, and the bound is not off
// by one. A small test cap stands in for the 64 MiB production cap.
func TestRun_PayloadPastTheHardCap_Allows(t *testing.T) {
	t.Parallel()
	if MaxPayloadBytes != 64<<20 {
		t.Fatalf("MaxPayloadBytes = %d, want 64 MiB", MaxPayloadBytes)
	}
	root := fixtureR(t)
	payload := payloadOf(root, skillRel)
	payload += strings.Repeat(" ", 4096-len(payload))
	opts := Options{payloadCap: 4096}
	if class, _ := runGuard(strings.NewReader(payload), testDialect{}, opts).denied(t); class != string(ClassGeneratedSurface) {
		t.Fatalf("a payload of exactly the cap: class %s", class)
	}
	over := runGuard(strings.NewReader(payload+" "), testDialect{}, opts)
	over.allowed(t, "one byte past the cap")
	if over.stderr != "autopus edit-guard: allow (payload over 64 MiB)\n" {
		t.Errorf("over-cap diagnostic = %q", over.stderr)
	}
}

// M2: the decoders read keys exactly and the last occurrence wins, as the
// hosts' JSON parsers do, so a second spelling of file_path cannot redirect the
// guard to another file than the host writes.
func TestDecode_KeysMatchExactlyAndTheLastOccurrenceWins(t *testing.T) {
	t.Parallel()
	claude := dialectOf(t, PlatformClaudeCode)
	for payload, want := range map[string]string{
		`{"tool_name":"Edit","tool_input":{"file_path":"a.go","FILE_PATH":"b.go"}}`:                "a.go",
		`{"tool_name":"Edit","tool_input":{"File_Path":"b.go","file_path":"a.go"}}`:                "a.go",
		`{"tool_name":"Edit","tool_input":{"file_path":"b.go","file_path":"a.go"}}`:                "a.go",
		`{"tool_name":"Edit","tool_input":{"file_path":"a.go","file_path":42}}`:                    "",
		`{"tool_name":"Edit","tool_input":{"content":{"file_path":"b.go"},"file_path":"a.go"}}`:    "a.go",
		`{"tool_name":"Edit","tool_input":{"file_path":"a.go"},"tool_input":{"file_path":"c.go"}}`: "c.go",
	} {
		call, err := claude.Decode([]byte(payload))
		got := strings.Join(call.Targets, ",")
		if err != nil || got != want {
			t.Errorf("Decode(%s) = %q, %v; want %q", payload, got, err, want)
		}
	}
	for _, payload := range []string{`{"tool_name":"Edit"} {}`, `{"tool_name":"Edit","tool_input":{"file_path":"a.go"}`,
		`{"tool_name":["Edit"],"tool_input":{"file_path":"a.go"}}`} {
		if _, err := claude.Decode([]byte(payload)); err == nil {
			t.Errorf("Decode(%s) accepted a malformed payload", payload)
		}
	}
}
