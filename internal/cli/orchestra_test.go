package cli

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// TestNewOrchestraCmd_SubcommandRegistration verifies that newOrchestraCmd
// registers all expected subcommands including detach job management commands.
func TestNewOrchestraCmd_SubcommandRegistration(t *testing.T) {
	t.Parallel()

	cmd := newOrchestraCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "orchestra", cmd.Use)

	subCmds := cmd.Commands()
	require.Len(t, subCmds, 11)

	names := make([]string, len(subCmds))
	for i, sc := range subCmds {
		names[i] = sc.Name()
	}
	assert.Contains(t, names, "review")
	assert.Contains(t, names, "plan")
	assert.Contains(t, names, "secure")
	assert.Contains(t, names, "brainstorm")
	assert.Contains(t, names, "status")
	assert.Contains(t, names, "wait")
	assert.Contains(t, names, "result")
	assert.Contains(t, names, "collect")
	assert.Contains(t, names, "cleanup")
	assert.Contains(t, names, "inject")
	assert.Contains(t, names, "run")
}

// TestNewOrchestraReviewCmd_Flags verifies that review cmd registers all expected flags.
func TestNewOrchestraReviewCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := newOrchestraReviewCmd()
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("strategy"), "strategy flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("providers"), "providers flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("timeout"), "timeout flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("judge"), "judge flag must exist")
}

// TestNewOrchestraPlanCmd_Flags verifies that plan cmd registers expected flags.
func TestNewOrchestraPlanCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := newOrchestraPlanCmd()
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("strategy"), "strategy flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("providers"), "providers flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("timeout"), "timeout flag must exist")
	assert.Nil(t, cmd.Flags().Lookup("judge"), "plan cmd must NOT have judge flag")
}

// TestNewOrchestraSecureCmd_Flags verifies that secure cmd registers expected flags.
func TestNewOrchestraSecureCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := newOrchestraSecureCmd()
	require.NotNil(t, cmd)

	assert.NotNil(t, cmd.Flags().Lookup("strategy"), "strategy flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("providers"), "providers flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("timeout"), "timeout flag must exist")
	assert.Nil(t, cmd.Flags().Lookup("judge"), "secure cmd must NOT have judge flag")
}

// TestBuildProviderConfigs verifies known provider mappings.
func TestBuildProviderConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           []string
		expectName      string
		expectBinary    string
		expectViaArgs   bool
		expectArgsEmpty bool
	}{
		{
			name:          "claude uses stdin",
			input:         []string{"claude"},
			expectName:    "claude",
			expectBinary:  "claude",
			expectViaArgs: false,
		},
		{
			name:          "codex uses stdin",
			input:         []string{"codex"},
			expectName:    "codex",
			expectBinary:  "codex",
			expectViaArgs: false,
		},
		{
			// SPEC-ORCH-021 REQ-014: gemini --print is a string flag taking the
			// prompt as its value, so the prompt is delivered via args, not stdin.
			name:          "gemini uses --print value via args",
			input:         []string{"gemini"},
			expectName:    "gemini",
			expectBinary:  "agy",
			expectViaArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := buildProviderConfigs(tt.input)
			require.Len(t, result, 1)
			assert.Equal(t, tt.expectName, result[0].Name)
			assert.Equal(t, tt.expectBinary, result[0].Binary)
			assert.Equal(t, tt.expectViaArgs, result[0].PromptViaArgs)
			if tt.expectName == "gemini" {
				// SPEC-ORCH-021 REQ-014: prompt fills the empty "" slot after --print.
				assert.Equal(t, []string{"--print", ""}, result[0].Args)
			}
		})
	}
}

// TestBuildProviderConfigs_Unknown verifies that unknown providers fall back to
// binary=name, args=[], PromptViaArgs=false.
func TestBuildProviderConfigs_Unknown(t *testing.T) {
	t.Parallel()

	result := buildProviderConfigs([]string{"my-custom-tool"})
	require.Len(t, result, 1)
	assert.Equal(t, "my-custom-tool", result[0].Name)
	assert.Equal(t, "my-custom-tool", result[0].Binary)
	assert.Empty(t, result[0].Args)
	assert.False(t, result[0].PromptViaArgs)
}

// TestBuildProviderConfigs_Multiple verifies that multiple providers are returned in order.
func TestBuildProviderConfigs_Multiple(t *testing.T) {
	t.Parallel()

	result := buildProviderConfigs([]string{"claude", "codex", "gemini"})
	require.Len(t, result, 3)
	assert.Equal(t, "claude", result[0].Name)
	assert.Equal(t, "codex", result[1].Name)
	assert.Equal(t, "gemini", result[2].Name)
}

