package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// The six historical reason texts are part of the plan and brainstorm output
// contract (S17), so the typed violation must render them byte-identically.
func TestReadOnlyProviderPolicy_ViolationKeepsHistoricalErrorText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider orchestra.ProviderConfig
		want     string
	}{
		{
			name:     "unsupported provider",
			provider: orchestra.ProviderConfig{Name: "custom", Binary: "custom-agent"},
			want:     `read-only provider policy: unsupported provider "custom"`,
		},
		{
			name:     "native binary",
			provider: orchestra.ProviderConfig{Name: "codex", Binary: "/tmp/codex", Args: []string{"exec"}},
			want:     `read-only provider policy: provider "codex" requires native binary "codex"`,
		},
		{
			name:     "unsafe argv",
			provider: orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print", "--dangerously-skip-permissions"}},
			want:     `read-only provider policy: provider "claude" contains unsafe argv "--dangerously-skip-permissions"`,
		},
		{
			name:     "unsupported argv",
			provider: orchestra.ProviderConfig{Name: "codex", Binary: "codex", Args: []string{"exec", "--profile", "attacker"}},
			want:     `read-only provider policy: provider "codex" contains unsupported argv "--profile"`,
		},
		{
			name:     "incomplete argv",
			provider: orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print", "--model"}},
			want:     `read-only provider policy: provider "claude" has incomplete argv "--model"`,
		},
		{
			name:     "unsafe value",
			provider: orchestra.ProviderConfig{Name: "codex", Binary: "codex", Args: []string{"exec", "--sandbox", "danger-full-access"}},
			want:     `read-only provider policy: provider "codex" contains unsafe value for "--sandbox"`,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{tt.provider}, readOnlyPolicyOptions{})
			require.Error(t, err)
			assert.Equal(t, tt.want, err.Error())
		})
	}
}

// Callers that name a config key and a remedy (the spec review Error
// Contract) read the provider, field, item, and value from the typed error.
func TestReadOnlyProviderPolicy_ViolationCarriesTypedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   orchestra.ProviderConfig
		want       readOnlyPolicyViolation
		wantReason string
	}{
		{
			name:       "unsupported provider",
			provider:   orchestra.ProviderConfig{Name: "opencode", Binary: "opencode"},
			want:       readOnlyPolicyViolation{Provider: "opencode", Item: "opencode", Kind: readOnlyUnsupportedProvider},
			wantReason: `unsupported provider "opencode"`,
		},
		{
			name:       "wrapper binary",
			provider:   orchestra.ProviderConfig{Name: "claude", Binary: "/tmp/claude-wrapper", Args: []string{"--print"}},
			want:       readOnlyPolicyViolation{Provider: "claude", Field: "binary", Item: "claude", Value: "/tmp/claude-wrapper", Kind: readOnlyNativeBinary},
			wantReason: `requires native binary "claude"`,
		},
		{
			name: "unsafe pane argv",
			provider: orchestra.ProviderConfig{
				Name: "claude", Binary: "claude", Args: []string{"--print"}, PaneArgs: []string{"--print", "--dangerously-skip-permissions"},
			},
			want:       readOnlyPolicyViolation{Provider: "claude", Field: "pane_args", Item: "--dangerously-skip-permissions", Kind: readOnlyUnsafeArgv},
			wantReason: `contains unsafe argv "--dangerously-skip-permissions"`,
		},
		{
			name:       "unsupported argv",
			provider:   orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print", "--verbose"}},
			want:       readOnlyPolicyViolation{Provider: "claude", Field: "args", Item: "--verbose", Kind: readOnlyUnsupportedArgv},
			wantReason: `contains unsupported argv "--verbose"`,
		},
		{
			name:       "incomplete argv",
			provider:   orchestra.ProviderConfig{Name: "gemini", Binary: "agy", Args: []string{"--print", "", "--mode"}},
			want:       readOnlyPolicyViolation{Provider: "gemini", Field: "args", Item: "--mode", Kind: readOnlyIncompleteArgv},
			wantReason: `has incomplete argv "--mode"`,
		},
		{
			name:       "separated unsafe value",
			provider:   orchestra.ProviderConfig{Name: "codex", Binary: "codex", Args: []string{"exec", "-c", "sandbox_mode=workspace-write"}},
			want:       readOnlyPolicyViolation{Provider: "codex", Field: "args", Item: "-c", Value: "sandbox_mode=workspace-write", Kind: readOnlyUnsafeValue},
			wantReason: `contains unsafe value for "-c"`,
		},
		{
			name:       "inline unsafe value",
			provider:   orchestra.ProviderConfig{Name: "codex", Binary: "codex", Args: []string{"exec", "-c=sandbox_mode=workspace-write"}},
			want:       readOnlyPolicyViolation{Provider: "codex", Field: "args", Item: "-c", Value: "sandbox_mode=workspace-write", Inline: true, Kind: readOnlyUnsafeValue},
			wantReason: `contains unsafe value for "-c"`,
		},
		{
			name:       "schema flag",
			provider:   orchestra.ProviderConfig{Name: "claude", Binary: "claude", Args: []string{"--print"}, SchemaFlag: "--permission-mode=bypassPermissions"},
			want:       readOnlyPolicyViolation{Provider: "claude", Field: "subprocess.schema_flag", Item: "--permission-mode=bypassPermissions", Kind: readOnlyUnsupportedSchemaFlag},
			wantReason: `unsupported schema flag "--permission-mode=bypassPermissions"`,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{tt.provider}, readOnlyPolicyOptions{})
			var violation *readOnlyPolicyViolation
			require.True(t, errors.As(err, &violation), "policy errors must be typed violations: %v", err)
			assert.Equal(t, tt.want, *violation)
			assert.Equal(t, tt.wantReason, violation.Reason())
		})
	}
}

// The schema flag is a new rejection; its plan and brainstorm text follows the
// historical "provider <name> <reason>" shape.
func TestReadOnlyProviderPolicy_SchemaFlagViolationText(t *testing.T) {
	t.Parallel()

	_, err := applyReadOnlyProviderPolicy([]orchestra.ProviderConfig{{
		Name: "gemini", Binary: "agy", Args: []string{"--print", ""}, SchemaFlag: "--json-schema",
	}}, readOnlyPolicyOptions{})
	require.Error(t, err)
	assert.Equal(t, `read-only provider policy: provider "gemini" has unsupported schema flag "--json-schema"`, err.Error())
}
