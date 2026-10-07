package cli_test

// SPEC-EDITGUARD-001 T15 at the process level, through the real `auto`
// binary built without fault seams: every stdin of S1, S7, and S16 exits 0
// (never 2) with the contract's exact stdout (CE-3), and the S17 lock
// transitions that need no seam hold across concurrent processes, observed
// through exit status, `auto fix lock --list --json`, and P(Edit, T) (CE-4).

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditGuardBinary_AcceptanceStdinCorpus_ExitsZeroWithContractBytes(t *testing.T) {
	t.Parallel()
	bin := buildGuardBinary(t)
	claudeDeny := hookDeny(binGSCon)
	type row struct {
		label, platform, stdin, stdout, stderr string // stderr "*" accepts any single line
	}
	r := binFixtureR(t)
	rows := []row{
		{"S1.1 Edit", "claude-code", binEdit(r, "Edit", binSkill), claudeDeny, ""},
		{"S1.2 MultiEdit absolute", "claude-code", binEdit(r, "MultiEdit", filepath.Join(r, binSkill)), claudeDeny, ""},
		{"S1.3 merge", "claude-code", binEdit(r, "Edit", ".claude/settings.json"), "", ""},
		{"S1.3 marker", "claude-code", binEdit(r, "Edit", "CLAUDE.md"), "", ""},
		{"S1.3 unlisted", "claude-code", binEdit(r, "Write", ".claude/commands/my-cmd.md"), "", ""},
		{"S1.4 brainstorm", "claude-code", binEdit(r, "Write", ".autopus/brainstorms/BS-001.md"), "", ""},
		{"S1.4 spec", "claude-code", binEdit(r, "Write", ".autopus/specs/SPEC-X-001/spec.md"), "", ""},
		{"S1.4 forged source", "claude-code", binEdit(r, "Edit", "pkg/main.go"), "", ""},
		{"S1.5 always plus merge", "claude-code", binEdit(r, "Edit", ".agents/skills/x/SKILL.md"), "", ""},
		{"S1.5 config.toml", "claude-code", binEdit(r, "Edit", "config.toml"), "", ""},
		{"S1.5 new file", "claude-code", binEdit(r, "Write", ".claude/skills/new/SKILL.md"), "", ""},
		{"S1.6 Bash", "claude-code", `{"tool_name":"Bash","tool_input":{"command":"ls"}}`, "", "*"},
		{"S1 .git/hooks", "claude-code", binEdit(r, "Edit", ".git/hooks/pre-commit"), "", ""},
		{"S7 truncated", "claude-code", `{"tool_input":`, "", "autopus edit-guard: allow (payload malformed)\n"},
		{"S7 zero bytes", "claude-code", "", "", "autopus edit-guard: allow (empty payload)\n"},
		{"S7 1048577 bytes", "claude-code", strings.Repeat("a", 1<<20+1), "", "autopus edit-guard: allow (payload over 1 MiB)\n"},
		{"S7 no target", "claude-code", `{"tool_name":"Edit","tool_input":{}}`, "", "autopus edit-guard: allow (no target path)\n"},
		{"S7 numeric target", "claude-code", `{"tool_name":"Edit","tool_input":{"file_path":42}}`, "", "*"},
		{"S16 opencode malformed second", "opencode", binOpenCode(r, binSkill, 42), decisionDeny(binGSCon), ""},
		{"S16 opencode malformed first", "opencode", binOpenCode(r, 42, binSkill), decisionDeny(binGSCon), ""},
		{"S16 opencode only malformed", "opencode", binOpenCode(r, 42), "", "autopus edit-guard: allow (no target path)\n"},
	}
	for _, c := range rows {
		got := guardProc(t, bin, r, c.platform, c.stdin)
		assert.Equal(t, c.stdout, got.stdout, c.label)
		if c.stderr != "*" {
			assert.Equal(t, c.stderr, got.stderr, c.label)
		}
	}

	// S16 rows with a faulty state, one fresh fixture each.
	lockedR := func(paths ...string) string {
		root := binFixtureR(t)
		require.Equal(t, 0, fixProc(t, bin, root, append([]string{"lock", "--"}, paths...)...).code)
		return root
	}
	symlinked := func(paths ...string) string {
		root := lockedR(paths...)
		moved := filepath.Join(t.TempDir(), "runtime")
		require.NoError(t, os.Rename(filepath.Join(root, ".autopus", "runtime"), moved))
		require.NoError(t, os.Symlink(moved, filepath.Join(root, ".autopus", "runtime")))
		return root
	}
	corrupt := func(root, rel, content string) string { binWrite(t, root, rel, content); return root }
	const manifestFault = "autopus edit-guard: allow (manifest unreadable: .autopus/opencode-manifest.json)\n"
	faultRows := []struct {
		label, root, target, stdout, stderr string
	}{
		{"symlinked runtime holding the skill's lock", symlinked(binSkill), binSkill, claudeDeny, ""},
		{"symlinked runtime holding T's lock", symlinked(binT), binT, "",
			"autopus edit-guard: allow (lock state unusable: .autopus/runtime/fix-locks)\n"},
		{"corrupt record next to T's lock", corrupt(lockedR(binT), ".autopus/runtime/fix-locks/planted.json", "not json"),
			binT, hookDeny(binTFL), ""},
		{"T locked, claude manifest corrupt", corrupt(lockedR(binT), ".autopus/claude-code-manifest.json", "{"),
			binT, hookDeny(binTFL), ""},
		{"opencode manifest corrupt, overridden file", corrupt(binFixtureR(t), ".autopus/opencode-manifest.json", "{"),
			".agents/skills/x/SKILL.md", "", manifestFault},
		{"opencode manifest corrupt, generated file", corrupt(binFixtureR(t), ".autopus/opencode-manifest.json", "{"),
			binSkill, "", manifestFault},
	}
	for _, c := range faultRows {
		got := guardProc(t, bin, c.root, "claude-code", binEdit(c.root, "Edit", c.target))
		assert.Equal(t, c.stdout, got.stdout, c.label)
		assert.Equal(t, c.stderr, got.stderr, c.label)
	}
}

