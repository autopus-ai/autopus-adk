package pipeline_test

import (
	"context"
	"testing"

	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/stretchr/testify/require"
)

func TestPipelineOwnershipMismatchBlocksBeforeAcceptingWorkerReceipt(t *testing.T) {
	receipt := pipelineWorkerReceiptFixture()
	receipt.ChangedFiles = []string{"pkg/other/secret.go"}
	backend := &workerReceiptBackend{outputs: []string{pipelineMarkedReceipt(t, receipt)}}
	engine := pipeline.NewSubprocessEngine(pipeline.EngineConfig{
		SpecID: "SPEC-OWNERSHIP-001", Platform: "plain",
		Strategy: pipeline.StrategySequential, Backend: backend,
	})
	result, err := engine.Run(context.Background())
	require.ErrorContains(t, err, "outside declared ownership")
	require.Equal(t, pipeline.TerminalBlocked, result.Receipt.Terminal)
	require.Empty(t, result.Receipt.WorkerReceipts)
	require.Equal(t, 1, result.Receipt.DispatchCount)
	require.Zero(t, result.Receipt.CompletedPhaseCount)
}
