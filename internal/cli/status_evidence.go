package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// Evidence labels: what status can prove about a SPEC beyond the status its
// document declares. A declared status alone never yields verified
// (SPEC-STATUSEVIDENCE-001 REQ-SEV-DECL-01).
const (
	evidenceVerified       = "verified"
	evidenceStale          = "stale"
	evidenceUnverified     = "unverified"
	evidenceFailed         = "failed"
	evidenceUnfinished     = "unfinished"
	evidenceDocumentedOnly = "documented_only"
	evidenceNone           = "none"
)

// evidenceLabelOrder is the order summaries count the labels in.
var evidenceLabelOrder = []string{
	evidenceVerified, evidenceStale, evidenceUnverified, evidenceFailed,
	evidenceUnfinished, evidenceDocumentedOnly, evidenceNone,
}

// statusEvidence keeps a SPEC's execution, verification, and review evidence
// apart from its declared status and from each other.
type statusEvidence struct {
	Label        string             `json:"label"`
	Execution    *statusExecution   `json:"execution,omitempty"`
	Verification statusVerification `json:"verification"`
	Review       *statusReview      `json:"review,omitempty"`
	// stateRoot is the directory whose pipeline state holds this evidence;
	// pipeline commands for the SPEC run there.
	stateRoot string
}

// statusExecution is the last pipeline run of a SPEC as its receipt records it.
type statusExecution struct {
	Source               string                `json:"source"`
	Error                string                `json:"error,omitempty"`
	RunID                string                `json:"run_id,omitempty"`
	TaskID               string                `json:"task_id,omitempty"`
	Attempt              int                   `json:"attempt,omitempty"`
	ResumedFrom          string                `json:"resumed_from,omitempty"`
	Platform             string                `json:"platform,omitempty"`
	Route                string                `json:"route,omitempty"`
	Terminal             string                `json:"terminal"`
	StopReason           string                `json:"stop_reason,omitempty"`
	Blocker              string                `json:"blocker,omitempty"`
	BaseHead             string                `json:"base_head,omitempty"`
	FinishedAt           string                `json:"finished_at,omitempty"`
	Phases               pipelinePhaseCounts   `json:"phases"`
	Dispatches           int                   `json:"dispatches"`
	CumulativeDispatches int                   `json:"cumulative_dispatches"`
	Resumable            bool                  `json:"resumable"`
	ResumeReason         string                `json:"resume_reason,omitempty"`
	ResumeCommand        string                `json:"resume_command,omitempty"`
	BlockedAttempt       *statusBlockedAttempt `json:"blocked_attempt,omitempty"`
}

