package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeReact runs `react <args...>` through the real command tree.
func executeReact(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newReactCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// `react apply` must require exactly one run id.
func TestReactApplyCmd_RequiresSingleRunID(t *testing.T) {
	for _, args := range [][]string{{"apply"}, {"apply", "1", "2"}} {
		_, err := executeReact(t, args...)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	}
}

// --force must reach runReactApply and suppress the confirmation prompt.
func TestReactApplyCmd_ForceFlagSkipsPrompt(t *testing.T) {
	seedReactReport(t, "909")
	stdinScript(t, "n\n")

	out, err := executeReact(t, "apply", "909", "--force")
	require.NoError(t, err)
	assert.NotContains(t, out, "Proceed with applying fix?")
	assert.Contains(t, out, "Delegate to debugger agent")
}

// --quiet must reach runReactCheck and suppress the no-failure notice.
func TestReactCheckCmd_QuietFlagSuppressesNotice(t *testing.T) {
	stubReactExec(t, ghInstalled, func(name string, _ ...string) ([]byte, error) {
		if name == "git" {
			return []byte("origin\n"), nil
		}
		return []byte("[]"), nil
	})

	quiet, err := executeReact(t, "check", "--quiet")
	require.NoError(t, err)
	assert.Empty(t, quiet)

	verbose, err := executeReact(t, "check")
	require.NoError(t, err)
	assert.Contains(t, verbose, "No recent CI failures found.")
}

// The verify command must reject the strict/disabled gate combination end to end.
func TestVerifyCmd_RejectsStrictWithDisabledGate(t *testing.T) {
	cmd := newVerifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"--strict-visual-gate", "--visual-gate=false"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--strict-visual-gate requires --visual-gate=true")
}