func TestEditGuardBinary_LockTransitionsAcrossConcurrentProcesses(t *testing.T) {
	t.Parallel()
	bin := buildGuardBinary(t)
	root := binFixtureR(t)
	binWrite(t, root, "internal/foo/u_test.go", binUBody)
	editT := binEdit(root, "Edit", binT)

	// Step 1: a batch with a missing file locks nothing.
	assert.Equal(t, 1, fixProc(t, bin, root, "lock", binT, "missing_test.go").code)
	assert.Empty(t, listLocks(t, bin, root))

	// Step 4: eight concurrent processes lock T exactly once. Step 1 failed
	// validation before creating any state, so they also race to create the
	// store and its lock file (the T15 finding fixed in openStoreLock).
	concurrently(t, 8, func(int) procResult { return fixProc(t, bin, root, "lock", binT) }, 0)
	locks := listLocks(t, bin, root)
	require.Len(t, locks, 1)
	assert.Equal(t, binListEntry{Path: binT, State: "active", Integrity: "unchanged",
		CreatedAt: locks[0].CreatedAt, ExpiresAt: locks[0].ExpiresAt}, locks[0])
	requireTTL(t, locks[0], 24*time.Hour)
	assert.Equal(t, binTHash, recordHash(t, root, binT))

	// Step 5: rewrites racing re-locks never replace the first hash.
	concurrently(t, 8, func(i int) procResult {
		if i%2 == 0 {
			assert.NoError(t, os.WriteFile(filepath.Join(root, binT), []byte(binWeak), 0o644))
			return procResult{}
		}
		return fixProc(t, bin, root, "lock", binT)
	}, 0)
	assert.Equal(t, binTHash, recordHash(t, root, binT))
	assert.Equal(t, hookDeny(binTFL), guardProc(t, bin, root, "claude-code", editT).stdout)

	// Step 6: an unlock naming an unlocked path removes nothing.
	assert.Equal(t, 1, fixProc(t, bin, root, "unlock", binT, "not_locked_test.go").code)
	require.Len(t, listLocks(t, bin, root), 1)

	// Step 7 without the pause: the verdict reads modified (exit 3), and the
	// next lock records the content present now.
	got := fixProc(t, bin, root, "unlock", "--json", "--", binT)
	assert.Equal(t, 3, got.code)
	assert.Equal(t, `{"schema":"autopus.fix_unlock.v1","results":[{"path":"`+binT+`","verdict":"modified",`+
		`"locked_sha256":"`+binTHash+`","current_sha256":"`+binWeakSH+`"}]}`+"\n", got.stdout)
	require.Equal(t, 0, fixProc(t, bin, root, "lock", "--", binT, "internal/foo/u_test.go").code)
	assert.Equal(t, binWeakSH, recordHash(t, root, binT))

	// Step 9: --all releases a corrupt record as unverifiable (exit 3).
	binWrite(t, root, ".autopus/runtime/fix-locks/planted.json", "not json")
	got = fixProc(t, bin, root, "unlock", "--all", "--json")
	assert.Equal(t, 3, got.code)
	assert.Equal(t, `{"schema":"autopus.fix_unlock.v1","results":[`+
		`{"path":".autopus/runtime/fix-locks/planted.json","verdict":"unverifiable","locked_sha256":"","current_sha256":""},`+
		`{"path":"`+binT+`","verdict":"unchanged","locked_sha256":"`+binWeakSH+`","current_sha256":"`+binWeakSH+`"},`+
		`{"path":"internal/foo/u_test.go","verdict":"unchanged","locked_sha256":"`+binUHash+
		`","current_sha256":"`+binUHash+`"}]}`+"\n", got.stdout)
	assert.Empty(t, listLocks(t, bin, root))

	// Step 10: with the locks gone the guard allows T again.
	assert.Equal(t, procResult{}, guardProc(t, bin, root, "claude-code", editT))
}

// concurrently starts n workers at once and requires every exit status to be
// want; a worker that runs no process returns the zero result.
func concurrently(t *testing.T, n int, work func(i int) procResult, want int) {
	t.Helper()
	results := make([]procResult, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			results[i] = work(i)
		})
	}
	close(start)
	wg.Wait()
	for i, got := range results {
		assert.Equal(t, want, got.code, "worker %d: %+v", i, got)
	}
}

func requireTTL(t *testing.T, entry binListEntry, ttl time.Duration) {
	t.Helper()
	created, err := time.Parse(time.RFC3339, entry.CreatedAt)
	require.NoError(t, err)
	expires, err := time.Parse(time.RFC3339, entry.ExpiresAt)
	require.NoError(t, err)
	require.Equal(t, ttl, expires.Sub(created))
}
