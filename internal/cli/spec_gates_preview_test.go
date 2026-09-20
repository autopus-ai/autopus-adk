package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/spec/gates"
	"github.com/stretchr/testify/require"
)

func TestSpecGatesReadOnlyPreservesConfigAndReceipt(t *testing.T) {
	root, specDir := specGatesProject(t)
	configPath := filepath.Join(root, "autopus.yaml")
	configBytes := []byte("project_name: preview\nmode: full\nplatforms: [codex]\n")
	require.NoError(t, os.WriteFile(configPath, configBytes, 0o600))
	receiptPath := filepath.Join(specDir, "gate-applicability.json")
	for _, existing := range []bool{false, true} {
		if existing {
			require.NoError(t, os.WriteFile(receiptPath, []byte("previous receipt\n"), 0o600))
		}
		out, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go", "--read-only", "--json")
		require.NoError(t, err)
		var receipt gates.ApplicabilityReceipt
		require.NoError(t, json.Unmarshal([]byte(out), &receipt))
		decision, found := receipt.Decision(gates.GateValidation)
		require.True(t, found)
		require.Equal(t, gates.Required, decision.Applicability)
		got, err := os.ReadFile(configPath)
		require.NoError(t, err)
		require.Equal(t, configBytes, got)
		got, err = os.ReadFile(receiptPath)
		if existing {
			require.NoError(t, err)
			require.Equal(t, "previous receipt\n", string(got))
		} else {
			require.True(t, os.IsNotExist(err))
		}
	}
}

func TestSpecGatesNoReuseKeepsFreshEvidenceRequired(t *testing.T) {
	_, specDir := specGatesProject(t)
	_, err := runSpecGates(t, "record", specDir, "--gate", "build", "--status", "pass", "--inputs", "pkg/a/*.go")
	require.NoError(t, err)
	out, err := runSpecGates(t, specDir, "--changed", "pkg/a/x.go", "--read-only", "--no-reuse")
	require.NoError(t, err)
	require.Contains(t, out, "build: required")
	require.Contains(t, out, "reuse disabled by caller")
	require.NotContains(t, out, "receipt:")
}
