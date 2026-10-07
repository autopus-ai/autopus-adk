package adapter

// SPEC-PANERM-001 T11: transactionStepHook is the fault-injection seam that
// S12 uses to prove a platform update is atomic. These tests do not run in
// parallel because the seam is process-wide.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionStepHook_SeesEveryStepInOrder(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "old.sh"), []byte("old\n"), 0o755))
	var steps []string
	restore := SetTransactionStepHookForTest(func(op, path string) error {
		steps = append(steps, op+" "+path)
		return nil
	})
	defer restore()

	_, err := ApplyTransaction(root, "codex", TransactionPlan{
		Removes:  []TransactionRemove{{Path: "old.sh"}, {Path: "./missing.sh"}},
		Writes:   []TransactionWrite{{Path: filepath.Join("dir", "new.txt"), Content: []byte("new\n")}},
		Manifest: NewManifest("codex"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"remove old.sh", "remove missing.sh", "write dir/new.txt", "write .autopus/codex-manifest.json"}, steps)
}

func TestTransactionStepHook_InjectedWriteFaultRollsBackTheRemoves(t *testing.T) {
	root := t.TempDir()
	scriptPath := filepath.Join(root, ".claude", "hooks", "autopus", "hook-claude-stop.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(scriptPath), 0o755))
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o750))
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte("{\"hooks\":{}}\n"), 0o644))
	injected := errors.New("injected transaction fault")
	restore := SetTransactionStepHookForTest(func(op, _ string) error {
		if op == "write" {
			return injected
		}
		return nil
	})

	_, err := ApplyTransaction(root, "claude-code", TransactionPlan{
		Removes:  []TransactionRemove{{Path: ".claude/hooks/autopus/hook-claude-stop.sh"}},
		Writes:   []TransactionWrite{{Path: ".claude/settings.json", Content: []byte("{}\n")}},
		Manifest: NewManifest("claude-code"),
	})
	restore()

	require.ErrorIs(t, err, injected)
	data, readErr := os.ReadFile(scriptPath)
	require.NoError(t, readErr)
	assert.Equal(t, "#!/bin/sh\nexit 0\n", string(data), "the removed script is restored byte-identical")
	info, statErr := os.Stat(scriptPath)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o750), info.Mode().Perm(), "with its mode")
	settings, readErr := os.ReadFile(settingsPath)
	require.NoError(t, readErr)
	assert.Equal(t, "{\"hooks\":{}}\n", string(settings), "the failed write never lands")
	assert.NoFileExists(t, filepath.Join(root, ".autopus", "claude-code-manifest.json"))

	_, err = ApplyTransaction(root, "claude-code", TransactionPlan{
		Writes: []TransactionWrite{{Path: ".claude/settings.json", Content: []byte("{}\n")}},
	})
	require.NoError(t, err, "restore clears the seam")
}
