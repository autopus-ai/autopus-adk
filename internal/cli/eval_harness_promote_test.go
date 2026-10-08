package cli

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/insajin/autopus-adk/pkg/harneval/intake"
	"github.com/insajin/autopus-adk/pkg/secretscan"
)

// The S6 candidate of fingerprint Y and the files its promotion writes.
const (
	promoteCandidateID = "GTC-023e9302ff0b"
	promoteCandidate   = "evals/harness/candidates/" + promoteCandidateID + ".json"
	promoteTask        = "evals/harness/tasks/surface/GT-INC-023E9302.json"
	promoteLink        = "evals/harness/candidates/promoted/GT-INC-023E9302.json"
)

// promoteCodexAdapter is the fake surface adapter under the codex name, so
// the S6 assertion reads .codex/hooks.json from the codex platform.
type promoteCodexAdapter struct{ harnessFakeAdapter }

func (promoteCodexAdapter) Name() string { return "codex" }

// promoteHarnessDeps is harnessDeps with a claude-code surface holding the
// router and a codex surface holding .codex/hooks.json.
func promoteHarnessDeps() evalHarnessDeps {
	deps := harnessDeps()
	deps.run.Adapters = func(root string, _ harneval.Pins, _ []byte) []adapter.PlatformAdapter {
		return []adapter.PlatformAdapter{
			harnessFakeAdapter{root: root, files: []string{harnessRouter}},
			promoteCodexAdapter{harnessFakeAdapter{root: root, files: []string{".codex/hooks.json"}}},
		}
	}
	return deps
}

// writePromoteCandidate lets intake create the candidate of the Y entry
// L-002 under tree, through the production redactor.
func writePromoteCandidate(t *testing.T, tree *harnessTree) {
	t.Helper()
	entry := intake.Entry{
		ID: "L-002", Type: "fix_pattern", Pattern: "hook missing in codex",
		Files:    []string{"pkg/content/a.go", "pkg/content/hooks.go", "pkg/content/z.go"},
		Packages: []string{"pkg/adapter", "pkg/content"},
		Expected: "y", Actual: "ya", Repro: "auto init",
	}
	redactor := intake.RedactorFunc(func(text string) string { masked, _ := secretscan.Redact(text); return masked })
	result, err := intake.Run(intake.Request{Root: tree.root, Entries: []intake.Entry{entry}, LearningIDs: []string{"L-002"}, Redactor: redactor})
	require.NoError(t, err)
	require.Equal(t, promoteCandidateID, result.Rows[0].CandidateID)
}

// editPromoteCandidate decodes the candidate, applies edit, and writes it back.
func editPromoteCandidate(t *testing.T, tree *harnessTree, edit func(*intake.Candidate)) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tree.root, filepath.FromSlash(promoteCandidate)))
	require.NoError(t, err)
	var candidate intake.Candidate
	require.NoError(t, json.Unmarshal(data, &candidate))
	edit(&candidate)
	tree.writeJSON(promoteCandidate, candidate)
}

// completePromoteDraft writes the S6 category and assertion into the draft.
func completePromoteDraft(t *testing.T, tree *harnessTree) {
	t.Helper()
	editPromoteCandidate(t, tree, func(c *intake.Candidate) {
		c.Task.Category = "hooks_settings"
		c.Task.Assertions = []harneval.Assertion{{Kind: harneval.AssertFileExists, Platform: "codex", Path: ".codex/hooks.json"}}
	})
}

