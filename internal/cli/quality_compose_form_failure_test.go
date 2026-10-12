package cli

// SPEC-MODELUX-002 failure paths of the form flow: a form that cannot run
// leaves the update one warning line and lets it generate, a plan that
// cannot read the file warns and returns to the menu, and a plan without a
// write or a refresh without a historical preset writes nothing.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A form that fails to run, unlike Ctrl+C, does not end the update: the
// step leaves one warning line, and the update generates as before.
func TestUpdateComposeForms_FormFailureWarnsAndTheUpdateGenerates(t *testing.T) {
	run := newComposeUpdateRun(t, true, "")
	dir := run.settled(t, "R")
	originalProbe, originalRun := composeFormsProbe, composeRunForm
	t.Cleanup(func() { composeFormsProbe, composeRunForm = originalProbe, originalRun })
	composeFormsProbe = func(*cobra.Command) bool { return true }
	composeRunForm = func(*cobra.Command, *huh.Form) error { return errors.New("terminal gone") }
	before := composeSessionHash(t, dir)

	out, err := runComposeUpdate(t, formInput(), "--dir", dir)

	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, "  경고: terminal gone\n"), out)
	assert.Contains(t, out, "Update complete: 5 platform(s) updated")
	assert.Equal(t, before, composeSessionHash(t, dir))
}

// A 모든 도구 같게 plan that cannot read autopus.yaml for its rewrite notice
// warns once and returns to the menu; nothing is written.
func TestComposeForms_UnreadableFileAtThePlanWarnsAndWritesNothing(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	session := newComposeFormSession(t, dir)
	path := filepath.Join(dir, "autopus.yaml")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, out, err := runComposeForms(t, session, composeFormOptions{}, "2\n", "1\n", "\n", "3\n", "\n", "\n", "\n", "\n")

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(out, "  경고: read config: "), out)
	assert.NotContains(t, out, "적용할까요?")
	require.NoError(t, os.Chmod(path, 0o644))
	assert.Equal(t, "balanced", composeBindLoad(t, dir).Quality.Default)
}

// A plan without a write and a refresh without a historical preset each
// leave one warning line and write nothing.
func TestComposeFormRun_PlanWithoutAWriteAndRefreshWithoutHistoryWarn(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	before := composeSessionHash(t, dir)
	cmd, out := newComposeTestCommand()
	run := &composeFormRun{composeDriverState: newComposeFormSession(t, dir).start(cmd), cmd: cmd, form: composeFormTextKO}

	run.apply(composeDriverPlan{})
	require.NoError(t, run.refresh())

	assert.Equal(t, 2, strings.Count(out.String(), "  경고: "), out.String())
	assert.False(t, run.applied)
	assert.Equal(t, before, composeSessionHash(t, dir))
}
