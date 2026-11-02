package cli

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// statusResume is what `auto pipeline run <SPEC> --continue` would do with a
// stopped run, decided by the checks the CLI runs and the engine's own resume
// admission, output-record verification included (REQ-SEV-EXEC-02).
type statusResume struct {
	Admitted    bool     `json:"admitted"`
	Reason      string   `json:"reason"`
	Restores    []string `json:"restores"`
	Reruns      []string `json:"reruns"`
	Checked     []string `json:"checked"`
	Command     string   `json:"command,omitempty"`
	CommandNote string   `json:"command_note,omitempty"`
}

// statusResumeChecks are the checks an admission covers. The provider CLI
// and delegation limits are checked only when the run starts.
var statusResumeChecks = []string{
	"git HEAD", "SPEC snapshot", "checkpoint identity and version", "dependencies", "route", "output records",
}

// pipelinePlatformBinaries maps the platforms `auto pipeline run --platform`
// accepts to the CLI each one runs. A platform outside it never reaches a
// recommended command.
var pipelinePlatformBinaries = map[string]string{
	"claude": "claude", "codex": "codex", "gemini": "agy", "omp": "omp",
}

func (r *statusEvidenceReader) assessResume(
	root, specID, specDir string, cp *pipeline.Checkpoint, execution *statusExecution,
) *statusResume {
	resume := &statusResume{Restores: []string{}, Reruns: []string{}, Checked: statusResumeChecks}
	head := r.head(root)
	switch {
	case head.err != nil:
		resume.Reason = "git HEAD unavailable; --continue cannot verify the checkpoint without it"
		return resume
	case cp.GitCommitHash != head.head:
		resume.Reason = "HEAD moved since the run; --continue refuses a stale checkpoint"
		return resume
	}
	snapshot, err := pipeline.SpecSnapshotHash(specDir)
	if err != nil {
		resume.Reason = statusSingleLine("SPEC documents unreadable: " + err.Error())
		return resume
	}
	route, _ := resolvePipelineRoute(root, specDir, specID)
	admission := pipeline.AdmitResume(cp, specID, snapshot, head.head, route)
	if admission.Err != nil {
		resume.Reason = statusSingleLine(admission.Err.Error())
		return resume
	}
	resume.Admitted = true
	resume.Restores, resume.Reruns = phaseNames(admission.Restored), phaseNames(admission.Rerun)
	resume.Reason = "the engine admits it"
	resume.Command, resume.CommandNote = r.resumeCommand(specID, execution)
	return resume
}

// resumeCommand builds the command from validated values only: a SPEC ID the
// pipeline accepts, a known platform, and a known execution owner. It pins
// the run's platform and owner so a resume does not switch providers or
// owners because PATH changed or a default applies (REQ-SEV-EXEC-02,
// REQ-SEV-NEXT-04). Without a command, the note says why.
func (r *statusEvidenceReader) resumeCommand(specID string, execution *statusExecution) (string, string) {
	if err := pipeline.ValidateSpecID(specID); err != nil {
		return "", "the SPEC ID cannot be passed to a command"
	}
	binary, known := pipelinePlatformBinaries[execution.Platform]
	switch {
	case !known:
		return "", "the run's recorded platform is not one auto pipeline run accepts"
	case !r.installed[binary]:
		return "", "the run used " + execution.Platform + ", whose " + binary + " CLI is not installed"
	}
	command := "auto pipeline run " + specID + " --continue --platform " + execution.Platform
	if execution.Platform != "omp" {
		return command, ""
	}
	switch execution.ExecutionOwner {
	case pipelineExecutionOwnerOrca:
		return command + " --execution-owner orca", ""
	case pipelineExecutionOwnerOMP:
		return command, ""
	}
	// An owner-less resume would silently run under the default owner omp.
	// Status names neither owner for the user; they check which ran.
	return "", "execution owner unknown: the run recorded no dispatch; check whether omp or orca ran it, then run `" +
		command + " --execution-owner <omp|orca>`"
}

// recordedExecutionOwner reads which owner ran an OMP run's dispatches from
// the run's own receipt: orca dispatches report backend orca, the native OMP
// DAG an rpc backend. A run that dispatched nothing records no owner.
func recordedExecutionOwner(receipt *pipeline.OrchestrationRunReceipt) string {
	owner := "unknown"
	for _, dispatch := range receipt.ProviderReceipts {
		switch {
		case dispatch.Backend == pipelineOrcaBackendName:
			return pipelineExecutionOwnerOrca
		case strings.HasPrefix(dispatch.Backend, "rpc-"):
			owner = pipelineExecutionOwnerOMP
		}
	}
	return owner
}

func phaseNames(phases []pipeline.PhaseID) []string {
	names := make([]string, len(phases))
	for i, phase := range phases {
		names[i] = string(phase)
	}
	return names
}