// promoteFileSHA returns the SHA-256 hex of the file at rel below tree.
func promoteFileSHA(t *testing.T, tree *harnessTree, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tree.root, filepath.FromSlash(rel)))
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestEvalHarnessPromote_S6Flow_DraftPromoteRunAndBaseline is S6 at the CLI.
// Not parallel: promote and run generate surfaces, which swaps PATH and HOME.
func TestEvalHarnessPromote_S6Flow_DraftPromoteRunAndBaseline(t *testing.T) {
	// Given a golden set pinned by its baseline and the intake candidate.
	tree := standardHarnessTree(t)
	deps := promoteHarnessDeps()
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--init", "--dir", tree.root).code)
	writePromoteCandidate(t, tree)
	promote := func() harnessOutcome {
		return runHarness(t, deps, "promote", promoteCandidateID, "--format", "json", "--dir", tree.root)
	}

	// When the untouched draft and the category-only draft are promoted,
	// then each is refused with its first gap and the candidate is kept.
	for _, step := range []struct{ detail, category string }{{"category", ""}, {"assertions", "hooks_settings"}} {
		editPromoteCandidate(t, tree, func(c *intake.Candidate) { c.Task.Category = step.category })
		before := promoteFileSHA(t, tree, promoteCandidate)
		got := promote()
		assert.Equal(t, 1, got.code)
		assert.Empty(t, got.stdout)
		assert.Contains(t, got.stderr, "draft_incomplete: "+step.detail)
		assert.Equal(t, before, promoteFileSHA(t, tree, promoteCandidate))
	}

	// When the S6 assertion is added and the candidate is promoted.
	completePromoteDraft(t, tree)
	got := promote()

	// Then the promote document alone is on stdout and the guidance on stderr.
	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, map[string]any{
		"schema_version": "harness_promote_result.v1", "result": "promoted",
		"candidate_id": promoteCandidateID, "task_id": "GT-INC-023E9302",
		"task_path": promoteTask, "link_path": promoteLink, "current_outcome": "pass",
	}, harnessDoc(t, got.stdout))
	assert.Contains(t, got.stderr, "auto eval harness baseline --update")
	assert.FileExists(t, filepath.Join(tree.root, filepath.FromSlash(promoteTask)))
	assert.FileExists(t, filepath.Join(tree.root, filepath.FromSlash(promoteLink)))
	assert.NoFileExists(t, filepath.Join(tree.root, filepath.FromSlash(promoteCandidate)))

	// And the next run shows the task as new until the baseline pins it.
	run := runHarness(t, deps, "run", "--format", "json", "--dir", tree.root)
	assert.Equal(t, 1, run.code)
	doc := harnessDoc(t, run.stdout)
	assert.Equal(t, []any{"set_digest_mismatch"}, doc["failure_reasons"])
	assert.Contains(t, doc["transitions"], map[string]any{"task_id": "GT-INC-023E9302", "kind": "new"})
	require.Equal(t, 0, runHarness(t, deps, "baseline", "--update", "--dir", tree.root).code)
	run = runHarness(t, deps, "run", "--format", "json", "--dir", tree.root)
	assert.Equal(t, 0, run.code, run.stderr)
	assert.Equal(t, "pass", harnessDoc(t, run.stdout)["status"])

	// And a rerun finds no candidate and changes nothing.
	task := promoteFileSHA(t, tree, promoteTask)
	again := promote()
	assert.Equal(t, 1, again.code)
	assert.Empty(t, again.stdout)
	assert.Contains(t, again.stderr, "candidate_missing")
	assert.Equal(t, task, promoteFileSHA(t, tree, promoteTask))
}

