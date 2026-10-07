package cli_test

// SPEC-PANERM-001 T3: the stale completion-hook workspaces (testdata/stale_hooks)
// that binary O (v0.50.123) generated. The retraction oracle was red at B and
// runs since T11 landed the group S retraction.

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const staleHooksDir = "testdata/stale_hooks"

// staleHookScripts are the group S script names (spec.md Retired Surface Inventory).
var staleHookScripts = []string{
	"hook-claude-stop.sh", "hook-claude-sessionstart.sh", "hook-codex-stop.sh", "hook-codex-sessionstart.sh",
	"hook-gemini-stop.sh", "hook-gemini-afteragent.sh", "hook-gemini-sessionstart.sh", "hook-opencode-complete.ts",
}

var staleHookSettingsFiles = []string{
	".claude/settings.json", ".codex/hooks.json", ".agents/hooks.json", ".gemini/settings.json", "opencode.json",
}

type staleHookWorkspace struct {
	name     string
	opencode string   // fake `opencode --version` major; empty means no opencode on PATH
	deleted  []string // S11 deletion set
}

var staleHookRetractionWorkspaces = []staleHookWorkspace{
	{name: "W-claude", deleted: []string{
		".claude/hooks/autopus/hook-claude-sessionstart.sh", ".claude/hooks/autopus/hook-claude-stop.sh",
		".claude/hooks/autopus/hook-codex-sessionstart.sh", ".claude/hooks/autopus/hook-codex-stop.sh",
		".claude/hooks/autopus/hook-gemini-afteragent.sh", ".claude/hooks/autopus/hook-gemini-sessionstart.sh",
		".claude/hooks/autopus/hook-gemini-stop.sh", ".claude/hooks/autopus/hook-opencode-complete.ts",
	}},
	{name: "W-codex", deleted: []string{".codex/hooks/autopus/hook-codex-sessionstart.sh", ".codex/hooks/autopus/hook-codex-stop.sh"}},
	{name: "W-agy", deleted: []string{".gemini/hooks/autopus/hook-gemini-afteragent.sh", ".gemini/hooks/autopus/hook-gemini-stop.sh"}},
	{name: "W-oc2", opencode: "2.0.0", deleted: []string{".claude/hooks/autopus/hook-opencode-complete.ts"}},
	{name: "W-oc1", opencode: "1.0.0", deleted: []string{".claude/hooks/autopus/hook-opencode-complete.ts"}},
}

// copyStaleHookWorkspace copies one fixture into a temp dir and replaces the
// {{ROOT}} placeholder with the copy's absolute path.
func copyStaleHookWorkspace(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	src := filepath.Join(staleHooksDir, name)
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(src, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data = bytes.ReplaceAll(data, []byte("{{ROOT}}"), []byte(root))
		target := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	require.NoError(t, err)
	return root
}

// useStaleHookEnv pins HOME to a scratch dir and PATH to the workspace's fake
// OpenCode CLI only, so no host provider binary or catalog is reachable.
func useStaleHookEnv(t *testing.T, opencode string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path := t.TempDir()
	if opencode != "" {
		abs, err := filepath.Abs(filepath.Join(staleHooksDir, "fakebin", "opencode-"+opencode))
		require.NoError(t, err)
		path = abs
	}
	t.Setenv("PATH", path)
}

func staleHookFiles(t *testing.T, root string) map[string]bool {
	t.Helper()
	files := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, ".autopus/txns/") && !strings.HasPrefix(rel, ".autopus/backup/") {
			files[rel] = true
		}
		return nil
	})
	require.NoError(t, err)
	return files
}

func runStaleHookUpdate(t *testing.T, root string) (string, error) {
	t.Helper()
	cmd := newTestRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--dir", root})
	err := cmd.Execute()
	return out.String(), err
}

func readJSONTree(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var tree map[string]any
	require.NoError(t, json.Unmarshal(data, &tree))
	return tree
}

