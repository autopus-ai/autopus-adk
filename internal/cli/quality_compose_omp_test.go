package cli

// SPEC-MODELUX-002 T6 (REQ-MUX2-04, REQ-MUX2-10, REQ-MUX2-11; acceptance S3,
// S8, S9): the OMP tab asks what the OMP editor asked, previews the plan's
// real routes, writes only through applyOMPProfile, and syncs the loaded
// config from the written file.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeOMPActivations records every activation an OMP write asks for.
type composeOMPActivations struct {
	calls int
	cfgs  []*config.HarnessConfig
}

// composeOMPDeps answers catalog for every OMP read and records activations.
func composeOMPDeps(catalog []byte) (ompPlatformDependencies, *composeOMPActivations) {
	activations := &composeOMPActivations{}
	runner := &ompCLIFakeRunner{catalog: catalog}
	return ompPlatformDependencies{
		newRunner: func() omp.OMPModelCatalogRunner { return runner },
		activate: func(_ context.Context, _ string, cfg *config.HarnessConfig) error {
			activations.calls++
			activations.cfgs = append(activations.cfgs, cfg)
			return nil
		},
	}, activations
}

// composeFormS8 answers S8 in `auto quality` over O3 up to its confirmation:
// 구성 편집, OMP, 에이전트별 지정, GPT, task, openai-codex/gpt-6-astra, max,
// 완료.
var composeFormS8 = []string{"2\n", "4\n", "4\n", "1\n", "5\n", "3\n", "2\n", "\n"}

const composeOMPHeader = "OMP (요청 route · catalog 확인 전)"

func runComposeOMPForms(t *testing.T, dir string, deps ompPlatformDependencies, lines ...string) (string, error) {
	t.Helper()
	options := composeFormOptions{Presets: true, OMP: &composeOMPTab{deps: deps}}
	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), options, lines...)
	return out, err
}

// S8: the pin goes through applyOMPProfile once, the summary shows the
// plan's route, the quality block keeps its values, and the table drawn
// next already reads the synced role_model_policy.
func TestComposeOMPTab_PinAppliesThroughTheOMPWriterAndSyncs(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex", "omp")
	before := readBlockFixture(t, dir)
	deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())

	out, err := runComposeOMPForms(t, dir, deps, composeFormLines(composeFormS8, "2\n", "\n")...)

	require.NoError(t, err)
	assert.Contains(t, out, "어느 도구를 바꿀까요? \n1. 모든 도구 같게\n2. Claude Code\n3. Codex\n4. OMP\n")
	assert.Contains(t, out, "OMP balanced — model preview (no changes yet)\n")
	assert.Regexp(t, `(?m)^task\*\s+openai-codex/gpt-6-astra\s+max$`, out)
	assert.Contains(t, out, composeFormTextKO.OMPRewrite+"\n적용할까요? \n1. 취소\n2. 적용\n")
	assert.Contains(t, out, "OMP models applied. Start a new OMP session to use them.\n")
	assert.Equal(t, 1, activations.calls)
	after := readBlockFixture(t, dir)
	policy := rawRoleModelPolicyOf(t, after)
	assert.Equal(t, "balanced", policy.Profile)
	assert.Equal(t, "openai", policy.Family)
	require.Len(t, policy.Agents, 1)
	require.Len(t, policy.Agents["task"].Candidates, 1)
	candidate := policy.Agents["task"].Candidates[0]
	assert.Equal(t, []string{"openai-codex/gpt-6-astra", "max", "openai"},
		[]string{candidate.Selector, candidate.Thinking, candidate.Family})
	assert.Equal(t, rawQualityOf(t, before), rawQualityOf(t, after))
	redrawn := out[strings.LastIndex(out, composeViewStatusD):]
	assert.Contains(t, composeTableHeader(t, redrawn, composeViewStatusD), composeOMPHeader)
	assert.Equal(t, "task · 6-astra:max", composeRenderedTable(t, redrawn, composeViewStatusD)["executor"][composeOMPHeader])
}

