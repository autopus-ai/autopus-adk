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
	assert.NoError(t, applyProviderEnv(&fakeCommand{}, nil), "nothing to drop needs no capability")
	assert.ErrorIs(t, applyProviderEnv(&fakeCommand{}, []string{"GH_TOKEN"}), errProviderEnvUnsupported)
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
