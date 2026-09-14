package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/workerreceipt"
)

const (
	// PipelineRouteVersion identifies the executable five-phase route.
	PipelineRouteVersion = "pipeline-route.v1"
	// OrchestrationRunReceiptVersion identifies the persisted run receipt schema.
	OrchestrationRunReceiptVersion = "orchestration_run_receipt.v1"
)

// TerminalState is the single terminal outcome of a pipeline run.
type TerminalState string

const (
	TerminalCompleted        TerminalState = "completed"
	TerminalBlocked          TerminalState = "blocked"
	TerminalFailedBudget     TerminalState = "failed_budget"
	TerminalCancelled        TerminalState = "cancelled"
	TerminalPartialPreserved TerminalState = "partial_preserved"
	TerminalDryRun           TerminalState = "dry_run"
)

// PhaseTransition records one atomic phase status transition.
type PhaseTransition struct {
	Sequence        int              `json:"sequence" yaml:"sequence"`
	PhaseID         PhaseID          `json:"phase_id,omitempty" yaml:"phase_id,omitempty"`
	Status          CheckpointStatus `json:"status,omitempty" yaml:"status,omitempty"`
	Attempt         int              `json:"attempt,omitempty" yaml:"attempt,omitempty"`
	Verdict         GateVerdict      `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Reason          string           `json:"reason,omitempty" yaml:"reason,omitempty"`
	State           string           `json:"state,omitempty" yaml:"state,omitempty"`
	AnalysisVerdict string           `json:"analysis_verdict,omitempty" yaml:"analysis_verdict,omitempty"`
	GateStatus      string           `json:"gate_status,omitempty" yaml:"gate_status,omitempty"`
}

// ProviderRunReceipt records one observed phase backend dispatch.
type ProviderRunReceipt struct {
	Provider     string `json:"provider" yaml:"provider"`
	Role         string `json:"role" yaml:"role"`
	Attempt      int    `json:"attempt" yaml:"attempt"`
	Backend      string `json:"backend" yaml:"backend"`
	ExitCode     int    `json:"exit_code" yaml:"exit_code"`
	TimedOut     bool   `json:"timed_out" yaml:"timed_out"`
	Usable       bool   `json:"usable" yaml:"usable"`
	FailureClass string `json:"failure_class,omitempty" yaml:"failure_class,omitempty"`
	Artifact     string `json:"artifact,omitempty" yaml:"artifact,omitempty"`
}

// WorkerRunReceipt is the canonical multi-agent worker handoff shape.
type WorkerRunReceipt = workerreceipt.Receipt

// PhaseReceipt is the final observed state for one pipeline phase.
type PhaseReceipt struct {
	PhaseID  PhaseID          `json:"phase_id" yaml:"phase_id"`
	Status   CheckpointStatus `json:"status" yaml:"status"`
	Attempts int              `json:"attempts" yaml:"attempts"`
	Gate     GateType         `json:"gate" yaml:"gate"`
	Verdict  GateVerdict      `json:"verdict,omitempty" yaml:"verdict,omitempty"`
	Error    string           `json:"error,omitempty" yaml:"error,omitempty"`
}

// OrchestrationRunReceipt is the versioned evidence envelope for a run.
type OrchestrationRunReceipt struct {
	SchemaVersion       string               `json:"schema" yaml:"schema"`
	RunID               string               `json:"run_id" yaml:"run_id"`
	RouteID             string               `json:"route_id" yaml:"route_id"`
	RouteVersion        string               `json:"route_version" yaml:"route_version"`
	RoutePhaseSet       PhaseRoute           `json:"route_phase_set" yaml:"route_phase_set"`
	SpecID              string               `json:"spec_id" yaml:"spec_id"`
	RequestedStrategy   Strategy             `json:"requested_strategy" yaml:"requested_strategy"`
	EffectiveStrategy   Strategy             `json:"effective_strategy" yaml:"effective_strategy"`
	RequestedProviders  []string             `json:"requested_providers" yaml:"requested_providers"`
	ConfiguredProviders []string             `json:"configured_providers" yaml:"configured_providers"`
	ResolvedProviders   []string             `json:"resolved_providers" yaml:"resolved_providers"`
	AttemptedProviders  []string             `json:"attempted_providers" yaml:"attempted_providers"`
	UsableProviders     []string             `json:"usable_providers" yaml:"usable_providers"`
	FailedProviders     []string             `json:"failed_providers" yaml:"failed_providers"`
	DegradedReasons     []string             `json:"degraded_reasons" yaml:"degraded_reasons"`
	CriticalVeto        bool                 `json:"critical_veto" yaml:"critical_veto"`
	DispatchCount       int                  `json:"dispatch_count" yaml:"dispatch_count"`
	CompletedPhaseCount int                  `json:"completed_phase_count" yaml:"completed_phase_count"`
	AnalysisVerdict     GateVerdict          `json:"analysis_verdict" yaml:"analysis_verdict"`
	GateStatus          string               `json:"gate_status" yaml:"gate_status"`
	JudgeStatus         string               `json:"judge_status" yaml:"judge_status"`
	QuorumRequired      int                  `json:"quorum_required" yaml:"quorum_required"`
	QuorumMet           bool                 `json:"quorum_met" yaml:"quorum_met"`
	Attempts            int                  `json:"attempts" yaml:"attempts"`
	ProviderReceipts    []ProviderRunReceipt `json:"provider_receipts" yaml:"provider_receipts"`
	WorkerReceipts      []WorkerRunReceipt   `json:"worker_receipts" yaml:"worker_receipts"`
	Artifacts           []string             `json:"artifacts" yaml:"artifacts"`
	Phases              []PhaseReceipt       `json:"phases" yaml:"phases"`
	Transitions         []PhaseTransition    `json:"transitions" yaml:"transitions"`
	Terminal            TerminalState        `json:"terminal_state" yaml:"terminal_state"`
	Blocker             string               `json:"blocker,omitempty" yaml:"blocker,omitempty"`
	StartedAt           time.Time            `json:"started_at" yaml:"started_at"`
	FinishedAt          *time.Time           `json:"finished_at,omitempty" yaml:"finished_at,omitempty"`
}

func newRunReceipt(specID string, requested, effective Strategy, route PhaseRoute, phases []Phase) OrchestrationRunReceipt {
	receipt := OrchestrationRunReceipt{
		SchemaVersion:       OrchestrationRunReceiptVersion,
		RunID:               newRunID(),
		RouteID:             "pipeline",
		RouteVersion:        PipelineRouteVersion,
		RoutePhaseSet:       route,
		SpecID:              specID,
		RequestedStrategy:   requested,
		EffectiveStrategy:   effective,
		GateStatus:          "pending",
		JudgeStatus:         "not_requested",
		StartedAt:           time.Now().UTC(),
		Phases:              make([]PhaseReceipt, len(phases)),
		Transitions:         []PhaseTransition{},
		RequestedProviders:  []string{},
		ConfiguredProviders: []string{},
		ResolvedProviders:   []string{},
		AttemptedProviders:  []string{},
		UsableProviders:     []string{},
		FailedProviders:     []string{},
		DegradedReasons:     []string{},
		ProviderReceipts:    []ProviderRunReceipt{},
		WorkerReceipts:      []WorkerRunReceipt{},
		Artifacts:           []string{},
	}
	for i, phase := range phases {
		receipt.Phases[i] = PhaseReceipt{PhaseID: phase.ID, Status: CheckpointStatusPending, Gate: phase.Gate}
	}
	return receipt
}

// NewBlockedRunReceipt creates a terminal preflight receipt without dispatches.
func NewBlockedRunReceipt(specID string, requested Strategy, blocker string) OrchestrationRunReceipt {
	requested, effective, _ := effectivePipelineStrategy(requested)
	receipt := newRunReceipt(specID, requested, effective, RouteFull, DefaultPhases())
	receipt.finish(TerminalBlocked, blocker)
	return receipt
}

func (r *OrchestrationRunReceipt) configureProvider(provider string) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return
	}
	r.RequestedProviders = appendUnique(r.RequestedProviders, provider)
	r.ConfiguredProviders = appendUnique(r.ConfiguredProviders, provider)
	r.ResolvedProviders = appendUnique(r.ResolvedProviders, provider)
	r.QuorumRequired = 1
}

func (r *OrchestrationRunReceipt) recordDispatch(providerFallback string, phaseID PhaseID, attempt int, resp *PhaseResponse, runErr error) {
	provider, backend, role := providerFallback, "phase_backend", string(phaseID)
	receipt := ProviderRunReceipt{Provider: provider, Backend: backend, Role: role, Attempt: attempt}
	if resp != nil {
		if resp.Provider != "" {
			receipt.Provider = resp.Provider
		}
		if resp.Backend != "" {
			receipt.Backend = resp.Backend
		}
		if resp.Role != "" {
			receipt.Role = resp.Role
		}
		receipt.ExitCode = resp.ExitCode
		receipt.TimedOut = resp.TimedOut
		receipt.FailureClass = resp.FailureClass
		receipt.Artifact = resp.Artifact
		receipt.Usable = runErr == nil && !resp.TimedOut && resp.ExitCode == 0 && strings.TrimSpace(resp.Output) != ""
	}
	if !receipt.Usable && receipt.FailureClass == "" {
		switch {
		case resp != nil && resp.TimedOut:
			receipt.FailureClass = "timeout"
		case resp != nil && strings.TrimSpace(resp.Output) == "":
			receipt.FailureClass = "empty_output"
		case resp != nil && resp.ExitCode != 0:
			receipt.FailureClass = "execution_error"
		case runErr != nil:
			receipt.FailureClass = "execution_error"
		default:
			receipt.FailureClass = "empty_response"
		}
	}
	r.ProviderReceipts = append(r.ProviderReceipts, receipt)
	r.AttemptedProviders = appendUnique(r.AttemptedProviders, receipt.Provider)
	if receipt.Usable {
		r.UsableProviders = appendUnique(r.UsableProviders, receipt.Provider)
	} else {
		r.FailedProviders = appendUnique(r.FailedProviders, receipt.Provider)
		r.DegradedReasons = appendUnique(r.DegradedReasons, "provider_failure")
	}
	if receipt.Artifact != "" {
		r.Artifacts = appendUnique(r.Artifacts, receipt.Artifact)
	}
	if attempt > r.Attempts {
		r.Attempts = attempt
	}
	r.QuorumMet = r.QuorumRequired > 0 && len(r.UsableProviders) >= r.QuorumRequired
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func newRunID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "pipeline-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("pipeline-%d", time.Now().UTC().UnixNano())
}

func (r *OrchestrationRunReceipt) transition(phaseID PhaseID, status CheckpointStatus, attempt int, verdict GateVerdict, reason string) {
	r.Transitions = append(r.Transitions, PhaseTransition{
		Sequence: len(r.Transitions) + 1,
		PhaseID:  phaseID,
		Status:   status,
		Attempt:  attempt,
		Verdict:  verdict,
		Reason:   reason,
	})
	for i := range r.Phases {
		if r.Phases[i].PhaseID != phaseID {
			continue
		}
		r.Phases[i].Status = status
		if attempt > r.Phases[i].Attempts {
			r.Phases[i].Attempts = attempt
		}
		r.Phases[i].Verdict = verdict
		r.Phases[i].Error = reason
		break
	}
}

func (r *OrchestrationRunReceipt) finish(terminal TerminalState, blocker string) {
	now := time.Now().UTC()
	r.Terminal = terminal
	r.Blocker = blocker
	r.FinishedAt = &now
	switch terminal {
	case TerminalCompleted, TerminalDryRun:
		r.GateStatus = "passed"
		r.AnalysisVerdict = VerdictPass
	default:
		r.GateStatus = "blocked"
		r.AnalysisVerdict = VerdictFail
	}
	transitions := make([]PhaseTransition, 0, len(r.Transitions)+1)
	for _, transition := range r.Transitions {
		if transition.State == "" {
			transitions = append(transitions, transition)
		}
	}
	r.Transitions = append(transitions, PhaseTransition{
		State: string(r.Terminal), AnalysisVerdict: string(r.AnalysisVerdict), GateStatus: r.GateStatus,
	})
	for i := range r.Transitions {
		r.Transitions[i].Sequence = i + 1
	}
}