// statusBlockedAttempt is a later run stopped before dispatch while the
// canonical checkpoint was kept (`<SPEC>.blocked.yaml`).
type statusBlockedAttempt struct {
	Source     string `json:"source"`
	Blocker    string `json:"blocker"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// statusEvidenceReader reads evidence for every SPEC of one status run and
// caches what SPECs share.
type statusEvidenceReader struct {
	baseDir string
	idCount map[string]int
	heads   map[string]gitHeadResult
	trees   treeDigestCache
}

type gitHeadResult struct {
	head string
	err  error
}

func newStatusEvidenceReader(baseDir string, specs []specEntry) *statusEvidenceReader {
	reader := &statusEvidenceReader{
		baseDir: baseDir, idCount: map[string]int{},
		heads: map[string]gitHeadResult{}, trees: treeDigestCache{},
	}
	for _, entry := range specs {
		reader.idCount[entry.id]++
	}
	return reader
}

func (r *statusEvidenceReader) read(entry specEntry) statusEvidence {
	specDir := filepath.Dir(entry.path)
	root := r.stateRootFor(entry)
	stateDir := filepath.Join(root, pipelineStateDir)
	evidence := statusEvidence{stateRoot: root}
	var latest *pipeline.OrchestrationRunReceipt
	checkpointPath := filepath.Join(stateDir, entry.id+".yaml")
	if cp, err := pipeline.LoadFile(checkpointPath); err == nil {
		evidence.Execution = r.execution(root, checkpointPath, entry.id, specDir, cp)
		if cp.Receipt != nil && cp.Receipt.Terminal != pipeline.TerminalDryRun {
			latest = cp.Receipt
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		evidence.Execution = &statusExecution{
			Source: relativeStatusPath(r.baseDir, checkpointPath), Error: "checkpoint unreadable: " + err.Error(),
		}
	}
	evidence.Verification = checkPipelineVerification(root, stateDir, entry.id, specDir, latest, r.trees)
	evidence.Review = readStatusReview(r.baseDir, specDir)
	evidence.Label = evidenceLabel(entry.isDone(), evidence.Execution, evidence.Verification)
	return evidence
}

// stateRootFor finds where a SPEC's pipeline state lives: the root holding its
// .autopus/specs, or the status directory when the run was started there and
// the SPEC ID names only this SPEC.
func (r *statusEvidenceReader) stateRootFor(entry specEntry) string {
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(entry.path))))
	candidates := []string{moduleRoot}
	if filepath.Clean(moduleRoot) != filepath.Clean(r.baseDir) && r.idCount[entry.id] == 1 {
		candidates = append(candidates, r.baseDir)
	}
	for _, root := range candidates {
		stateDir := filepath.Join(root, pipelineStateDir)
		for _, name := range []string{entry.id + ".yaml", entry.id + ".verification.json"} {
			if _, err := os.Stat(filepath.Join(stateDir, name)); err == nil {
				return root
			}
		}
	}
	return moduleRoot
}

func (r *statusEvidenceReader) execution(
	root, checkpointPath, specID, specDir string,
	cp *pipeline.Checkpoint,
) *statusExecution {
	execution := &statusExecution{Source: relativeStatusPath(r.baseDir, checkpointPath), BaseHead: cp.GitCommitHash}
	receipt := cp.Receipt
	if receipt == nil {
		// A checkpoint without a receipt records no terminal state.
		return execution
	}
	execution.RunID, execution.TaskID, execution.ResumedFrom = receipt.RunID, receipt.TaskID, receipt.ResumedFrom
	execution.Attempt = len(receipt.PriorAttempts) + 1
	if len(receipt.ResolvedProviders) > 0 {
		execution.Platform = receipt.ResolvedProviders[0]
	}
	execution.Route = string(pipeline.NormalizeRoute(receipt.RoutePhaseSet))
	execution.Terminal = string(receipt.Terminal)
	execution.Blocker = statusSingleLine(receipt.Blocker)
	if receipt.FinishedAt != nil {
		execution.FinishedAt = receipt.FinishedAt.UTC().Format(time.RFC3339)
	}
	execution.Phases = countPipelinePhases(*receipt)
	execution.Dispatches, execution.CumulativeDispatches = receipt.DispatchCount, receipt.CumulativeDispatchCount
	if pipelineRunStopped(receipt.Terminal) {
		execution.StopReason = string(receipt.Terminal)
		if n := len(receipt.Failures); n > 0 && receipt.Failures[n-1].Decision != "" {
			execution.StopReason = string(receipt.Failures[n-1].Decision)
		}
		execution.Resumable, execution.ResumeReason = r.assessResume(root, specID, specDir, cp)
		if execution.Resumable {
			execution.ResumeReason = "completed phases are restored once their outputs are re-verified"
			if execution.Phases.Executed+execution.Phases.Restored == 0 {
				execution.ResumeReason = "no phase completed, so the resume starts from the first phase"
			}
			owner := recordedExecutionOwner(filepath.Dir(checkpointPath), specID)
			execution.ResumeCommand = pipelineResumeCommand(specID, execution.Platform, owner)
		}
	}
	execution.BlockedAttempt = r.blockedAttempt(checkpointPath, specID)
	return execution
}

// assessResume predicts whether `auto pipeline run <SPEC> --continue` would be
// admitted, applying the checks the CLI and engine run before any dispatch,
// and says why not. Output records are verified only when the run resumes.
func (r *statusEvidenceReader) assessResume(root, specID, specDir string, cp *pipeline.Checkpoint) (bool, string) {
	head := r.head(root)
	if head.err != nil {
		return false, "git HEAD unavailable; --continue needs it"
	}
	if cp.GitCommitHash != head.head {
		return false, "HEAD moved since the run; --continue refuses a stale checkpoint"
	}
	snapshot, err := pipeline.SpecSnapshotHash(specDir)
	if err != nil {
		return false, "SPEC documents unreadable: " + err.Error()
	}
	if cp.SnapshotHash != "" && cp.SnapshotHash != snapshot {
		return false, "SPEC documents changed since the run; --continue refuses a changed SPEC"
	}
	if err := cp.ValidateResume(specID, pipeline.PipelineRouteVersion, snapshot); err != nil {
		return false, err.Error()
	}
	route, _ := resolvePipelineRoute(root, specDir, specID)
	if err := cp.ValidateResumeRoute(route); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (r *statusEvidenceReader) head(root string) gitHeadResult {
	if cached, ok := r.heads[root]; ok {
		return cached
	}
	head, err := gitHeadAt(root)
	r.heads[root] = gitHeadResult{head: head, err: err}
	return r.heads[root]
}

// blockedAttempt reports a blocked sidecar written after the checkpoint.
func (r *statusEvidenceReader) blockedAttempt(checkpointPath, specID string) *statusBlockedAttempt {
	path := filepath.Join(filepath.Dir(checkpointPath), specID+".blocked.yaml")
	sidecar, err := os.Stat(path)
	if err != nil {
		return nil
	}
	canonical, err := os.Stat(checkpointPath)
	if err != nil || !sidecar.ModTime().After(canonical.ModTime()) {
		return nil
	}
	attempt := &statusBlockedAttempt{Source: relativeStatusPath(r.baseDir, path)}
	cp, err := pipeline.LoadFile(path)
	if err != nil || cp.Receipt == nil {
		attempt.Blocker = "blocked receipt unreadable"
		return attempt
	}
	attempt.Blocker = statusSingleLine(cp.Receipt.Blocker)
	if cp.Receipt.FinishedAt != nil {
		attempt.FinishedAt = cp.Receipt.FinishedAt.UTC().Format(time.RFC3339)
	}
	return attempt
}

// evidenceLabel ranks the evidence: a stopped or unfinished last run outranks
// an older verification, and only a fresh baseline yields verified.
func evidenceLabel(declaredDone bool, execution *statusExecution, verification statusVerification) string {
	if execution != nil && execution.Error == "" {
		switch {
		case execution.Terminal == "":
			return evidenceUnfinished
		case pipelineRunStopped(pipeline.TerminalState(execution.Terminal)):
			return evidenceFailed
		}
	}
	switch verification.State {
	case freshnessFresh:
		return evidenceVerified
	case freshnessStale:
		return evidenceStale
	case freshnessUnknown:
		return evidenceUnverified
	}
	if declaredDone {
		return evidenceDocumentedOnly
	}
	return evidenceNone
}

func pipelineRunStopped(terminal pipeline.TerminalState) bool {
	switch terminal {
	case pipeline.TerminalBlocked, pipeline.TerminalFailedBudget,
		pipeline.TerminalCancelled, pipeline.TerminalPartialPreserved:
		return true
	default:
		return false
	}
}

// pipelineResumeCommand pins the platform and OMP execution owner the run
// used, so a resume does not switch providers because PATH changed.
func pipelineResumeCommand(specID, platform, owner string) string {
	command := "auto pipeline run " + specID + " --continue"
	if platform != "" {
		command += " --platform " + platform
	}
	if platform == "omp" && owner == pipelineExecutionOwnerOrca {
		command += " --execution-owner " + owner
	}
	return command
}

// recordedExecutionOwner reads the owner the last OMP run of a SPEC used.
func recordedExecutionOwner(stateDir, specID string) string {
	data, err := os.ReadFile(filepath.Join(stateDir, specID+".execution-owner.json"))
	if err != nil {
		return ""
	}
	var receipt pipelineExecutionOwnerReceipt
	if json.Unmarshal(data, &receipt) != nil {
		return ""
	}
	return receipt.Owner
}

// statusSingleLine keeps a receipt string on one bounded line.
func statusSingleLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	const limit = 240
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}
