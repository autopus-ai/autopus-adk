package cli_test

// Shared fixtures of the SPEC-EDITGUARD-001 T15 verification corpora: a
// five-platform consumer project, its manifests read independently of
// pkg/editguard, and the file-editing payload shapes each enforced lane's host
// sends (the A1, A2, A3, and T11 fixtures under pkg/editguard/testdata).

import (
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// corpusPlatforms are the installed platforms of every matrix lane.
	corpusPlatforms = "claude-code,codex,opencode,antigravity-cli,omp"
	// corpusManifestEnv names a directory whose *-manifest.json files replace
	// the generated ones, so the corpora also run over a real repository's.
	corpusManifestEnv = "AUTOPUS_EDITGUARD_CORPUS_MANIFESTS"
)

// corpusNamespace is the guard namespace of spec.md Outcome Boundary, spelled
// out here rather than taken from pkg/workflow so the corpus cannot agree with
// a wrong namespace by construction.
var corpusNamespace = []string{".claude/", ".codex/", ".gemini/", ".opencode/", ".agents/", ".omp/", ".autopus/plugins/"}

func inCorpusNamespace(p string) bool {
	if strings.HasPrefix(p, ".claude/worktrees/") {
		return false
	}
	for _, prefix := range corpusNamespace {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// corpusProject runs `auto init` for every platform in a fresh directory and
// returns the symlink-free project root. Init probes the installed host CLIs
// (`opencode --version`, `codex --version`), which on a fresh HOME run their
// first-time setup: HOME, the XDG directories, and CODEX_HOME are pinned below
// the test, and PATH is cut to the system directories so no host CLI runs.
// On the T15 host that took init from 15.5 s to 2.9 s and left all 544
// manifest entries and their policies identical.
func corpusProject(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS != "windows" {
		t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+"/bin")
	}
	for _, xdg := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(xdg, filepath.Join(home, strings.ToLower(xdg)))
	}
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex-home"))
	t.Setenv("AUTOPUS_EDIT_GUARD", "")
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// Root-local git hooks are only written into a real gitdir (see
	// adapter.SupportsRootGitHooks), and the corpus covers them.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	runEditGuardCLI(t, "init", "--dir", root, "--project", "corpus", "--platforms", corpusPlatforms)
	if src := os.Getenv(corpusManifestEnv); src != "" {
		generated, err := filepath.Glob(filepath.Join(root, ".autopus", "*-manifest.json"))
		require.NoError(t, err)
		for _, name := range generated {
			require.NoError(t, os.Remove(name))
		}
		real, err := filepath.Glob(filepath.Join(src, "*-manifest.json"))
		require.NoError(t, err)
		require.NotEmpty(t, real, "%s=%s holds no manifest", corpusManifestEnv, src)
		for _, name := range real {
			require.NoError(t, os.WriteFile(filepath.Join(root, ".autopus", filepath.Base(name)),
				readEditGuardFile(t, name), 0o644))
		}
		t.Logf("manifests copied from %s", src)
	}
	return root
}

// corpusEntry is what every manifest of a root says about one path.
type corpusEntry struct {
	always   []string // manifests listing the path always, in lexical order
	override bool     // some manifest lists it merge or marker
}

// readCorpusManifests parses every `.autopus/*-manifest.json` of root in
// lexical order and returns the entries by path and the manifest names.
func readCorpusManifests(t *testing.T, root string) (map[string]*corpusEntry, []string) {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(root, ".autopus", "*-manifest.json"))
	require.NoError(t, err)
	sort.Strings(names)
	entries := map[string]*corpusEntry{}
	rels := make([]string, 0, len(names))
	for _, name := range names {
		rel := ".autopus/" + filepath.Base(name)
		rels = append(rels, rel)
		var doc struct {
			Files map[string]struct {
				Policy string `json:"policy"`
			} `json:"files"`
		}
		require.NoError(t, json.Unmarshal(readEditGuardFile(t, name), &doc), rel)
		for p, file := range doc.Files {
			entry := entries[p]
			if entry == nil {
				entry = &corpusEntry{}
				entries[p] = entry
			}
			switch file.Policy {
			case "always":
				entry.always = append(entry.always, rel)
			case "merge", "marker":
				entry.override = true
			default:
				t.Fatalf("%s lists %s with unknown policy %q", rel, p, file.Policy)
			}
		}
	}
	return entries, rels
}

// gsConReason is the GS-CON row of spec.md's reason table for a plain path.
func gsConReason(t *testing.T, p, manifest string) string {
	t.Helper()
	// The literal below is only the contract text for paths display leaves
	// unchanged and JSON leaves unescaped.
	require.Equal(t, path.Clean(p), p, "corpus path is not clean")
	require.LessOrEqual(t, len(p), 256, "corpus path is longer than the display cap")
	require.False(t, strings.ContainsAny(p, "\"\\<>&") || strings.ContainsFunc(p, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}), "corpus path %q needs escaping", p)
	return "autopus edit-guard [generated_surface]: " + p + " is generated (manifest " + manifest +
		", policy always). Change autopus.yaml or the upstream Autopus source, then run: auto update"
}

