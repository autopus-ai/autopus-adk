package orchestra

import "strings"

const OrchestraRouteVersion = "orchestra-route.v1"

// noneBackendMarker records that no backend produced a usable response for a
// provider (REQ-013).
const noneBackendMarker = "none"

// OrchestrationTransition records the authoritative terminal projection for a run.
type OrchestrationTransition struct {
	Sequence        int    `json:"sequence"`
	State           string `json:"state"`
	AnalysisVerdict string `json:"analysis_verdict"`
	GateStatus      string `json:"gate_status"`
}

// ProviderRunReceipt records one observed provider or judge dispatch outcome.
// The command/cwd/pid/sandbox/timestamp fields are present only for attempts
// that started a provider process, so receipts of routed backends stay unchanged.
type ProviderRunReceipt struct {
	Provider     string   `json:"provider"`
	Role         string   `json:"role"`
	Attempt      int      `json:"attempt"`
	Backend      string   `json:"backend"`
	ModelFamily  string   `json:"model_family,omitempty"`
	ExitCode     int      `json:"exit_code"`
	TimedOut     bool     `json:"timed_out"`
	Usable       bool     `json:"usable"`
	FailureClass string   `json:"failure_class,omitempty"`
	Artifact     string   `json:"artifact,omitempty"`
	Command      []string `json:"command,omitempty"`
	Cwd          string   `json:"cwd,omitempty"`
	PID          int      `json:"pid,omitempty"`
	SandboxMode  string   `json:"sandbox_mode,omitempty"`
	StartedAt    string   `json:"started_at,omitempty"`
	EndedAt      string   `json:"ended_at,omitempty"`
}

// WorkerRunReceipt is the common multi-agent handoff shape used by platform adapters.
type WorkerRunReceipt struct {
	OwnedPaths       []string `json:"owned_paths"`
	ChangedFiles     []string `json:"changed_files"`
	Verification     []string `json:"verification"`
	Blockers         []string `json:"blockers"`
	NextRequiredStep string   `json:"next_required_step"`
}

// OrchestrationRunReceipt is the canonical machine-readable run evidence.
// @AX:ANCHOR: [AUTO] versioned JSON receipt contract consumed by CLI artifacts and downstream orchestration evidence readers
// @AX:REASON: [AUTO] field names and additive evidence projections must remain compatible with orchestration_run_receipt.v1
type OrchestrationRunReceipt struct {
	Schema                 string                          `json:"schema"`
	RunID                  string                          `json:"run_id"`
	RouteID                string                          `json:"route_id"`
	RouteVersion           string                          `json:"route_version"`
	RequestedStrategy      Strategy                        `json:"requested_strategy"`
	EffectiveStrategy      Strategy                        `json:"effective_strategy"`
	RequestedProviders     []string                        `json:"requested_providers"`
	ConfiguredProviders    []string                        `json:"configured_providers"`
	ResolvedProviders      []string                        `json:"resolved_providers"`
	AttemptedProviders     []string                        `json:"attempted_providers"`
	UsableProviders        []string                        `json:"usable_providers"`
	FailedProviders        []string                        `json:"failed_providers"`
	DegradedReasons        []string                        `json:"degraded_reasons"`
	CriticalVeto           bool                            `json:"critical_veto"`
	AnalysisVerdict        string                          `json:"analysis_verdict"`
	GateStatus             string                          `json:"gate_status"`
	TerminalState          string                          `json:"terminal_state"`
	JudgeStatus            string                          `json:"judge_status"`
	DispatchCount          int                             `json:"dispatch_count"`
	QuorumRequired         int                             `json:"quorum_required"`
	QuorumMet              bool                            `json:"quorum_met"`
	QuorumUsable           int                             `json:"quorum_usable"`
	Attempts               int                             `json:"attempts"`
	ConsensusMetrics       *ConsensusMetrics               `json:"consensus_metrics,omitempty"`
	JudgeSeparation        *JudgeSeparationEvidence        `json:"judge_separation,omitempty"`
	FreshJudgeSession      *FreshJudgeSessionEvidence      `json:"fresh_judge_session,omitempty"`
	JudgeProviderSelection *JudgeProviderSelectionEvidence `json:"judge_provider_selection,omitempty"`
	ProviderReceipts       []ProviderRunReceipt            `json:"provider_receipts"`
	WorkerReceipts         []WorkerRunReceipt              `json:"worker_receipts"`
	Artifacts              []string                        `json:"artifacts"`
	Transitions            []OrchestrationTransition       `json:"transitions"`
	Workspace              *WorkspaceEvidence              `json:"workspace,omitempty"`
}

