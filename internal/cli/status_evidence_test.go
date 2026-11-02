package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkVerification asserts one SPEC's label, verification state, and reasons.
func checkVerification(t *testing.T, f *statusFixture, specID, label, state string, reasons ...string) {
	t.Helper()
	evidence := f.evidence(specID)
	assert.Equal(t, label, evidence.Label)
	assert.Equal(t, state, evidence.Verification.State)
	assert.Equal(t, reasons, evidence.Verification.Reasons)
}

// S2: verification stays fresh when the verified tree is committed or only a
// status line moves, and goes stale when code or requirements change.
func TestStatusEvidence_VerificationGoesStaleWhenCodeChanges(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-VER-001"
	f := newStatusFixture(t, "claude-code")
	f.writeSpec(specID, "approved")
	f.commit("baseline")
	f.write("pkg/app/app.go", "package app\n\n// implemented\n")
	result := f.completedRun(specID)
	f.recordBaseline(specID, result)

	checkVerification(t, f, specID, "verified", "fresh")
	f.commit("commit the verified tree")
	checkVerification(t, f, specID, "verified", "fresh")

	f.write("pkg/app/app.go", "package app\n\n// edited after verification\n")
	checkVerification(t, f, specID, "stale", "stale", "working tree changed since verification: pkg/app/app.go")
	runGitCommand(t, f.root, "checkout", "--", "pkg/app/app.go")
	checkVerification(t, f, specID, "verified", "fresh")

	// Review promotion and sync rewrite the status line; that is no change.
	spec := f.read(".autopus/specs/" + specID + "/spec.md")
	f.write(".autopus/specs/"+specID+"/spec.md", strings.Replace(spec, "**Status**: approved", "**Status**: completed", 1))
	checkVerification(t, f, specID, "verified", "fresh")
	acceptance := f.read(".autopus/specs/" + specID + "/acceptance.md")
	f.write(".autopus/specs/"+specID+"/acceptance.md", acceptance+"And errors are reported.\n")
	checkVerification(t, f, specID, "stale", "stale", "SPEC documents changed since verification")
	f.write(".autopus/specs/"+specID+"/acceptance.md", acceptance)

	removeIfExists(t, pipelineVerificationPath(filepath.Join(f.root, pipelineStateDir), specID))
	checkVerification(t, f, specID, "unverified", "unknown", "no verification baseline")
	assert.Contains(t, f.runStatus(), "verification: unknown (run "+result.Receipt.RunID+
		", base HEAD none, recorded never) — no verification baseline")
}

// A change git would not report — a file marked assume-unchanged or
// skip-worktree — makes verification unknown, never fresh; an executable-bit
// change counts even with core.fileMode off.
func TestStatusEvidence_HiddenChangesAreNeverFresh(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-HIDDEN-001"
	f := newStatusFixture(t, "claude-code")
	f.writeSpec(specID, "approved")
	f.write("pkg/app/other.go", "package app\n")
	f.commit("baseline")
	f.recordBaseline(specID, f.completedRun(specID))
	checkVerification(t, f, specID, "verified", "fresh")

	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		runGitCommand(t, f.root, "update-index", flag, "pkg/app/other.go")
		f.write("pkg/app/other.go", "package app\n\n// hidden from git diff\n")
		evidence := f.evidence(specID)
		assert.Equal(t, "unverified", evidence.Label, flag)
		require.Len(t, evidence.Verification.Reasons, 1, flag)
		assert.Contains(t, evidence.Verification.Reasons[0], "git hides changes to tracked files: "+
			"1 marked assume-unchanged or skip-worktree: pkg/app/other.go", flag)
		runGitCommand(t, f.root, "update-index", "--no"+strings.TrimPrefix(flag, "-"), "pkg/app/other.go")
		runGitCommand(t, f.root, "checkout", "--", "pkg/app/other.go")
		checkVerification(t, f, specID, "verified", "fresh")
	}
	if runtime.GOOS != "windows" {
		runGitCommand(t, f.root, "config", "core.fileMode", "false")
		require.NoError(t, os.Chmod(filepath.Join(f.root, "pkg/app/other.go"), 0o755))
		checkVerification(t, f, specID, "stale", "stale", "working tree changed since verification: pkg/app/other.go")
	}
}

