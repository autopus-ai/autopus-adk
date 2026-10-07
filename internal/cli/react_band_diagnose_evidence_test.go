package cli

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

func writeBandReactReport(t *testing.T, projectDir string, runID int64, body string) string {
	t.Helper()
	dir := filepath.Join(projectDir, ".autopus", "react")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, strconv.FormatInt(runID, 10)+".md")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// FR-11 and REQ-17: the evidence of a CI claim is the failed-step log of
// each failed run attempt of its current block (1041 and 1042 in the O2
// fixture) and an existing react report, each sanitized; a failed fetch is
// skipped, never kept partly.
func TestReactBandDiagnose_EvidenceFetchesFailedRunLogsAndReactReports(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	require.Equal(t, []healthband.RunRef{{RunID: 1041, Attempt: 1}, {RunID: 1042, Attempt: 1}}, fixture.claim.FailedRuns)
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	runner.answers["gh run view 1041 -R acme/app --attempt 1 --log-failed"] = fakeBandAnswer{stdout: "half a log", err: errors.New("exit status 1")}
	runner.answers["gh run view 1042 -R acme/app --attempt 1 --log-failed"] = fakeBandAnswer{stdout: "using ghp_" + strings.Repeat("C", 36) + " for auth\nstep 4 failed\n"}
	writeBandReactReport(t, fixture.projectDir, 1041, "# CI Failure Report\n\nopen "+fixture.projectDir+"/.env failed\n")
	target := bandTestTarget
	source := bandRunEvidence{client: testBandClient(runner), target: &target, projectDir: fixture.projectDir}

	logs, reports := source.Evidence(context.Background(), fixture.claim)

	assert.Equal(t, []string{
		"gh run view 1041 -R acme/app --attempt 1 --log-failed",
		"gh run view 1042 -R acme/app --attempt 1 --log-failed",
	}, runner.argvs("gh"))
	require.Len(t, logs, 1)
	assert.Equal(t, int64(1042), logs[0].RunID)
	assert.Equal(t, 1, logs[0].Attempt)
	assert.Equal(t, "using [REDACTED_SECRET] for auth\nstep 4 failed", logs[0].Evidence.Text)
	assert.Equal(t, promptlayer.RedactionRedacted, logs[0].Evidence.RedactionStatus)
	require.Len(t, reports, 1)
	assert.Equal(t, int64(1041), reports[0].RunID)
	assert.Equal(t, "# CI Failure Report\n\nopen <project>/.env failed", reports[0].Evidence.Text)
}

// Without a resolved repository (--no-fetch or a skipped CI source) no gh
// call runs; a react report that is a symlink, or that cannot be opened, is
// never read.
func TestReactBandDiagnose_EvidenceWithoutRepositoryReadsOnlyRegularReports(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX symlinks and file modes that bind the user")
	}
	fixture := newBandDiagnoseFixture(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside the project\n"), 0o600))
	reactDir := filepath.Join(fixture.projectDir, ".autopus", "react")
	require.NoError(t, os.MkdirAll(reactDir, 0o755))
	require.NoError(t, os.Symlink(secret, filepath.Join(reactDir, "1041.md")))
	require.NoError(t, os.WriteFile(filepath.Join(reactDir, "1042.md"), []byte("unreadable\n"), 0o000))
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	source := bandRunEvidence{client: testBandClient(runner), projectDir: fixture.projectDir}

	logs, reports := source.Evidence(context.Background(), fixture.claim)

	assert.Empty(t, logs)
	assert.Empty(t, reports)
	assert.Empty(t, runner.recorded("gh"))
}

// The diagnoser feeds the gathered evidence to the prompt and the BS: the
// prompt manifest names the fetched log and the report, and the BS fences
// both.
func TestReactBandDiagnose_EvidenceReachesThePromptAndTheBS(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	runner := scriptedBandRunner("git@github.com:acme/app.git", "main", "[]")
	runner.answers["gh run view"] = fakeBandAnswer{stdout: "step 4 failed\n"}
	writeBandReactReport(t, fixture.projectDir, 1042, "# CI Failure Report\n")
	target := bandTestTarget
	d := fixture.diagnoser(bandHarness("claude", "", map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()}),
		bandRunEvidence{client: testBandClient(runner), target: &target, projectDir: fixture.projectDir})
	var prompt string
	d.run = func(_ context.Context, _ orchestra.OrchestraConfig, _ orchestra.ProviderConfig, sent string) (*orchestra.ProviderResponse, error) {
		prompt = sent
		return &orchestra.ProviderResponse{Output: "diagnosis"}, nil
	}

	outcome := d.Run(context.Background(), fixture.claim)

	var ids []string
	for _, entry := range outcome.PromptManifest[2:] {
		ids = append(ids, entry.ID)
	}
	// promptlayer.Render orders the ephemeral layers by ID.
	assert.Equal(t, []string{"band.evidence.react.1042", "band.evidence.run.1041.a1", "band.evidence.run.1042.a1"}, ids)
	assert.Contains(t, prompt, "Existing react report excerpt for CI run 1042:")
	bs := fixture.bs(t, outcome.BSID)
	assert.Contains(t, bs, "### Evidence: CI run 1042 attempt 1\n")
	assert.Contains(t, bs, "### Evidence: react report for CI run 1042\n")
}

// S12: no band source imports pkg/qa/agentexec, whose generate argv carries
// no read-only flags.
func TestReactBandDiagnose_NoBandSourceImportsAgentexec(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("react_band*.go")
	require.NoError(t, err)
	core, err := filepath.Glob(filepath.Join("..", "..", "pkg", "healthband", "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, core)
	files = append(files, core...)
	require.Contains(t, files, "react_band_diagnose.go")
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		require.NoError(t, err, file)
		for _, spec := range parsed.Imports {
			assert.NotEqual(t, `"github.com/insajin/autopus-adk/pkg/qa/agentexec"`, spec.Path.Value, file)
		}
	}
}
