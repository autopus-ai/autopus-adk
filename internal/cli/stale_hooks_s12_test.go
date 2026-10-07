package cli_test

// SPEC-PANERM-001 T3 (S12): hook retraction is idempotent, bounded, and
// atomic. The second `auto update` changes nothing outside the transaction
// records and never touches the user-level settings file; a fault injected
// into the first write after the removes leaves the claude-code surface
// byte-identical, and a clean rerun reaches the S11 end state. Both oracles
// were red at B and run since T11; the W-mix and W-oc-bad guards live in
// stale_hooks_fixture_test.go, and the binary O revert path is RFP-3 (T19).

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// setTransactionStepHook installs hook as adapter.transactionStepHook, the
// seam of plan.md T11 that ApplyTransaction calls before each step with the
// step kind ("remove" or "write"; the manifest is a write) and the
// root-relative path, and returns a restore func.
var setTransactionStepHook = adapter.SetTransactionStepHookForTest

var staleHookManifestGeneratedAt = regexp.MustCompile(`"generated_at":\s*"[^"]*"`)

// staleHookHash is H(w) of acceptance.md: the sorted (path, mode, sha256) list
// of every file under root except .git/ and the transaction records, with
// generated_at removed from each platform manifest before hashing.
func staleHookHash(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, prefix := range []string{".git/", ".autopus/txns/", ".autopus/backup/"} {
			if strings.HasPrefix(rel, prefix) {
				return nil
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, ".autopus/") && strings.HasSuffix(rel, "-manifest.json") {
			data = staleHookManifestGeneratedAt.ReplaceAll(data, nil)
		}
		sum := sha256.Sum256(data)
		entries = append(entries, fmt.Sprintf("%s %o %s", rel, info.Mode().Perm(), hex.EncodeToString(sum[:])))
		return nil
	})
	require.NoError(t, err)
	sort.Strings(entries)
	return entries
}

// writeUserLevelClaudeSettings puts a Stop handler for hook-claude-stop.sh in
// the scratch HOME; auto update must never edit it.
func writeUserLevelClaudeSettings(t *testing.T) (string, []byte) {
	t.Helper()
	path := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	data := []byte(`{"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command",` +
		`"command":"$HOME/.claude/hooks/autopus/hook-claude-stop.sh","timeout":10}]}]}}` + "\n")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path, data
}

func TestStaleHookFixtures_SecondUpdateChangesNothing(t *testing.T) {
	for _, ws := range staleHookRetractionWorkspaces {
		t.Run(ws.name, func(t *testing.T) {
			useStaleHookEnv(t, ws.opencode)
			homeSettings, homeData := writeUserLevelClaudeSettings(t)
			root := copyStaleHookWorkspace(t, ws.name)

			out, err := runStaleHookUpdate(t, root)
			require.NoError(t, err, out)
			for _, rel := range ws.deleted {
				assert.NoFileExists(t, filepath.Join(root, rel), "the first update retracts group S")
			}
			first, firstFiles := staleHookHash(t, root), staleHookFiles(t, root)

			out, err = runStaleHookUpdate(t, root)
			require.NoError(t, err, out)
			assert.Equal(t, first, staleHookHash(t, root), "H(w) after the second update")
			assert.Equal(t, firstFiles, staleHookFiles(t, root), "the second update removes and adds no file")
			got, err := os.ReadFile(homeSettings)
			require.NoError(t, err)
			assert.Equal(t, string(homeData), string(got), "the user-level settings file stays byte-identical")
		})
	}
}

// claudeSurface reads the claude-code transaction's settings, group S
// scripts, and manifest.
func claudeSurface(t *testing.T, root string) map[string]string {
	t.Helper()
	surface := map[string]string{}
	paths := []string{".claude/settings.json", ".autopus/claude-code-manifest.json"}
	paths = append(paths, staleHookRetractionWorkspaces[0].deleted...)
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, err, rel)
		surface[rel] = string(data)
	}
	return surface
}

func TestStaleHookFixtures_FailedUpdateKeepsClaudeSurface(t *testing.T) {
	useStaleHookEnv(t, "")
	root := copyStaleHookWorkspace(t, "W-claude")
	before := claudeSurface(t, root)

	injected := errors.New("injected transaction fault")
	removes, failed := 0, 0
	restore := setTransactionStepHook(func(op, _ string) error {
		if op == "remove" {
			removes++
		}
		if op == "write" && removes > 0 && failed == 0 {
			failed++
			return injected
		}
		return nil
	})
	out, err := runStaleHookUpdate(t, root)
	restore()

	require.Error(t, err, out)
	assert.True(t, strings.HasPrefix(err.Error(), "플랫폼 업데이트 실패: claude-code: "), err.Error())
	assert.Contains(t, out, "  ✗ claude-code: ")
	assert.Contains(t, out, injected.Error())
	assert.Equal(t, 1, failed, "the fault lands on the first write after the removes")
	assert.Equal(t, before, claudeSurface(t, root), "a failed claude-code transaction keeps its files byte-identical")

	out, err = runStaleHookUpdate(t, root)
	require.NoError(t, err, out)
	for _, rel := range staleHookRetractionWorkspaces[0].deleted {
		assert.NoFileExists(t, filepath.Join(root, rel), "a clean rerun reaches the S11 end state")
	}
}
