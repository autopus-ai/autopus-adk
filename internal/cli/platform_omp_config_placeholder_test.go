package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The OMP profile writer re-encodes the whole document through
// replaceAutopusConfigSection. yaml.Marshal spells an escaped "\x24{NAME}"
// in plain text, so without a guard a value config.Load kept literal would
// come back as a placeholder it expands — leaking the variable on reload.
func TestReplaceAutopusConfigSection_RefusesToActivateAnEscapedPlaceholder(t *testing.T) {
	for _, unset := range []bool{true, false} {
		t.Setenv("ZZTOK", "s3cret")
		if unset {
			require.NoError(t, os.Unsetenv("ZZTOK"))
		}
		original := []byte("mode: full\nproject_name: \"\\x24{ZZTOK}\"\nplatforms:\n  - claude-code\n" +
			"role_model_policy:\n  profile: balanced\n")
		cfg, err := config.ParseConfig(original)
		require.NoError(t, err)

		encoded, err := marshalAutopusConfig(original, cfg)

		require.ErrorIs(t, err, errRewriteActivatesPlaceholder, "unset=%v", unset)
		assert.Nil(t, encoded)
	}
}

func TestReplaceAutopusConfigSection_KeepsAnAlreadyActivePlaceholder(t *testing.T) {
	t.Setenv("ZZTOK", "s3cret")
	require.NoError(t, os.Unsetenv("ZZTOK"))
	original := []byte("mode: full\nproject_name: \"${ZZTOK}\"\nplatforms:\n  - claude-code\n" +
		"role_model_policy:\n  profile: balanced\n")
	cfg, err := config.ParseConfig(original)
	require.NoError(t, err)

	encoded, err := marshalAutopusConfig(original, cfg)

	require.NoError(t, err)
	assert.Contains(t, string(encoded), "${ZZTOK}")
}
