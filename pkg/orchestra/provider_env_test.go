package orchestra

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A provider that must drop variables never starts with the whole
// environment: a command without SetEnv fails closed, and the filter drops
// every spelling of a name while keeping the rest in order.
func TestApplyProviderEnv_DropsNamedVariablesOrFailsClosed(t *testing.T) {
	t.Parallel()
	assert.NoError(t, applyProviderEnv(&fakeCommand{}, nil, nil), "nothing to drop needs no capability")
	assert.ErrorIs(t, applyProviderEnv(&fakeCommand{}, nil, []string{"GH_TOKEN"}), errProviderEnvUnsupported)
	assert.Equal(t, []string{"PATH=/bin", "KEEP=1", "GH_TOKENS=other"},
		EnvironWithout([]string{"PATH=/bin", "gh_token=x", "GH_TOKEN=y", "KEEP=1", "GH_TOKENS=other"}, []string{"GH_TOKEN"}))
}

// A name that ends in * drops every variable with that prefix, in any letter
// case, so a family such as AWS_CONTAINER_* needs no list of its members.
func TestEnvironWithout_TrailingStarDropsAPrefixFamily(t *testing.T) {
	t.Parallel()
	env := []string{"AWS_CONTAINER_CREDENTIALS_FULL_URI=x", "aws_container_authorization_token=y", "AWS_CONTAINER=bare",
		"AWS_CONTAINERS=other", "AWS_REGION=eu", "*=star"}
	assert.Equal(t, []string{"AWS_CONTAINER=bare", "AWS_CONTAINERS=other", "AWS_REGION=eu", "*=star"},
		EnvironWithout(env, []string{"AWS_CONTAINER_*"}))
	assert.Equal(t, env, EnvironWithout(env, []string{"*"}), "a bare * names no family")
}

// envRecordingCommand is a fake command that takes a replaced environment.
type envRecordingCommand struct {
	fakeCommand
	env []string
}

func (c *envRecordingCommand) SetEnv(env []string) { c.env = env }

// A keep list names the only variables that survive, by exact name in its
// letter case or by a prefix family, so a variable nobody listed, such as an
// agent session's socket or token, never reaches the provider.
func TestEnvironOnly_KeepsExactNamesAndPrefixFamilies(t *testing.T) {
	t.Parallel()
	env := []string{"PATH=/bin", "path=/x", "LC_ALL=C", "LC_=bare", "LCX=no", "CLAUDECODE=1", "HOME=/h", "*=star"}
	assert.Equal(t, []string{"PATH=/bin", "LC_ALL=C", "LC_=bare", "HOME=/h"}, EnvironOnly(env, []string{"PATH", "LC_*", "HOME"}))
	assert.Empty(t, EnvironOnly(env, []string{"*"}), "a bare * names no family")
	assert.Empty(t, EnvironOnly(env, nil))
}

// A provider with a keep list starts with only those variables, and its
// unset list still drops from what the keep list kept; a command without
// SetEnv fails closed.
func TestApplyProviderEnv_KeepListThenUnsetList(t *testing.T) {
	t.Setenv("ORCH_KEEP_A", "a")
	t.Setenv("ORCH_KEEP_B", "b")
	t.Setenv("ORCH_DROPPED", "x")
	assert.ErrorIs(t, applyProviderEnv(&fakeCommand{}, []string{"ORCH_KEEP_*"}, nil), errProviderEnvUnsupported)
	cmd := &envRecordingCommand{}
	assert.NoError(t, applyProviderEnv(cmd, []string{"ORCH_KEEP_*"}, []string{"ORCH_KEEP_B"}))
	assert.Equal(t, []string{"ORCH_KEEP_A=a"}, cmd.env)
}
