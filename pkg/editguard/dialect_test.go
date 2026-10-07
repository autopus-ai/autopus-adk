package editguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hostPayload reads a payload a real host sent in the T0 probes
// (evidence/t0-probes.txt), with the probe root placeholder set to root.
func hostPayload(t *testing.T, name, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "hostpayload", name))
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "<R>", string(quoted[1:len(quoted)-1]))
}

func dialectOf(t *testing.T, platform string) Dialect {
	t.Helper()
	dialect, ok := DialectFor(platform)
	if !ok || dialect == nil {
		t.Fatalf("DialectFor(%q) found no dialect", platform)
	}
	return dialect
}

// claudeDeny is DENY of acceptance.md: the exact Claude Code deny JSON plus
// one newline, spelled by hand for reasons that need no JSON escaping.
func claudeDeny(reason string) string {
	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",` +
		`"permissionDecisionReason":"` + reason + `"}}` + "\n"
}

// claudePayload is P(tool, path) of acceptance.md.
func claudePayload(cwd, tool, path string) string {
	data, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": cwd, "hook_event_name": "PreToolUse",
		"tool_name": tool, "tool_input": map[string]any{"file_path": path}})
	return string(data)
}

// S1 row 1 from the payload Claude Code 2.1.289 sent in probe A1: the fields
// the host adds are ignored and the deny is the exact contract bytes.
func TestClaudeDialect_HostEditPayload_DeniesWithTheContractBytes(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	got := runGuard(strings.NewReader(hostPayload(t, "claude-code-2.1.289-edit.json", root)),
		dialectOf(t, PlatformClaudeCode), Options{})
	if got.code != 0 || got.stdout != claudeDeny(gsConReason) || got.stdoutWrites != 1 || got.stderr != "" {
		t.Fatalf("got %+v\nwant stdout %q", got, claudeDeny(gsConReason))
	}
}

// S1: every Claude Code payload exits 0, and only the unoverridden generated
// file is denied, byte for byte.
func TestClaudeDialect_FixtureR_DecisionTableBytes(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformClaudeCode)
	deny := claudeDeny(gsConReason)
	rows := []struct{ tool, path, want string }{
		{"Edit", skillRel, deny},
		{"MultiEdit", filepath.Join(root, skillRel), deny},
		{"Write", skillRel, deny},
		{"Edit", ".claude/settings.json", ""},
		{"Edit", "CLAUDE.md", ""},
		{"Write", ".claude/commands/my-cmd.md", ""},
		{"Write", ".autopus/brainstorms/BS-001.md", ""},
		{"Write", ".autopus/specs/SPEC-X-001/spec.md", ""},
		{"Edit", "pkg/main.go", ""},
		{"Edit", ".agents/skills/x/SKILL.md", ""},
		{"Edit", "config.toml", ""},
		{"Write", ".claude/skills/new/SKILL.md", ""},
	}
	for _, row := range rows {
		got := runGuard(strings.NewReader(claudePayload(root, row.tool, row.path)), dialect, Options{})
		if got.code != 0 || got.stdout != row.want || got.stderr != "" || row.want != "" && got.stdoutWrites != 1 {
			t.Errorf("P(%s, %s): got %+v, want stdout %q", row.tool, row.path, got, row.want)
		}
	}
	bash := runGuard(strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`), dialect, Options{})
	bash.allowed(t, "Bash payload")
}

// S7: call-level faults in a Claude Code payload exit 0 with empty stdout and
// at most one stderr line.
func TestClaudeDialect_CallLevelFaults_Allow(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformClaudeCode)
	for label, stdin := range map[string]string{
		"truncated json":        `{"tool_input":`,
		"zero bytes":            "",
		"1048577 bytes":         strings.Repeat("a", MaxPayloadBytes+1),
		"no target":             `{"tool_name":"Edit","tool_input":{}}`,
		"non-string target":     `{"tool_name":"Edit","tool_input":{"file_path":42}}`,
		"no tool input":         `{"tool_name":"Write"}`,
		"tool input not object": `{"tool_name":"Edit","tool_input":"x"}`,
		"cwd not a string":      `{"cwd":7,"tool_name":"Edit","tool_input":{"file_path":"x"}}`,
		"json array":            `[]`,
		"json null":             `null`,
	} {
		runGuard(strings.NewReader(stdin), dialect, Options{}).allowed(t, label)
	}
	panicked := runGuard(strings.NewReader(claudePayload(root, "Edit", skillRel)), dialect,
		Options{panicSeam: func() { panic("seam") }})
	panicked.allowed(t, "recovered panic")
}

// S3: the GS-SRC reason reaches Claude Code with its "&&" unescaped.
func TestClaudeDialect_SourceRepoReason_KeepsAmpersandsLiteral(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	for _, marker := range []string{"content/x.md", "templates/x.tmpl", "cmd/generate-templates/main.go"} {
		writeFile(t, root, marker, "x")
	}
	got := runGuard(strings.NewReader(claudePayload(root, "Edit", skillRel)), dialectOf(t, PlatformClaudeCode), Options{})
	if got.stdout != claudeDeny(gsSrcReason) {
		t.Fatalf("stdout = %q\nwant     %q", got.stdout, claudeDeny(gsSrcReason))
	}
}

// S4 and S8: an FL reason with quotes survives the JSON encoding exactly, on
// one line.
func TestClaudeDialect_LockReasonWithQuotes_RoundTrips(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip(`a file name with " is invalid on windows`)
	}
	root := lockProject(t)
	rel := `internal/foo/it's "q"_test.go`
	writeFile(t, root, rel, tContent)
	mustLock(t, openTestStore(t, root, newClock(t0)), rel)
	got := runGuard(strings.NewReader(claudePayload(root, "Edit", rel)), dialectOf(t, PlatformClaudeCode),
		Options{Now: newClock(t0).Now})
	var doc struct {
		Output struct {
			Event    string `json:"hookEventName"`
			Decision string `json:"permissionDecision"`
			Reason   string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil || strings.Count(got.stdout, "\n") != 1 {
		t.Fatalf("stdout %q is not one JSON line: %v", got.stdout, err)
	}
	want := flHead(rel) + flTail + `'internal/foo/it'\''s "q"_test.go'`
	if doc.Output.Event != "PreToolUse" || doc.Output.Decision != "deny" || doc.Output.Reason != want {
		t.Fatalf("decoded %+v\nwant reason %q", doc.Output, want)
	}
}

func TestDialectFor_OnlyVerifiedPlatformsHaveADialect(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{"", "gemini-cli", "antigravity-cli", "omp", "Claude-Code", "claude"} {
		if dialect, ok := DialectFor(platform); ok || dialect != nil {
			t.Errorf("DialectFor(%q) = %v, %v; want no dialect", platform, dialect, ok)
		}
	}
}

// A dialect encodes only a deny with a reason; anything else is a fault the
// guard turns into an allow.
func TestDialects_EncodeOnlyADenyWithAReason(t *testing.T) {
	t.Parallel()
	for _, platform := range []string{PlatformClaudeCode, PlatformOpenCode, PlatformCodex, PlatformGemini} {
		dialect := dialectOf(t, platform)
		for _, decision := range []Decision{{}, {Deny: true, Class: ClassFixLock}} {
			if out, err := dialect.EncodeDeny(decision); err == nil || out != nil {
				t.Errorf("%s EncodeDeny(%+v) = %q, %v; want an error", platform, decision, out, err)
			}
		}
	}
}
