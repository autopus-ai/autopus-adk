package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixture names of SPEC-EDITGUARD-001 acceptance.md: the generated skill of
// fixture R and the reproduction test T with its SHA-256.
const (
	egSkill   = ".claude/skills/auto-fix/SKILL.md"
	egT       = "internal/foo/foo_repro_test.go"
	egTBody   = "package foo\n"
	egTHash   = "1b63a92736f126a00f521c0ef804e67d0cf949b5ff790d6d4c3a4b7681da8d21"
	egWeak    = "package foo // weakened\n"
	egWeakSum = "e4303e1b170ed51f64829282b577efdb87a5334d86f2b07f9185084fea2f6b96"
	egGSCon   = "autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated " +
		"(manifest .autopus/claude-code-manifest.json, policy always). " +
		"Change autopus.yaml or the upstream Autopus source, then run: auto update"
	egTFL = "autopus edit-guard [fix_lock]: internal/foo/foo_repro_test.go is the locked reproduction test " +
		"of an in-progress /auto fix. Fix the code under test instead. If the test itself is wrong, stop and " +
		"ask the user to run: auto fix unlock -- 'internal/foo/foo_repro_test.go'"
)

var egT0 = time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

// egProject builds a consumer project: autopus.yaml, a claude-code manifest
// that lists the auto-fix skill as always, the skill, and T.
func egProject(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	egWrite(t, root, "autopus.yaml", "project:\n  name: guard-cli\n")
	egWrite(t, root, ".autopus/claude-code-manifest.json",
		`{"files":{".claude/skills/auto-fix/SKILL.md":{"checksum":"x","policy":"always"}}}`)
	egWrite(t, root, egSkill, "x")
	egWrite(t, root, egT, egTBody)
	return root
}

func egWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type egResult struct {
	code           int
	stdout, stderr string
	err            error
}

// egRun executes the real root command with stdin and maps its error to the
// process exit status the way Execute does.
func egRun(t *testing.T, stdin string, args ...string) egResult {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	code := 0
	if err != nil {
		code = exitCodeForError(err)
	}
	return egResult{code: code, stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// egHermetic clears the bypass variable a developer shell may export and
// pins the clock of the guard and of auto fix to *now.
func egHermetic(t *testing.T, now *time.Time) {
	t.Helper()
	t.Setenv(editGuardEnv, "")
	previous := editGuardClock
	editGuardClock = func() time.Time { return *now }
	t.Cleanup(func() { editGuardClock = previous })
}

// egEdit is P(tool, path) of acceptance.md.
func egEdit(cwd, tool, path string) string {
	data, _ := json.Marshal(map[string]any{"session_id": "s1", "cwd": cwd, "hook_event_name": "PreToolUse",
		"tool_name": tool, "tool_input": map[string]any{"file_path": path}})
	return string(data)
}

// egClaudeDeny is the exact Claude Code deny of the Decision Output Contract.
func egClaudeDeny(reason string) string {
	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",` +
		`"permissionDecisionReason":"` + reason + `"}}` + "\n"
}
