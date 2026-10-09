package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func TestWriteOrchestraCLIOutput_JSONIncludesTypedReceiptAndMergedResult(t *testing.T) {
	t.Parallel()

	result := &orchestra.OrchestraResult{
		Merged: "frozen findings",
		RunReceipt: &orchestra.OrchestrationRunReceipt{
			Schema:        orchestra.OrchestrationReceiptSchema,
			GateStatus:    "blocked",
			DispatchCount: 2,
		},
	}
	var out bytes.Buffer

	err := writeOrchestraCLIOutput(&out, result, orchestraOutputJSON)

	require.NoError(t, err)
	var payload orchestraCLIOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	assert.Equal(t, orchestraCLIOutputSchema, payload.Schema)
	assert.Equal(t, "frozen findings", payload.Merged)
	require.NotNil(t, payload.Receipt)
	assert.Equal(t, orchestra.OrchestrationReceiptSchema, payload.Receipt.Schema)
	assert.Equal(t, "blocked", payload.Receipt.GateStatus)
	assert.NotContains(t, out.String(), `"session_id"`)
	assert.NotContains(t, out.String(), `"round_history"`)
	assert.NotContains(t, out.String(), `"panes"`)
}

func TestWriteOrchestraCLIOutput_JSONFailsClosedWithoutReceipt(t *testing.T) {
	t.Parallel()

	err := writeOrchestraCLIOutput(&bytes.Buffer{}, &orchestra.OrchestraResult{Merged: "text only"}, orchestraOutputJSON)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "receipt")
}

func TestSaveOrchestraResult_WritesReceiptSidecar(t *testing.T) {
	root := t.TempDir()
	chdirForTest(t, root)
	result := &orchestra.OrchestraResult{
		Merged: "review output",
		RunReceipt: &orchestra.OrchestrationRunReceipt{
			Schema: orchestra.OrchestrationReceiptSchema,
		},
	}

	resultPath, err := saveOrchestraResult("review", "consensus", []string{"claude"}, ResolvedOrchestraTimeout{}, result)

	require.NoError(t, err)
	receiptPath := resultPath + ".receipt.json"
	receiptBody, readErr := os.ReadFile(receiptPath)
	require.NoError(t, readErr)
	var receipt orchestra.OrchestrationRunReceipt
	require.NoError(t, json.Unmarshal(receiptBody, &receipt))
	assert.Equal(t, orchestra.OrchestrationReceiptSchema, receipt.Schema)
	assert.Equal(t, filepath.Dir(resultPath), filepath.Dir(receiptPath))
}

func TestOrchestraReviewAndBrainstorm_RegisterTypedOutputFormat(t *testing.T) {
	t.Parallel()

	for _, cmd := range []*cobra.Command{newOrchestraReviewCmd(), newOrchestraBrainstormCmd()} {
		assert.NotNil(t, cmd.Flags().Lookup("format"))
		assert.NotNil(t, cmd.Flags().Lookup("no-detach"))
	}
}
