package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The v0.50.125 OMP contracts run on agent_source: native, the source they
// were written for (SPEC-MODELUX-003 Existing Test Changes rule 2); the
// default autopus source has its own tests.

func nativeOMPConfig(cfg *config.HarnessConfig) *config.HarnessConfig {
	cfg.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	return cfg
}

// markOMPProjectNative moves a fixture project written by config.Save onto
// the native source.
func markOMPProjectNative(t *testing.T, root string) {
	t.Helper()
	cfg, err := config.LoadPreview(root)
	require.NoError(t, err)
	require.NoError(t, config.Save(root, nativeOMPConfig(cfg)))
}

// writeNativeOMPProjectConfig gives a bare receipt fixture the native config
// its receipt was generated for.
func writeNativeOMPProjectConfig(t *testing.T, root string) {
	t.Helper()
	cfg := config.DefaultFullConfig("omp-native")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, nativeOMPConfig(cfg)))
}

// writeNativeComposeFixture is writeComposeFixture on the native source.
func writeNativeComposeFixture(t *testing.T, platforms ...string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, nativeOMPConfig(composeFixtureConfig(platforms...))))
	return dir
}
