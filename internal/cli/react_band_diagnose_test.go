package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// S12 claude-ok row and S19: the judge runs once with the shared read-only
// projection, in the project dir, on the rendered layer prompt; its output
// is fenced in the one BS the claim writes, and the outcome carries the
// prompt manifest without content.
func TestReactBandDiagnose_ClaudeRunsProjectedReadOnlyArgvAndWritesBS(t *testing.T) {
	fakes := installBandFakeProviders(t)
	fixture := newBandDiagnoseFixture(t)
	harness := bandHarness("claude", "", map[string]config.ProviderEntry{"claude": config.DefaultClaudeProviderEntry()})

	outcome := fixture.diagnoser(harness, bandS19Evidence(fixture.projectDir)).Run(context.Background(), fixture.claim)

	assert.Empty(t, outcome.Status, "an executed diagnose claim is done")
	assert.Equal(t, "ok", outcome.DiagnosisStatus)
	assert.Equal(t, "BS-BAND-001", outcome.BSID)
	assert.Equal(t, "written", outcome.BSStatus)
	assert.Equal(t, []string{"claude"}, fakes.calls(t), "exactly one provider call")

	argv := fakes.argv(t, "claude")
	assert.Equal(t, "--tools=Read,Grep,Glob", argv[len(argv)-1], "the inline tools item stays last")
	assert.Contains(t, strings.Join(argv, " "), "--permission-mode plan")
	assert.Subset(t, argv, []string{"--safe-mode", "--no-session-persistence", "--disable-slash-commands", "--strict-mcp-config"})
	project, err := filepath.EvalSymlinks(fixture.projectDir)
	require.NoError(t, err)
	assert.Equal(t, project, strings.TrimSpace(fakes.record(t, "claude", "cwd")))

	prompt := fakes.record(t, "claude", "stdin")
	assert.Contains(t, prompt, "read-only diagnosis agent")
	assert.Contains(t, prompt, "Failed-step log excerpt of CI run 4242 attempt 1")
	assert.Contains(t, prompt, "using [REDACTED_SECRET] for auth")
	for _, forbidden := range []string{"ghp_", "BS-BAND"} {
		assert.NotContains(t, prompt, forbidden)
	}

	hash, err := healthband.EventHash(fixture.claim.Event)
	require.NoError(t, err)
	var ids []string
	for _, entry := range outcome.PromptManifest {
		ids = append(ids, entry.ID+" "+string(entry.Kind)+" "+entry.RedactionStatus)
	}
	assert.Equal(t, []string{
		"band.instructions.v1 stable passed", "band.evaluation." + hash + " snapshot passed",
		"band.evidence.run.4242.a1 ephemeral redacted", "band.evidence.run.4243.a2 ephemeral passed",
	}, ids)
	assert.Equal(t, promptlayer.KindStable, outcome.PromptManifest[0].Kind)
	assert.True(t, outcome.PromptManifest[0].CacheEligible)

	assert.Equal(t, []string{"BS-BAND-001.md"}, fixture.bsFiles(t))
	bs := fixture.bs(t, "BS-BAND-001")
	assert.True(t, strings.HasPrefix(bs, "# BS-BAND-001: ci.failure_rate:CI tier 2 anomaly (e1042)\n"))
	assert.Contains(t, bs, "**Providers**: claude\n")
	assert.Contains(t, bs, "diagnosis_status: ok\n")
	assert.Contains(t, bs, healthband.Fence("### Summary\nThe flaky step failed."))
}

// The production backends register the OMP route (an unregistered route
// would be provider_backend_unavailable), and that route reports a missing
// omp executable as provider_missing. PATH is empty, so no session starts.
func TestReactBandDiagnose_ProductionOMPRouteWithoutOMPIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	fixture := newBandDiagnoseFixture(t)
	harness := bandHarness("claude", "", map[string]config.ProviderEntry{"claude": bandOMPClaude()})

	outcome := fixture.diagnoser(harness, nil).Run(context.Background(), fixture.claim)

	assert.Equal(t, "unavailable(provider_missing)", outcome.DiagnosisStatus)
	assert.Equal(t, "written", outcome.BSStatus)
}
