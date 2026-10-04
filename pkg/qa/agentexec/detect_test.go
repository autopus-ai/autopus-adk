package agentexec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lookPathOf(found ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, f := range found {
			if f == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestDetectTarget_PrefersTheFirstInstalledCLI(t *testing.T) {
	t.Parallel()
	noEnv := func(string) string { return "" }
	got, err := DetectTarget(lookPathOf("codex", "opencode"), noEnv)
	require.NoError(t, err)
	assert.Equal(t, TargetCodex, got)

	got, err = DetectTarget(lookPathOf("agy"), noEnv)
	require.NoError(t, err)
	assert.Equal(t, TargetGemini, got)

	got, err = DetectTarget(lookPathOf("claude", "codex"), noEnv)
	require.NoError(t, err)
	assert.Equal(t, TargetClaude, got)
}

func TestDetectTarget_NoCLIIsASetupGap(t *testing.T) {
	t.Parallel()
	_, err := DetectTarget(lookPathOf(), func(string) string { return "" })
	var gap *SetupGapError
	require.ErrorAs(t, err, &gap)
	assert.Equal(t, CodeCLIMissing, gap.Code)
}

func TestDetectTarget_OverrideNeedsNoLookup(t *testing.T) {
	t.Parallel()
	got, err := DetectTarget(lookPathOf(), func(string) string { return `["my-agent"]` })
	require.NoError(t, err)
	assert.Equal(t, TargetClaude, got)
}
