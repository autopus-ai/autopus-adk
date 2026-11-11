package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestVersionStopConf_Forms(t *testing.T) {
	t.Parallel()
	unset := config.HooksConf{}
	assert.True(t, unset.IsVersionStopEnabled(), "unset version_stop means on")
	assert.Equal(t, 0, unset.VersionStopMaxMajorBehind())

	off, err := config.LoadPreview(writeDiskConfig(t, "hooks:\n  version_stop: false\n"))
	require.NoError(t, err)
	assert.False(t, off.Hooks.IsVersionStopEnabled())

	mapped, err := config.LoadPreview(writeDiskConfig(t, "hooks:\n  version_stop: {max_major_behind: 2}\n"))
	require.NoError(t, err)
	assert.True(t, mapped.Hooks.IsVersionStopEnabled())
	assert.Equal(t, 2, mapped.Hooks.VersionStopMaxMajorBehind())

	for _, tc := range []struct{ body, want string }{
		{"hooks:\n  version_stop: sometimes\n", "hooks.version_stop must be true, false, or a mapping"},
		{"hooks:\n  version_stop: {max_major: 1}\n", "max_major"},
		{"hooks:\n  version_stop: {max_major_behind: -1}\n", "max_major_behind must be >= 0"},
	} {
		_, err := config.LoadPreview(writeDiskConfig(t, tc.body))
		assert.ErrorContains(t, err, tc.want, tc.body)
	}
}
