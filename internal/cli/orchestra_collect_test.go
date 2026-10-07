package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrchestraCollectCmd_Flags(t *testing.T) {
	t.Parallel()

	cmd := newLegacyOrchestraCollectCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "collect <session-id>", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("round"), "round flag must exist")
	assert.NotNil(t, cmd.Flags().Lookup("workspace-ref"), "legacy cmux recovery flag must exist")
}

func TestNewOrchestraCollectCmd_RequiresArgs(t *testing.T) {
	t.Parallel()

	cmd := newLegacyOrchestraCollectCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err, "should fail without session-id argument")
}