// staleHookEntries returns the event entries of a nested settings file
// (.claude/settings.json, .codex/hooks.json, .gemini/settings.json).
func staleHookEntries(tree map[string]any, event string) []map[string]any {
	hooks, _ := tree["hooks"].(map[string]any)
	raw, _ := hooks[event].([]any)
	entries := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if entry, ok := item.(map[string]any); ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func entriesWithMatcherPrefix(entries []map[string]any, prefix string) []map[string]any {
	var out []map[string]any
	for _, entry := range entries {
		if matcher, _ := entry["matcher"].(string); strings.HasPrefix(matcher, prefix) {
			out = append(out, entry)
		}
	}
	return out
}

func TestStaleHookFixtures_HoldTheirGroupSMembers(t *testing.T) {
	for _, ws := range staleHookRetractionWorkspaces {
		files := staleHookFiles(t, filepath.Join(staleHooksDir, ws.name))
		for _, rel := range ws.deleted {
			assert.True(t, files[rel], "%s must hold group S member %s", ws.name, rel)
		}
		var refs []string
		for _, rel := range staleHookSettingsFiles {
			if data, err := os.ReadFile(filepath.Join(staleHooksDir, ws.name, rel)); err == nil {
				refs = append(refs, staleHookScriptRefs(data)...)
			}
		}
		assert.NotEmpty(t, refs, "%s settings must reference a group S script", ws.name)
	}
}

func TestStaleHookFixtures_UpdateRetractsOnlyGroupS(t *testing.T) {
	for _, ws := range staleHookRetractionWorkspaces {
		t.Run(ws.name, func(t *testing.T) {
			useStaleHookEnv(t, ws.opencode)
			root := copyStaleHookWorkspace(t, ws.name)
			original := map[string]map[string]any{}
			for _, rel := range staleHookSettingsFiles {
				if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
					original[rel] = readJSONTree(t, filepath.Join(root, rel))
				}
			}
			before := staleHookFiles(t, root)
			out, err := runStaleHookUpdate(t, root)
			require.NoError(t, err, out)
			after := staleHookFiles(t, root)

			var deleted []string
			for rel := range before {
				if !after[rel] {
					deleted = append(deleted, rel)
				}
			}
			sort.Strings(deleted)
			assert.Equal(t, ws.deleted, deleted, "deleted files")

			for rel, tree := range original {
				data, err := os.ReadFile(filepath.Join(root, rel))
				require.NoError(t, err)
				assert.Empty(t, staleHookScriptRefs(data), "%s still references group S scripts", rel)
				assertStaleHookUserEntriesKept(t, rel, tree, readJSONTree(t, filepath.Join(root, rel)))
			}
		})
	}
}

// assertStaleHookUserEntriesKept checks the user-authored parts of one
// settings file: separate user entries, the mixed entry, the user hook set,
// and the user OpenCode plugin entries.
func assertStaleHookUserEntriesKept(t *testing.T, rel string, before, after map[string]any) {
	t.Helper()
	switch rel {
	case ".agents/hooks.json":
		assert.Equal(t, before["user-hooks"], after["user-hooks"], "user hook set")
	case "opencode.json":
		for _, key := range []string{"plugin", "plugins"} {
			beforeItems, _ := before[key].([]any)
			afterItems, _ := after[key].([]any)
			var kept []any
			for _, entry := range beforeItems {
				if text, _ := json.Marshal(entry); !strings.Contains(string(text), "hook-opencode-complete.ts") &&
					!strings.Contains(string(text), "autopus-hooks.js") {
					kept = append(kept, entry)
				}
			}
			var got []any
			for _, entry := range afterItems {
				if text, _ := json.Marshal(entry); !strings.Contains(string(text), "autopus-hooks.js") {
					got = append(got, entry)
				}
			}
			assert.Equal(t, kept, got, "user OpenCode %s entries", key)
		}
	default:
		hooks, _ := before["hooks"].(map[string]any)
		for event := range hooks {
			wantUser := entriesWithMatcherPrefix(staleHookEntries(before, event), "user-")
			gotEntries := staleHookEntries(after, event)
			assert.Equal(t, wantUser, entriesWithMatcherPrefix(gotEntries, "user-"), "%s %s user entries", rel, event)
			if len(wantUser) > 0 && len(gotEntries) > 0 {
				assert.Equal(t, wantUser[0], gotEntries[0], "%s %s user entry stays first", rel, event)
			}
			for _, mixed := range entriesWithMatcherPrefix(gotEntries, "mixed") {
				assert.Equal(t, []any{map[string]any{"command": "./scripts/notify.sh", "timeout": float64(10), "type": "command"}},
					mixed["hooks"], "%s %s mixed entry keeps only the user handler", rel, event)
			}
			if len(entriesWithMatcherPrefix(staleHookEntries(before, event), "mixed")) > 0 {
				assert.Len(t, entriesWithMatcherPrefix(gotEntries, "mixed"), 1, "%s %s mixed entry survives", rel, event)
			}
		}
	}
}

// staleHookScriptRefs lists the group S script names that data mentions.
func staleHookScriptRefs(data []byte) []string {
	var refs []string
	for _, script := range staleHookScripts {
		if bytes.Contains(data, []byte(script)) {
			refs = append(refs, script)
		}
	}
	return refs
}

func TestStaleHookFixtures_UpdateKeepsReferencedAndRejectedPlugins(t *testing.T) {
	const tsRel = ".claude/hooks/autopus/hook-opencode-complete.ts"
	for _, tc := range []struct {
		name, opencode, wantErr string
	}{
		{name: "W-mix"},
		{name: "W-oc-bad", opencode: "2.0.0", wantErr: "invalid native plugins array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useStaleHookEnv(t, tc.opencode)
			root := copyStaleHookWorkspace(t, tc.name)
			readBoth := func() (string, string) {
				config, err := os.ReadFile(filepath.Join(root, "opencode.json"))
				require.NoError(t, err)
				script, err := os.ReadFile(filepath.Join(root, tsRel))
				require.NoError(t, err)
				return string(config), string(script)
			}
			configBefore, scriptBefore := readBoth()
			out, err := runStaleHookUpdate(t, root)
			if tc.wantErr != "" {
				require.Error(t, err, out)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err, out)
			}
			configAfter, scriptAfter := readBoth()
			assert.Equal(t, configBefore, configAfter, "opencode.json stays byte-identical")
			assert.Equal(t, scriptBefore, scriptAfter, "the referenced .ts stays byte-identical")
		})
	}
}