// S8: a failed re-read after the write prints the line with its cause and
// ends the step with the reload error; the file keeps the applied policy.
func TestComposeOMPTab_FailedReReadEndsTheStep(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex", "omp")
	deps, _ := composeOMPDeps(ompCLIBalancedCatalogJSON())
	original := composeReloadConfig
	t.Cleanup(func() { composeReloadConfig = original })
	composeReloadConfig = func(string) (*config.HarnessConfig, error) { return nil, errors.New("disk gone") }

	out, err := runComposeOMPForms(t, dir, deps, composeFormLines(composeFormS8, "2\n", "\n")...)

	var reload composeReloadError
	require.ErrorAs(t, err, &reload)
	assert.Contains(t, out, "OMP 설정은 적용됐지만 다시 읽지 못했습니다: disk gone\n")
	assert.Equal(t, "balanced", rawRoleModelPolicyOf(t, readBlockFixture(t, dir)).Profile)
}

// S9: a blocker, a placeholder, a cancelled confirmation, and a pin-less
// selection each leave the file and OMP untouched.
func TestComposeOMPTab_RefusalsAndCancelWriteNothing(t *testing.T) {
	cases := []struct {
		name    string
		catalog func(*testing.T) []byte
		dir     func(*testing.T) string
		lines   []string
		want    []string
	}{
		{"blocker", func(t *testing.T) []byte { return ompCLIBalancedCatalogWithout(t, "openai-codex/gpt-6-astra") },
			composeO3, []string{"2\n", "4\n", "2\n", "1\n", "\n"}, []string{"omp_profile_candidate_unavailable", "task"}},
		{"placeholder", func(*testing.T) []byte { return ompCLIBalancedCatalogJSON() },
			writeComposeO4, []string{"2\n", "4\n", "2\n", "1\n", "\n"}, []string{"role_model_policy_placeholder_unsupported"}},
		{"cancel", func(*testing.T) []byte { return ompCLIBalancedCatalogJSON() },
			composeO3, composeFormLines(composeFormS8, "\n", "\n"), []string{"취소했습니다. 설정을 바꾸지 않았습니다.\n"}},
		{"no pin", func(*testing.T) []byte { return ompCLIBalancedCatalogJSON() },
			composeO3, []string{"2\n", "4\n", "4\n", "1\n", "\n", "\n"}, []string{"No agent was pinned; nothing changed.\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composeSessionCatalog(t, "CAT-A")
			dir := tc.dir(t)
			before := composeSessionHash(t, dir)
			deps, activations := composeOMPDeps(tc.catalog(t))

			out, err := runComposeOMPForms(t, dir, deps, tc.lines...)

			require.NoError(t, err)
			for _, want := range tc.want {
				assert.Contains(t, out, want)
			}
			if tc.name != "cancel" {
				assert.NotContains(t, out, "적용할까요?")
			}
			assert.Zero(t, activations.calls)
			assert.Equal(t, before, composeSessionHash(t, dir))
		})
	}
}

// S3: update shows OMP only for a configured omp; one it only detected
// gets the notice once, however often the tab opens.
func TestComposeOMPTab_DetectedOnlyOMPGetsTheNoticeOnce(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	session := newComposeFormSession(t, dir).withDetected([]string{"opencode", "omp"})
	deps, _ := composeOMPDeps(ompCLIBalancedCatalogJSON())

	_, out, err := runComposeForms(t, session, composeFormOptions{OMP: &composeOMPTab{deps: deps}},
		"2\n", "\n", "\n", "\n", "\n", "\n", "\n", "2\n", "\n", "\n", "\n", "\n", "\n", "\n", "\n")

	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out, composeFormTabD), out)
	assert.Equal(t, 1, strings.Count(out, composeFormTextKO.OMPDetectedOnly+"\n"))
	assert.NotRegexp(t, `(?m)^\d+\. (OMP|OpenCode)$`, out)
}

func composeO3(t *testing.T) string {
	t.Helper()
	return writeComposeFixture(t, "claude-code", "codex", "omp")
}

// writeComposeO4 is O3 with a role_model_policy whose family line reads
// ${OMP_FAMILY}, set to openai: the loader reads openai, and the OMP source
// check refuses the placeholder.
func writeComposeO4(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := composeFixtureConfig("claude-code", "codex", "omp")
	cfg.RoleModelPolicy = config.RoleModelPolicyConf{Version: config.RoleModelPolicyVersionV1, Profile: "balanced", Family: "openai"}
	require.NoError(t, config.Save(dir, cfg))
	rewriteComposeLine(t, dir+"/autopus.yaml", "    family: openai", "    family: ${OMP_FAMILY}")
	t.Setenv("OMP_FAMILY", "openai")
	loaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	require.Equal(t, "openai", loaded.RoleModelPolicy.Family)
	return dir
}