func refreshOrchestrationRunReceipt(result *OrchestraResult) {
	providerReceipts, attempts := buildProviderRunReceipts(result)
	result.RunReceipt = &OrchestrationRunReceipt{
		Schema:                 OrchestrationReceiptSchema,
		RunID:                  result.RunID,
		RouteID:                "orchestra:" + string(result.EffectiveStrategy),
		RouteVersion:           OrchestraRouteVersion,
		RequestedStrategy:      result.RequestedStrategy,
		EffectiveStrategy:      result.EffectiveStrategy,
		RequestedProviders:     cloneProviderNames(result.RequestedProviders),
		ConfiguredProviders:    cloneProviderNames(result.ConfiguredProviders),
		ResolvedProviders:      cloneProviderNames(result.ResolvedProviders),
		AttemptedProviders:     cloneProviderNames(result.AttemptedProviders),
		UsableProviders:        cloneProviderNames(result.UsableProviders),
		FailedProviders:        cloneProviderNames(result.FailedProviderNames),
		DegradedReasons:        cloneProviderNames(result.DegradedReasons),
		CriticalVeto:           result.Veto,
		AnalysisVerdict:        result.AnalysisVerdict,
		GateStatus:             result.GateStatus,
		TerminalState:          result.TerminalState,
		JudgeStatus:            result.JudgeStatus,
		DispatchCount:          result.DispatchCount,
		QuorumRequired:         result.QuorumRequired,
		QuorumMet:              result.QuorumMet,
		QuorumUsable:           result.QuorumUsable,
		Attempts:               attempts,
		ConsensusMetrics:       result.ConsensusMetrics,
		JudgeSeparation:        result.JudgeSeparation,
		FreshJudgeSession:      result.FreshJudgeSession,
		JudgeProviderSelection: result.JudgeProviderSelection,
		ProviderReceipts:       providerReceipts,
		WorkerReceipts:         []WorkerRunReceipt{},
		Artifacts:              collectRunArtifacts(result),
		Workspace:              result.Workspace,
		Transitions: []OrchestrationTransition{{
			Sequence: 1, State: result.TerminalState,
			AnalysisVerdict: result.AnalysisVerdict, GateStatus: result.GateStatus,
		}},
	}
}

// @AX:WARN: [AUTO] high-branch receipt projection — strategy, role, attempt, and failure paths converge here
// @AX:REASON: [AUTO] more than eight conditional branches determine externally visible provider receipt evidence
func buildProviderRunReceipts(result *OrchestraResult) ([]ProviderRunReceipt, int) {
	receipts := make([]ProviderRunReceipt, 0, result.DispatchCount)
	maxAttempt := 0
	if len(result.RoundHistory) > 0 {
		for roundIndex, responses := range result.RoundHistory {
			for _, response := range responses {
				if !responseWasDispatched(response) {
					continue
				}
				attempt := response.Attempt
				if attempt == 0 {
					attempt = roundIndex + 1
				}
				role := response.Role
				if role == "" {
					role = "participant"
					if result.EffectiveStrategy == StrategyDebate {
						role = "debater_r1"
						if attempt > 1 {
							role = "debater_r2"
						}
					}
				}
				receipts = append(receipts, providerRunReceipt(response, role, attempt, result.FailedProviders))
				if attempt > maxAttempt {
					maxAttempt = attempt
				}
			}
		}
		for _, response := range result.Responses {
			if strings.HasSuffix(response.Provider, " (judge)") && responseWasDispatched(response) {
				attempt := response.Attempt
				if attempt == 0 {
					attempt = 1
				}
				receipts = append(receipts, providerRunReceipt(response, "judge", attempt, result.FailedProviders))
				if attempt > maxAttempt {
					maxAttempt = attempt
				}
			}
		}
	} else {
		for _, response := range result.Responses {
			if !responseWasDispatched(response) {
				continue
			}
			role := "participant"
			if strings.HasSuffix(response.Provider, " (judge)") {
				role = "judge"
			}
			if response.Role != "" {
				role = response.Role
			}
			attempt := response.Attempt
			if attempt == 0 {
				attempt = 1
			}
			receipts = append(receipts, providerRunReceipt(response, role, attempt, result.FailedProviders))
			if attempt > maxAttempt {
				maxAttempt = attempt
			}
		}
		if len(receipts) > 0 {
			maxAttempt = 1
		}
	}
	for _, failed := range result.FailedProviders {
		if failed.PreflightFailed {
			continue
		}
		role := failed.Role
		if role == "" {
			role = "participant"
		}
		attempt := failed.Attempt
		if attempt == 0 {
			attempt = 1
		}
		if hasFailedReceipt(receipts, failed.Name, role, attempt) {
			continue
		}
		receipts = append(receipts, failedProviderRunReceipt(failed, role, attempt))
		if attempt > maxAttempt {
			maxAttempt = attempt
		}
	}
	for _, provider := range result.AttemptedProviders {
		if hasProviderReceipt(receipts, provider) {
			continue
		}
		receipts = append(receipts, ProviderRunReceipt{
			Provider: provider, Role: "participant", Attempt: 1,
			Usable: false, FailureClass: "strategy_cancelled",
		})
		if maxAttempt == 0 {
			maxAttempt = 1
		}
	}
	return receipts, maxAttempt
}

func responseWasDispatched(response ProviderResponse) bool {
	return response.ExecutedBackend != noneBackendMarker ||
		(response.TerminalState != TerminalSkipped && response.TerminalState != TerminalBlocked)
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func collectRunArtifacts(result *OrchestraResult) []string {
	var artifacts []string
	if result.Reliability != nil {
		artifacts = appendUniqueName(artifacts, result.Reliability.ArtifactDir)
	}
	for _, response := range result.Responses {
		artifacts = appendUniqueName(artifacts, response.Receipt)
	}
	for _, failed := range result.FailedProviders {
		artifacts = appendUniqueName(artifacts, failed.Receipt)
	}
	return artifacts
}