// corpusShape is one file-editing call shape of one enforced lane.
type corpusShape struct {
	lane, tool string
	payload    func(root, target string) string
	deny       func(reason string) string
}

func hookDeny(reason string) string {
	return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",` +
		`"permissionDecisionReason":"` + reason + `"}}` + "\n"
}

func decisionDeny(reason string) string { return `{"decision":"deny","reason":"` + reason + `"}` }

func decisionDenyLine(reason string) string { return decisionDeny(reason) + "\n" }

// absTarget is the target as a host that sends absolute paths spells it.
func absTarget(root, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(root, target)
}

func corpusJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func claudeShape(tool string, input func(root, target string) map[string]any) corpusShape {
	return corpusShape{lane: "claude-code", tool: tool, deny: hookDeny, payload: func(root, target string) string {
		return corpusJSON(map[string]any{"session_id": "s1", "transcript_path": filepath.Join(root, "t.jsonl"),
			"cwd": root, "permission_mode": "acceptEdits", "hook_event_name": "PreToolUse", "tool_name": tool,
			"tool_input": input(root, target), "tool_use_id": "toolu_corpus"})
	}}
}

func openCodeShape(tool string, targets func(target string) []string) corpusShape {
	return corpusShape{lane: "opencode", tool: tool, deny: decisionDeny, payload: func(root, target string) string {
		return corpusJSON(map[string]any{"platform": "opencode", "cwd": root, "tool_name": tool, "targets": targets(target)})
	}}
}

func codexShape(tool string, patch func(target string) string) corpusShape {
	return corpusShape{lane: "codex", tool: tool, deny: hookDeny, payload: func(root, target string) string {
		return corpusJSON(map[string]any{"session_id": "s1", "turn_id": "t1", "transcript_path": nil, "cwd": root,
			"hook_event_name": "PreToolUse", "model": "gpt-5.5", "permission_mode": "bypassPermissions",
			"tool_name": "apply_patch", "tool_input": map[string]any{"command": "*** Begin Patch\n" + patch(target) +
				"*** End Patch\n"}, "tool_use_id": "call_corpus"})
	}}
}

func geminiShape(tool string, input func(root, target string) map[string]any) corpusShape {
	return corpusShape{lane: "gemini", tool: tool, deny: decisionDenyLine, payload: func(root, target string) string {
		return corpusJSON(map[string]any{"session_id": "s1", "transcript_path": filepath.Join(root, "t.jsonl"),
			"cwd": root, "hook_event_name": "BeforeTool", "timestamp": "2026-10-07T02:00:27.159Z", "tool_name": tool,
			"tool_input": input(root, target)})
	}}
}

// corpusShapes are the twelve file-editing shapes of the four enforced lanes:
// absolute and relative targets, a patch whose protected target comes second,
// and a move destination.
func corpusShapes() []corpusShape {
	return []corpusShape{
		claudeShape("Edit", func(root, target string) map[string]any {
			return map[string]any{"file_path": absTarget(root, target), "old_string": "a", "new_string": "b", "replace_all": false}
		}),
		claudeShape("Write", func(root, target string) map[string]any {
			return map[string]any{"file_path": absTarget(root, target), "content": "x\n"}
		}),
		claudeShape("MultiEdit", func(_, target string) map[string]any {
			return map[string]any{"file_path": target, "edits": []any{map[string]any{"old_string": "a", "new_string": "b"}}}
		}),
		openCodeShape("edit", func(target string) []string { return []string{target} }),
		openCodeShape("write", func(target string) []string { return []string{target} }),
		openCodeShape("patch", func(target string) []string { return []string{"pkg/a.go", target} }),
		codexShape("update", func(target string) string { return "*** Update File: " + target + "\n@@\n-a\n+b\n" }),
		codexShape("add", func(target string) string { return "*** Add File: " + target + "\n+x\n" }),
		codexShape("delete", func(target string) string { return "*** Delete File: " + target + "\n" }),
		codexShape("move", func(target string) string {
			return "*** Update File: notes.txt\n*** Move to: " + target + "\n@@\n-a\n+b\n"
		}),
		geminiShape("write_file", func(root, target string) map[string]any {
			return map[string]any{"file_path": absTarget(root, target), "content": "x\n"}
		}),
		geminiShape("replace", func(_, target string) map[string]any {
			return map[string]any{"file_path": target, "instruction": "i", "old_string": "a", "new_string": "b"}
		}),
	}
}

// runCorpusGuard runs `auto guard edit --platform <lane>` in process; the
// command must exit 0 whatever it decides.
func runCorpusGuard(t *testing.T, lane, stdin string) (stdout, stderr string) {
	t.Helper()
	cmd := newTestRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"guard", "edit", "--platform", lane})
	require.NoError(t, cmd.Execute(), "auto guard edit must exit 0")
	return out.String(), errOut.String()
}