func TestEvalHarnessPromote_S6InvocationRefusals_ExitOneBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		args []string
		want string
	}{
		"relative escape":     {[]string{"promote", "../x"}, "candidate_id_invalid"},
		"upper-case hex":      {[]string{"promote", "GTC-023E9302FF0B"}, "candidate_id_invalid"},
		"path in the id":      {[]string{"promote", "GTC-023e9302ff0b/../x"}, "candidate_id_invalid"},
		"no automatic --all":  {[]string{"promote", promoteCandidateID, "--all"}, "unknown flag: --all"},
		"no automatic --auto": {[]string{"promote", promoteCandidateID, "--auto"}, "unknown flag: --auto"},
		"exactly one id":      {[]string{"promote", promoteCandidateID, "GTC-8e80c7a18029"}, "accepts 1 arg(s)"},
		"json only":           {[]string{"promote", promoteCandidateID, "--format", "text"}, `unsupported --format "text"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tree := standardHarnessTree(t)
			writePromoteCandidate(t, tree)
			before := promoteFileSHA(t, tree, promoteCandidate)

			got := runHarness(t, evalHarnessDeps{}, append(tc.args, "--dir", tree.root)...)

			assert.Equal(t, 1, got.code)
			assert.Empty(t, got.stdout)
			assert.Contains(t, got.stderr, tc.want)
			assert.Equal(t, before, promoteFileSHA(t, tree, promoteCandidate))
		})
	}
}

func TestEvalHarnessPromote_RefusalText_IsRedactedAndPrintable(t *testing.T) {
	t.Parallel()
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	secret := hex.EncodeToString(raw[:])
	cases := map[string]struct {
		seed           func(t *testing.T, tree *harnessTree)
		reason, absent string
	}{
		"secret in a candidate field name": {func(t *testing.T, tree *harnessTree) {
			body, err := os.ReadFile(filepath.Join(tree.root, filepath.FromSlash(promoteCandidate)))
			require.NoError(t, err)
			tree.write(promoteCandidate, strings.Replace(string(body), `"redacted": false,`, `"redacted": false, "token=`+secret+`": 1,`, 1))
		}, "candidate_invalid: decode", secret},
		"control character in a broken task file name": {func(t *testing.T, tree *harnessTree) {
			completePromoteDraft(t, tree)
			task := harnessSurfaceTask("GT-FIX-D", harnessRouter)
			task["extra"] = 1
			tree.writeJSON("evals/harness/tasks/surface/x\x1b[31my.json", task)
		}, "active_set_invalid: unknown_field", "\x1b"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Given a candidate and untrusted text the refusal would quote.
			tree := standardHarnessTree(t)
			writePromoteCandidate(t, tree)
			tc.seed(t, tree)

			// When the candidate is promoted.
			got := runHarness(t, evalHarnessDeps{}, "promote", promoteCandidateID, "--dir", tree.root)

			// Then the refusal names the defect without echoing the text.
			assert.Equal(t, 1, got.code)
			assert.Contains(t, got.stderr, tc.reason)
			assert.NotContains(t, got.stderr, tc.absent)
		})
	}
}

func TestEvalHarnessPromote_Guidance_NamesResumeAndNotEvaluatedReason(t *testing.T) {
	t.Parallel()
	var out strings.Builder

	writePromoteGuidance(&out, intake.PromoteResult{
		Result: intake.PromoteResultAlreadyActive, CandidateID: promoteCandidateID,
		TaskPath: promoteTask, LinkPath: promoteLink,
		CurrentOutcome: intake.OutcomeNotEvaluated, NotEvaluatedReason: "templates_stale",
	})

	assert.Equal(t, "harness-eval: finished the interrupted promotion of GTC-023e9302ff0b to "+
		"evals/harness/tasks/surface/GT-INC-023E9302.json with link record "+
		"evals/harness/candidates/promoted/GT-INC-023E9302.json; current_outcome not_evaluated (templates_stale)\n"+
		`harness-eval: pin the new task with "auto eval harness baseline --update"; `+
		`until then "auto eval harness run" reports it as new with set_digest_mismatch`+"\n", out.String())
}

func TestEvalHarnessPromote_MissingProjectRoot_IsAPlainError(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "absent")

	got := runHarness(t, evalHarnessDeps{}, "promote", promoteCandidateID, "--dir", missing)

	assert.Equal(t, 1, got.code)
	assert.Empty(t, got.stdout)
	assert.Contains(t, got.stderr, "Error: harness promote: open project root")
	assert.NoDirExists(t, missing)
}
