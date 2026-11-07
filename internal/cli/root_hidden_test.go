package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every listed command exists, is hidden from the root help, and still runs.
func TestRootHelp_HidesLegacyHarnessCommands(t *testing.T) {
	root := NewRootCmd()
	for _, name := range legacyHarnessCommands {
		cmd, _, err := root.Find([]string{name})
		require.NoError(t, err, name)
		require.Equal(t, name, cmd.Name(), "%s must still be registered", name)
		assert.True(t, cmd.Hidden, name)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	listing := out.String()
	for _, name := range legacyHarnessCommands {
		assert.NotRegexp(t, `(?m)^\s+`+name+`\s`, listing, name)
	}
	for _, kept := range []string{"review", "qa", "verify", "test", "sync", "update", "doctor", "spec", "orchestra", "workflow", "terminal", "status"} {
		assert.Regexp(t, `(?m)^\s+`+kept+`\s`, listing, kept)
	}
	assert.False(t, strings.Contains(listing, "Lock and verify the reproduction test"), listing)

	root = NewRootCmd()
	out.Reset()
	root.SetOut(&out)
	root.SetArgs([]string{"fix", "--help"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "Usage:")
}
