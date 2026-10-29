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

func runComposeOMPForms(t *testing.T, dir string, deps ompPlatformDependencies, lines ...string) (string, error) {
	t.Helper()
	options := composeFormOptions{Presets: true, OMP: &composeOMPTab{deps: deps}}
	_, out, err := runComposeForms(t, newComposeFormSession(t, dir), options, lines...)
	return out, err
}

// S8: a failed re-read after the write prints the line with its cause and
// ends the step with the reload error; the file keeps the applied policy.
// The pins are the role groups of S10.
func TestComposeOMPTab_FailedReReadEndsTheStep(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex", "omp")
	deps, activations := composeOMPDeps([]byte(composeOMPCatalogML2))
	original := composeReloadConfig
	t.Cleanup(func() { composeReloadConfig = original })
	composeReloadConfig = func(string) (*config.HarnessConfig, error) { return nil, errors.New("disk gone") }

	out, err := runComposeOMPForms(t, dir, deps, "2\n", "4\n", "1\n", "3\n", "1\n", "6\n", "2\n", "\n", "2\n", "2\n", "\n")

	var reload composeReloadError
	require.ErrorAs(t, err, &reload)
	assert.Equal(t, "OMP settings were applied but could not be re-read: disk gone", err.Error())
	assert.Contains(t, out, "OMP 설정은 적용됐지만 다시 읽지 못했습니다: disk gone\n")
	assert.Equal(t, 1, activations.calls)
	assert.Equal(t, composeOMPS10Policy(), rawRoleModelPolicyOf(t, readBlockFixture(t, dir)))
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
