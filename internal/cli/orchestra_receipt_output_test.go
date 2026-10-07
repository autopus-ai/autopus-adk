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

func TestWriteOrchestraCLIOutput_JSONYield_PreservesSessionContract(t *testing.T) {
	t.Parallel()

	want := orchestra.YieldOutput{
		Strategy:  "debate",
		Rounds:    1,
		SessionID: "orch-yield-json",
		Panes: map[string]string{
			"claude": "surface:41",
			"codex":  "surface:42",
		},
		RoundHistory: []orchestra.YieldRound{{
			Round: 1,
			Responses: []orchestra.YieldResponse{{
				Provider: "claude",
				Output:   "round one response",
			}},
		}},
	}
	result := &orchestra.OrchestraResult{
		Merged: "generic output must not replace the yield contract",
		RunReceipt: &orchestra.OrchestrationRunReceipt{
			Schema: orchestra.OrchestrationReceiptSchema,
		},
		Yield: &want,
	}
	var out bytes.Buffer

	err := writeOrchestraCLIOutput(&out, result, orchestraOutputJSON)

	require.NoError(t, err)
	var got orchestra.YieldOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), "stdout must contain exactly one JSON document")
	assert.Equal(t, want, got)
	assert.NotContains(t, out.String(), orchestraCLIOutputSchema)
	assert.NotContains(t, out.String(), "generic output must not replace the yield contract")
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

func TestOrchestraResultCmd_JSONReturnsDetachedReceipt(t *testing.T) {
	t.Parallel()

	jobDir := t.TempDir()
	jobID := "result-json-001"
	jobJSON := `{"id":"result-json-001","run_id":"run-json-001","status":"done","strategy":"consensus",` +
		`"providers":["claude","codex"],` +
		`"results":{"claude":{"provider":"claude","output":"ok"},"codex":{"provider":"codex","output":"ok"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(jobDir, jobID+".json"), []byte(jobJSON), 0o600))
	cmd := newLegacyOrchestraJobResultCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{jobID, "--job-dir", jobDir, "--format", "json"})

	err := cmd.Execute()

	require.NoError(t, err)
	var payload orchestraCLIOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	assert.Equal(t, orchestraCLIOutputSchema, payload.Schema)
	require.NotNil(t, payload.Receipt)
	assert.Equal(t, orchestra.OrchestrationReceiptSchema, payload.Receipt.Schema)
}

func TestOrchestraReviewAndBrainstorm_RegisterTypedOutputFormat(t *testing.T) {
	t.Parallel()

	for _, cmd := range []*cobra.Command{newOrchestraReviewCmd(), newOrchestraBrainstormCmd()} {
		assert.NotNil(t, cmd.Flags().Lookup("format"))
		assert.NotNil(t, cmd.Flags().Lookup("no-detach"))
	}
	assert.NotNil(t, newLegacyOrchestraJobWaitCmd().Flags().Lookup("format"))
}

func TestWriteOrchestraJobWaitOutput_JSONPointsToTypedResult(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := writeOrchestraJobWaitOutput(&out, "job-123", orchestra.JobStatusDone, orchestraOutputJSON)

	require.NoError(t, err)
	var payload orchestraJobWaitOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	assert.Equal(t, orchestraJobWaitOutputSchema, payload.Schema)
	assert.Equal(t, orchestra.JobStatusDone, payload.Status)
	assert.Equal(t, "auto orchestra result job-123 --format json", payload.NextRequiredStep)
}
