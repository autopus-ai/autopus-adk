package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func egRunGuard(t *testing.T, stdin string, args ...string) egResult {
	t.Helper()
	return egRun(t, stdin, append([]string{"guard", "edit"}, args...)...)
}

func (r egResult) wantAllow(t *testing.T, label string) {
	t.Helper()
	if r.code != 0 || r.err != nil || r.stdout != "" || strings.Count(r.stderr, "\n") > 1 {
		t.Errorf("%s: want exit 0, empty stdout, at most one stderr line; got %+v", label, r)
	}
}

// REQ-EG-02 and S1: `auto guard edit` answers a protected edit in the deny
// encoding of the platform named on its command line, exiting 0.
func TestGuardEditCLI_DeniesInThePlatformEncoding(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	opencode, _ := json.Marshal(map[string]any{"platform": "opencode", "cwd": root, "tool_name": "patch",
		"targets": []string{"pkg/a.go", egSkill}})
	codex, _ := json.Marshal(map[string]any{"cwd": root, "hook_event_name": "PreToolUse", "tool_name": "apply_patch",
		"tool_input": map[string]any{"command": "*** Begin Patch\n*** Update File: pkg/a.go\n*** Move to: " +
			egSkill + "\n@@\n-a\n+b\n*** End Patch\n"}})
	gemini, _ := json.Marshal(map[string]any{"cwd": root, "hook_event_name": "BeforeTool", "tool_name": "replace",
		"tool_input": map[string]any{"file_path": egSkill, "old_string": "x", "new_string": "y"}})
	cases := []struct {
		platform, stdin, want string
	}{
		{"claude-code", egEdit(root, "Edit", egSkill), egClaudeDeny(egGSCon)},
		{"opencode", string(opencode), `{"decision":"deny","reason":"` + egGSCon + `"}`},
		{"codex", string(codex), egClaudeDeny(egGSCon)},
		{"gemini", string(gemini), `{"decision":"deny","reason":"` + egGSCon + `"}` + "\n"},
	}
	for _, c := range cases {
		got := egRunGuard(t, c.stdin, "--platform", c.platform)
		if got.code != 0 || got.err != nil || got.stdout != c.want || got.stderr != "" {
			t.Errorf("%s: got %+v\nwant stdout %q", c.platform, got, c.want)
		}
	}
}

// S1 and S7: every other call, and every call-level fault, exits 0 with empty
// stdout and at most one stderr line.
func TestGuardEditCLI_AllowsEveryOtherCallWithExitZero(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	for label, stdin := range map[string]string{
		"settings file":     egEdit(root, "Edit", ".claude/settings.json"),
		"code under test":   egEdit(root, "Write", "internal/foo/foo.go"),
		"new skill":         egEdit(root, "Write", ".claude/skills/new/SKILL.md"),
		"bash":              `{"tool_name":"Bash","tool_input":{"command":"ls"}}`,
		"truncated json":    `{"tool_input":`,
		"zero bytes":        "",
		"1048577 bytes":     strings.Repeat("a", 1<<20+1),
		"no target":         `{"tool_name":"Edit","tool_input":{}}`,
		"non-string target": `{"tool_name":"Edit","tool_input":{"file_path":42}}`,
	} {
		egRunGuard(t, stdin, "--platform", "claude-code").wantAllow(t, label)
	}
}

// REQ-EG-11: the guard exits 0 even for an unknown platform or a command line
// it cannot parse, and a newer generated command line still decides.
func TestGuardEditCLI_UnknownPlatformAndUsageErrors_AllowWithExitZero(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	deny := egEdit(root, "Edit", egSkill)
	for label, c := range map[string]struct {
		args   []string
		stderr string
	}{
		// Antigravity is advisory-only: no lane registers the guard there.
		"antigravity has no dialect": {[]string{"--platform", "antigravity-cli"},
			"autopus edit-guard: allow (unknown platform)\n"},
		"no platform":            {nil, "autopus edit-guard: allow (unknown platform)\n"},
		"platform without value": {[]string{"--platform"}, "autopus edit-guard: allow (usage error)\n"},
	} {
		got := egRunGuard(t, deny, c.args...)
		got.wantAllow(t, label)
		if got.stderr != c.stderr {
			t.Errorf("%s: stderr = %q, want %q", label, got.stderr, c.stderr)
		}
	}
	got := egRunGuard(t, deny, "--platform", "claude-code", "--from-a-newer-release", "x", "extra")
	if got.code != 0 || got.stdout != egClaudeDeny(egGSCon) {
		t.Errorf("unknown flag and argument: %+v", got)
	}
}

// REQ-EG-20 and S15: AUTOPUS_EDIT_GUARD=off allows every edit with empty
// stdout and no stderr; any other value keeps the guard on.
func TestGuardEditCLI_EnvironmentOff_AllowsSilently(t *testing.T) {
	now := egT0
	egHermetic(t, &now)
	root := egProject(t)
	deny := egEdit(root, "Edit", egSkill)
	t.Setenv("AUTOPUS_EDIT_GUARD", "off")
	for _, args := range [][]string{{"--platform", "claude-code"}, {"--platform", "gemini"}, {"--platform"}} {
		if got := egRunGuard(t, deny, args...); got.code != 0 || got.stdout != "" || got.stderr != "" {
			t.Errorf("off %q: %+v", args, got)
		}
	}
	t.Setenv("AUTOPUS_EDIT_GUARD", "on")
	if got := egRunGuard(t, deny, "--platform", "claude-code"); got.stdout != egClaudeDeny(egGSCon) {
		t.Errorf("on: %+v", got)
	}
}
