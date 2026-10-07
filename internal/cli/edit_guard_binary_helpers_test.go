package cli_test

// Process-level fixtures of the SPEC-EDITGUARD-001 T15 verification: the
// real `auto` binary built from this tree, acceptance fixture R, and the
// P(tool, path) payload of acceptance.md.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Fixture names and contract texts of acceptance.md.
const (
	binSkill  = ".claude/skills/auto-fix/SKILL.md"
	binT      = "internal/foo/foo_repro_test.go"
	binTBody  = "package foo\n"
	binTHash  = "1b63a92736f126a00f521c0ef804e67d0cf949b5ff790d6d4c3a4b7681da8d21"
	binWeak   = "package foo // weakened\n"
	binWeakSH = "e4303e1b170ed51f64829282b577efdb87a5334d86f2b07f9185084fea2f6b96"
	binUBody  = "package foo\n\n// u\n"
	binUHash  = "e30dfc875b099c64e11f78d7d101ec198fa8eca49f58da40d9c19500980f58b8"
	binGSCon  = "autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated " +
		"(manifest .autopus/claude-code-manifest.json, policy always). " +
		"Change autopus.yaml or the upstream Autopus source, then run: auto update"
	binTFL = "autopus edit-guard [fix_lock]: internal/foo/foo_repro_test.go is the locked reproduction test " +
		"of an in-progress /auto fix. Fix the code under test instead. If the test itself is wrong, stop and " +
		"ask the user to run: auto fix unlock -- 'internal/foo/foo_repro_test.go'"
)

// buildGuardBinary builds ./cmd/auto without any test seam into a temp dir.
func buildGuardBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the process-level corpus runs an extensionless auto executable")
	}
	bin := filepath.Join(t.TempDir(), "auto")
	build := exec.Command("go", "build", "-o", bin, "./cmd/auto")
	build.Dir = editGuardModuleRoot(t)
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	return bin
}

type procResult struct {
	code           int
	stdout, stderr string
}

// runGuardProc runs the binary in dir with stdin and a minimal environment
// that cannot turn the guard off. It is safe on worker goroutines: a process
// that cannot start fails the test without stopping it and reads exit -1.
func runGuardProc(t *testing.T, bin, dir, stdin string, args ...string) procResult {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Errorf("run %v: %v", args, err)
		code = -1
	}
	return procResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// guardProc runs `auto guard edit --platform <platform>` and requires the
// contract's exit 0 and at most one stderr line, whatever the stdin.
func guardProc(t *testing.T, bin, dir, platform, stdin string) procResult {
	t.Helper()
	got := runGuardProc(t, bin, dir, stdin, "guard", "edit", "--platform", platform)
	require.Equal(t, 0, got.code, "auto guard edit must exit 0; stderr %q", got.stderr)
	require.LessOrEqual(t, strings.Count(got.stderr, "\n"), 1, "more than one stderr line: %q", got.stderr)
	return got
}

// binEdit is P(tool, path) of acceptance.md.
func binEdit(cwd, tool, path string) string {
	return corpusJSON(map[string]any{"session_id": "s1", "cwd": cwd, "hook_event_name": "PreToolUse",
		"tool_name": tool, "tool_input": map[string]any{"file_path": path}})
}

func binOpenCode(cwd string, targets ...any) string {
	return corpusJSON(map[string]any{"platform": "opencode", "cwd": cwd, "tool_name": "patch", "targets": targets})
}

func binWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// binFixtureR builds acceptance fixture R with T: forged and overriding
// entries next to the one real generated file, and the .git/hooks entry.
func binFixtureR(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	binWrite(t, root, "autopus.yaml", "project:\n  name: guard-binary\n")
	manifest := func(policies map[string]string) string {
		files := map[string]any{}
		for p, policy := range policies {
			files[p] = map[string]string{"checksum": "x", "policy": policy}
		}
		data, _ := json.Marshal(map[string]any{"version": "1.0.0", "files": files})
		return string(data)
	}
	binWrite(t, root, ".autopus/claude-code-manifest.json", manifest(map[string]string{
		binSkill: "always", ".claude/settings.json": "merge", "CLAUDE.md": "marker", "pkg/main.go": "always",
		".autopus/brainstorms/BS-001.md": "always", ".agents/skills/x/SKILL.md": "always",
		".git/hooks/pre-commit": "always"}))
	binWrite(t, root, ".autopus/opencode-manifest.json", manifest(map[string]string{".agents/skills/x/SKILL.md": "merge"}))
	for _, rel := range []string{binSkill, ".claude/settings.json", "CLAUDE.md", "pkg/main.go",
		".autopus/brainstorms/BS-001.md", ".agents/skills/x/SKILL.md", ".git/hooks/pre-commit"} {
		binWrite(t, root, rel, "x\n")
	}
	binWrite(t, root, binT, binTBody)
	return root
}

// fixProc runs `auto fix <args>` in root and returns its result.
func fixProc(t *testing.T, bin, root string, args ...string) procResult {
	t.Helper()
	return runGuardProc(t, bin, root, "", append([]string{"fix"}, args...)...)
}

type binListEntry struct {
	Path, State, Integrity string
	CreatedAt              string `json:"created_at"`
	ExpiresAt              string `json:"expires_at"`
}

// listLocks returns `auto fix lock --list --json` after checking its schema.
func listLocks(t *testing.T, bin, root string) []binListEntry {
	t.Helper()
	got := fixProc(t, bin, root, "lock", "--list", "--json")
	require.Equal(t, 0, got.code, got.stderr)
	var doc struct {
		Schema string         `json:"schema"`
		Locks  []binListEntry `json:"locks"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &doc), got.stdout)
	require.Equal(t, "autopus.fix_lock_list.v1", doc.Schema)
	return doc.Locks
}

// recordHash reads the sha256 the store holds for rel; the hash appears in no
// command output while the lock is held.
func recordHash(t *testing.T, root, rel string) string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(root, ".autopus", "runtime", "fix-locks", "*.json"))
	require.NoError(t, err)
	for _, name := range names {
		var rec struct{ Path, SHA256 string }
		if data, err := os.ReadFile(name); err == nil && json.Unmarshal(data, &rec) == nil && rec.Path == rel {
			return rec.SHA256
		}
	}
	t.Fatalf("no lock record for %s", rel)
	return ""
}