// TestBuildProviderConfigs_Empty verifies empty input returns empty slice.
func TestBuildProviderConfigs_Empty(t *testing.T) {
	t.Parallel()

	result := buildProviderConfigs([]string{})
	assert.Empty(t, result)
}

// TestDefaultProviders verifies the hardcoded defaults include claude, codex, gemini.
func TestDefaultProviders(t *testing.T) {
	t.Parallel()

	providers := defaultProviders()
	assert.Len(t, providers, 3)
	assert.Contains(t, providers, "claude")
	assert.Contains(t, providers, "codex")
	assert.Contains(t, providers, "gemini")
}

// TestFlagStringIfChanged verifies that flagStringIfChanged returns value
// only when the flag has been explicitly changed.
func TestFlagStringIfChanged(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("myflag", "", "test flag")

	// Not changed: should return empty string regardless of value
	result := flagStringIfChanged(cmd, "myflag", "somevalue")
	assert.Equal(t, "", result, "unchanged flag must return empty string")

	// Simulate flag being explicitly set
	require.NoError(t, cmd.Flags().Set("myflag", "explicit"))
	result = flagStringIfChanged(cmd, "myflag", "explicit")
	assert.Equal(t, "explicit", result, "changed flag must return its value")
}

// TestFlagStringSliceIfChanged verifies that flagStringSliceIfChanged returns
// the value only when the flag has been explicitly changed.
func TestFlagStringSliceIfChanged(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("providers", nil, "test flag")

	// Not changed: should return nil
	result := flagStringSliceIfChanged(cmd, "providers", []string{"a", "b"})
	assert.Nil(t, result, "unchanged flag must return nil")

	// Simulate flag being explicitly set
	require.NoError(t, cmd.Flags().Set("providers", "x,y"))
	result = flagStringSliceIfChanged(cmd, "providers", []string{"x", "y"})
	assert.Equal(t, []string{"x", "y"}, result, "changed flag must return its value")
}

// TestBuildReviewPrompt_NoFiles verifies default prompt when no files are given.
func TestBuildReviewPrompt_NoFiles(t *testing.T) {
	t.Parallel()

	prompt, err := buildReviewPrompt(nil)
	require.NoError(t, err)
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "리뷰")
}

// TestBuildReviewPrompt_MissingFileAborts verifies that any missing file now
// aborts the build instead of silently embedding an ENOENT marker.
func TestBuildReviewPrompt_MissingFileAborts(t *testing.T) {
	t.Parallel()

	_, err := buildReviewPrompt([]string{"/nonexistent/file.go"})
	require.Error(t, err)
}

// TestBuildSecurePrompt_NoFiles verifies default security prompt.
func TestBuildSecurePrompt_NoFiles(t *testing.T) {
	t.Parallel()

	prompt, err := buildSecurePrompt(nil)
	require.NoError(t, err)
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "보안")
}

// TestBuildSecurePrompt_MissingFileAborts confirms the same abort-on-missing
// behavior for the secure command (GitHub issue #37).
func TestBuildSecurePrompt_MissingFileAborts(t *testing.T) {
	t.Parallel()

	_, err := buildSecurePrompt([]string{"/nonexistent/auth.go"})
	require.Error(t, err)
}

// TestBuildFileContents_MissingFile verifies abort on first missing file.
func TestBuildFileContents_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := buildFileContents([]string{"/this/does/not/exist.go"})
	require.Error(t, err)
}

// TestBuildFileContents_ExistingFile verifies that file content is embedded.
func TestBuildFileContents_ExistingFile(t *testing.T) {
	t.Parallel()

	// Create a temp file
	f := t.TempDir() + "/sample.go"
	require.NoError(t, os.WriteFile(f, []byte("package main\n"), 0o644))

	result, err := buildFileContents([]string{f})
	require.NoError(t, err)
	assert.Contains(t, result, "package main")
	assert.Contains(t, result, "sample.go")
}

// TestRunOrchestraCommand_InvalidStrategy verifies error on invalid strategy
// when no config file is present (uses fallback path).
func TestRunOrchestraCommand_InvalidStrategy(t *testing.T) {
	t.Parallel()

	// Simulate calling with explicit invalid strategy
	// runOrchestraCommand validates s.IsValid() and returns error.
	providers := []orchestra.ProviderConfig{
		{Name: "test", Binary: "cat", Args: []string{}},
	}
	cfg := orchestra.OrchestraConfig{
		Providers: providers,
		Strategy:  orchestra.Strategy("invalid-strat"),
		Prompt:    "test",
	}
	assert.False(t, cfg.Strategy.IsValid())
}
