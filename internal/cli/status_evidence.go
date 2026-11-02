package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

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
	// stateRoot is the absolute directory whose pipeline state holds this
	// evidence; pipeline commands for the SPEC run there.
	stateRoot string
}

// statusExecution is the last pipeline run of a SPEC as its receipt records it.
type statusExecution struct {
	Source               string                `json:"source"`
	Root                 string                `json:"root"`
	OtherRoots           []string              `json:"other_roots,omitempty"`
	Error                string                `json:"error,omitempty"`
	RunID                string                `json:"run_id,omitempty"`
	TaskID               string                `json:"task_id,omitempty"`
	Attempt              int                   `json:"attempt,omitempty"`
	ResumedFrom          string                `json:"resumed_from,omitempty"`
	Platform             string                `json:"platform,omitempty"`
	ExecutionOwner       string                `json:"execution_owner,omitempty"`
	Route                string                `json:"route,omitempty"`
	Terminal             string                `json:"terminal"`
	StopReason           string                `json:"stop_reason,omitempty"`
	Blocker              string                `json:"blocker,omitempty"`
	BaseHead             string                `json:"base_head,omitempty"`
	FinishedAt           string                `json:"finished_at,omitempty"`
	Phases               pipelinePhaseCounts   `json:"phases"`
	Dispatches           int                   `json:"dispatches"`
	CumulativeDispatches int                   `json:"cumulative_dispatches"`
	Resume               *statusResume         `json:"resume,omitempty"`
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
	baseDir   string
	installed map[string]bool
	idCount   map[string]int
	heads     map[string]gitHeadResult
	trees     treeDigestCache
}

type gitHeadResult struct {
	head string
	err  error
}

func newStatusEvidenceReader(baseDir string, specs []specEntry, env statusEnvironment) *statusEvidenceReader {
	reader := &statusEvidenceReader{
		baseDir: statusAbsDir(baseDir), installed: map[string]bool{}, idCount: map[string]int{},
		heads: map[string]gitHeadResult{}, trees: treeDigestCache{},
	}
	for _, name := range env.InstalledCLIs {
		reader.installed[name] = true
	}
	for _, entry := range specs {
		reader.idCount[entry.id]++
	}
	return reader
}

func (r *statusEvidenceReader) read(entry specEntry) statusEvidence {
	specDir := statusAbsDir(filepath.Dir(entry.path))
	chosen, others := r.stateRoots(entry)
	evidence := statusEvidence{stateRoot: chosen.root}
	var latest *pipeline.OrchestrationRunReceipt
	switch {
	case chosen.checkpoint != nil:
		evidence.Execution = r.execution(chosen, entry.id, specDir)
		if receipt := chosen.checkpoint.Receipt; receipt != nil && receipt.Terminal != pipeline.TerminalDryRun {
			latest = receipt
		}
	case chosen.loadErr != nil:
		evidence.Execution = &statusExecution{
			Source: relativeStatusPath(r.baseDir, chosen.checkpointPath()), Root: chosen.root,
			Error: statusSingleLine("checkpoint unreadable: " + chosen.loadErr.Error()),
		}
	}
	if evidence.Execution != nil {
		evidence.Execution.OtherRoots = others
	}
	evidence.Verification = checkPipelineVerification(chosen.root, chosen.stateDir(), entry.id, specDir, latest, r.trees)
	evidence.Review = readStatusReview(r.baseDir, specDir)
	evidence.Label = evidenceLabel(entry.isDone(), evidence.Execution, evidence.Verification)
	return evidence
}

func (r *statusEvidenceReader) execution(chosen stateRootCandidate, specID, specDir string) *statusExecution {
	cp := chosen.checkpoint
	execution := &statusExecution{
		Source: relativeStatusPath(r.baseDir, chosen.checkpointPath()), Root: chosen.root,
		BaseHead: statusSingleLine(cp.GitCommitHash),
	}
	receipt := cp.Receipt
	if receipt == nil {
		// A checkpoint without a receipt records no terminal state.
		return execution
	}
	execution.RunID, execution.TaskID = statusSingleLine(receipt.RunID), statusSingleLine(receipt.TaskID)
	execution.ResumedFrom = statusSingleLine(receipt.ResumedFrom)
	execution.Attempt = len(receipt.PriorAttempts) + 1
	if len(receipt.ResolvedProviders) > 0 {
		execution.Platform = statusSingleLine(receipt.ResolvedProviders[0])
	}
	if execution.Platform == "omp" {
		execution.ExecutionOwner = recordedExecutionOwner(receipt)
	}
	execution.Route = string(pipeline.NormalizeRoute(receipt.RoutePhaseSet))
	execution.Terminal = statusSingleLine(string(receipt.Terminal))
	execution.Blocker = statusSingleLine(receipt.Blocker)
	if receipt.FinishedAt != nil {
		execution.FinishedAt = receipt.FinishedAt.UTC().Format(time.RFC3339)
	}
	execution.Phases = countPipelinePhases(*receipt)
	execution.Dispatches, execution.CumulativeDispatches = receipt.DispatchCount, receipt.CumulativeDispatchCount
	if pipelineRunStopped(receipt.Terminal) {
		execution.StopReason = pipelineStopReason(*receipt)
		execution.Resume = r.assessResume(chosen.root, specID, specDir, cp, execution)
	}
	execution.BlockedAttempt = r.blockedAttempt(chosen)
	return execution
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
func (r *statusEvidenceReader) blockedAttempt(chosen stateRootCandidate) *statusBlockedAttempt {
	path := filepath.Join(chosen.stateDir(), chosen.specID+".blocked.yaml")
	sidecar, err := os.Stat(path)
	if err != nil {
		return nil
	}
	canonical, err := os.Stat(chosen.checkpointPath())
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

// statusSingleLine makes a string from a receipt or the file system safe to
// show: control characters become spaces, whitespace collapses, and the
// result stays on one bounded line.
func statusSingleLine(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	const limit = 240
	if runes := []rune(text); len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

// statusAbsDir is dir made absolute; status names directories so commands
// can run from anywhere.
func statusAbsDir(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}