// A baseline that parses but cannot vouch for anything — a missing or
// malformed field, another SPEC, or tree paths that do not add up to the
// tree fingerprint — is unknown, never fresh.
func TestStatusEvidence_InvalidBaselineIsNeverFresh(t *testing.T) {
	t.Parallel()
	const specID = "SPEC-BASELINE-001"
	f := newStatusFixture(t, "claude-code")
	f.writeSpec(specID, "approved")
	f.commit("baseline")
	f.write("pkg/app/app.go", "package app\n\n// implemented\n")
	f.recordBaseline(specID, f.completedRun(specID))
	// The tree returns to base, so a baseline without tree_paths would match.
	runGitCommand(t, f.root, "checkout", "--", "pkg/app/app.go")
	path := pipelineVerificationPath(filepath.Join(f.root, pipelineStateDir), specID)
	valid, err := os.ReadFile(path)
	require.NoError(t, err)

	cases := map[string]struct {
		mutate func(map[string]any)
		reason string
	}{
		"tree_paths missing": {func(m map[string]any) { delete(m, "tree_paths") }, "tree_paths is missing"},
		"tree_paths null":    {func(m map[string]any) { m["tree_paths"] = nil }, "tree_paths is missing"},
		"aggregate mismatch": {func(m map[string]any) { m["tree_fingerprint"] = "sha256:" + strings.Repeat("0", 64) },
			"tree_fingerprint does not match tree_paths"},
		"malformed entry": {func(m map[string]any) { m["tree_paths"].(map[string]any)["x.go"] = "sha256:short" },
			`tree_paths entry "x.go" is malformed`},
		"base_head is a name":  {func(m map[string]any) { m["base_head"] = "HEAD" }, "base_head is not a commit id"},
		"another SPEC":         {func(m map[string]any) { m["spec_id"] = "SPEC-OTHER-001" }, `it names "SPEC-OTHER-001"`},
		"recorded_at missing":  {func(m map[string]any) { delete(m, "recorded_at") }, "recorded_at is missing"},
		"unrecognized schema":  {func(m map[string]any) { m["schema"] = "pipeline_verification_baseline.v9" }, `unrecognized schema "pipeline_verification_baseline.v9"`},
		"spec fingerprint bad": {func(m map[string]any) { m["spec_fingerprint"] = "md5:x" }, "spec_fingerprint is not a sha256 digest"},
	}
	for name, tc := range cases {
		var baseline map[string]any
		require.NoError(t, json.Unmarshal(valid, &baseline))
		tc.mutate(baseline)
		data, err := json.Marshal(baseline)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		evidence := f.evidence(specID)
		assert.Equal(t, "unverified", evidence.Label, name)
		assert.Equal(t, []string{"verification baseline is invalid: " + tc.reason}, evidence.Verification.Reasons, name)
	}
}

// A stopped or unfinished last run outranks older verification, a dry run is
// not execution evidence, and only fresh verification yields verified.
func TestStatusEvidence_LabelPrecedence(t *testing.T) {
	t.Parallel()
	fresh, none := statusVerification{State: "fresh"}, statusVerification{State: "none"}
	cases := []struct {
		name         string
		declaredDone bool
		execution    *statusExecution
		verification statusVerification
		want         string
	}{
		{"unfinished outranks fresh", true, &statusExecution{}, fresh, "unfinished"},
		{"stopped outranks fresh", true, &statusExecution{Terminal: "failed_budget"}, fresh, "failed"},
		{"completed and fresh", false, &statusExecution{Terminal: "completed"}, fresh, "verified"},
		{"dry run is no evidence", true, &statusExecution{Terminal: "dry_run"}, none, "documented_only"},
		{"unreadable checkpoint is no evidence", false, &statusExecution{Error: "checkpoint unreadable"}, none, "none"},
		{"unknown verification", true, nil, statusVerification{State: "unknown"}, "unverified"},
		{"stale verification", true, nil, statusVerification{State: "stale"}, "stale"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, evidenceLabel(tc.declaredDone, tc.execution, tc.verification), tc.name)
	}
}
