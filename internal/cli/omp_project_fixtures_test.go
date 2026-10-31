package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// The v0.50.125 OMP contracts ran on agent_source: native until v0.50.131
// removed it; every fixture here is on the one source left, autopus.

// writeOMPProjectConfig gives a bare receipt fixture the config its receipt
// was generated for.
func writeOMPProjectConfig(t *testing.T, root string) {
	t.Helper()
	cfg := config.DefaultFullConfig("omp-receipt")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(root, cfg))
}
