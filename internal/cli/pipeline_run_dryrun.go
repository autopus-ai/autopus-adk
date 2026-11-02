package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// pipelineDryRunStateDir keeps dry-run receipts apart from the canonical
// checkpoint. A dry run dispatches nothing, so it must never replace the
// evidence of the last real run that status and --continue read
// (SPEC-STATUSEVIDENCE-001 REQ-SEV-EXEC-04).
var pipelineDryRunStateDir = filepath.Join(pipelineStateDir, "dry-run")

// pipelineRunStateDir is where a run persists its checkpoint and receipt.
func pipelineRunStateDir(dryRun bool) string {
	if dryRun {
		return pipelineDryRunStateDir
	}
	return pipelineStateDir
}

// pipelineRunBlockedError persists a preflight blocker where the run keeps
// its state: a real run on the canonical surface, a dry run in its own
// directory, replacing only the previous dry run's receipt.
func pipelineRunBlockedError(
	dryRun bool, specID, snapshotHash, gitHash string, strategy pipeline.Strategy, cause error,
) error {
	if !dryRun {
		return pipelineBlockedError(specID, snapshotHash, gitHash, strategy, cause)
	}
	receipt := pipeline.NewBlockedRunReceipt(specID, strategy, cause.Error())
	statuses := make(map[string]pipeline.CheckpointStatus, len(pipeline.DefaultPhases()))
	for _, phase := range pipeline.DefaultPhases() {
		statuses[string(phase.ID)] = pipeline.CheckpointStatusPending
	}
	cp := pipeline.Checkpoint{
		Version: pipeline.CheckpointVersion, RouteVersion: pipeline.PipelineRouteVersion,
		SpecID: specID, SnapshotHash: snapshotHash, GitCommitHash: gitHash,
		TaskStatus: statuses, Receipt: &receipt,
	}
	err := os.MkdirAll(pipelineDryRunStateDir, 0o755)
	if err == nil {
		err = cp.SaveFile(filepath.Join(pipelineDryRunStateDir, specID+".yaml"))
	}
	if err != nil {
		return fmt.Errorf("%w (persist dry-run blocked receipt: %v)", cause, err)
	}
	return cause
}
