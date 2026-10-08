// Package orchestra provides the multi-coding CLI orchestration engine.
package orchestra

import (
	"slices"
	"time"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// Strategy는 오케스트레이션 전략이다.
type Strategy string

const (
	StrategyConsensus Strategy = "consensus"
	StrategyPipeline  Strategy = "pipeline"
	StrategyDebate    Strategy = "debate"
	StrategyFastest   Strategy = "fastest"
	StrategyRelay     Strategy = "relay"
	StrategyRecheck   Strategy = "recheck"
)

// IsValid reports whether the fallback policy is supported. The empty value
// preserves the legacy subprocess fallback for callers created before the
// explicit policy field was added.
func (m ReliabilityFallbackMode) IsValid() bool {
	return m == "" || m == FallbackModeSubprocess || m == FallbackModeSkip || m == FallbackModeAbort
}

// UsageCapability describes whether an execution path can expose trustworthy
// provider usage independently from its human-readable output.
type UsageCapability struct {
	Supported bool   `json:"supported"`
	Observed  bool   `json:"observed"`
	Source    string `json:"source,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ValidStrategies는 유효한 전략 목록이다.
var ValidStrategies = []Strategy{StrategyConsensus, StrategyPipeline, StrategyDebate, StrategyFastest, StrategyRelay, StrategyRecheck}

// IsValid는 전략의 유효성을 검증한다.
func (s Strategy) IsValid() bool {
	return slices.Contains(ValidStrategies, s)
}

// ProviderConfig는 ��로바이더 실행 설정이다.
type ProviderConfig struct {
	Name                string        // provider name (claude, codex, gemini)
	Backend             string        // request-specific execution backend
	Model               string        // provider/model selector used by routed backends
	Tools               []string      // tools exposed by the selected backend
	Binary              string        // executable binary path
	ModelFamily         string        // stable model-family identity used for judge separation policy
	Args                []string      // args for non-interactive mode
	ModelPolicy         string        // model selection ownership: quality-managed or user-pinned
	PromptViaArgs       bool          // true: pass prompt as last arg (gemini), false: pass via stdin (claude, codex)
	StartupTimeout      time.Duration // per-provider startup timeout; 0 uses name-based default
	ExecutionTimeout    time.Duration // per-provider execution timeout; 0 uses command/global timeout
	ResultReadyPatterns []string      // non-interactive: semantic output markers that indicate the useful result is complete
	ResultReadyGrace    time.Duration // non-interactive: required output idle window after a ready marker before forced cleanup
	SchemaFlag          string        // subprocess: CLI flag for JSON schema (e.g., "--schema")
	StdinMode           string        // subprocess: prompt delivery — "pipe" (default) or "file"
	OutputFormat        string        // subprocess: expected output — "json" (default) or "text"
	WorkDir             string        // subprocess: process working directory; empty inherits the orchestrator cwd (resolved from OrchestraConfig.ProviderWorkDir)
	SandboxMode         string        // policy-stamped sandbox mode recorded in receipts (read-only, unverified, workspace-write, unrestricted); empty infers from argv
	// UnsetEnv names inherited environment variables (case-insensitive; a
	// trailing * names a prefix) the provider process starts without, on the
	// subprocess path and on routed backends that honor it; empty inherits
	// the whole environment.
	UnsetEnv []string
	// KeepEnv, when not empty, names the only inherited environment
	// variables (exact names, letter case included; a trailing * names a
	// prefix) the provider process starts with on the subprocess path, and
	// UnsetEnv still drops from what it keeps. Routed backends do not read
	// it, so a caller that sets it registers no backend route.
	KeepEnv []string
	// MaxOutputBytes bounds what stdout and stderr each keep while the
	// subprocess runs; later bytes are drained and dropped. Zero uses the
	// package bound of every provider stream (fastFailBufferCap).
	MaxOutputBytes int
	// FastFailPatterns overrides the built-in provider fast-fail rules. When nil,
	// DefaultFastFailRules() is used (behavior identical to the legacy hardcoded set).
	FastFailPatterns []FastFailRule
}

// ReliabilityFallbackMode defines deterministic degradation behavior.
type ReliabilityFallbackMode string

const (
	FallbackModeSubprocess ReliabilityFallbackMode = "subprocess"
	FallbackModeSkip       ReliabilityFallbackMode = "skip"
	FallbackModeAbort      ReliabilityFallbackMode = "abort"
)

// ProviderResponse는 프로바이더 실행 결과이다.
type ProviderResponse struct {
	Provider    string        // 프로바이더 이름
	Output      string        // stdout 출력
	Error       string        // stderr 출력
	Duration    time.Duration // 실행 시간
	ExitCode    int           // 종료 코드
	TimedOut    bool          // 타임아웃 여부
	EmptyOutput bool          // true when stdout is empty (exit 0 but no content)
	Receipt     string        // reliability collection receipt path, if persisted
	// ExecutedBackend records which backend produced this response:
	// "subprocess", "omp" (SPEC-OMP-006 read-only RPC session), or
	// "" / "none" when none succeeded (REQ-005, F-003).
	ExecutedBackend string
	Role            string
	Attempt         int
	ModelFamily     string
	DegradedReasons []string
	TerminalState   string
	Usage           []telemetry.UsageEnvelope `json:"usage,omitempty"`
	UsageCapability UsageCapability           `json:"usage_capability"`
	Execution       *ProviderExecution        `json:"execution,omitempty"` // launch provenance for subprocess-backed attempts
	// @AX:NOTE: [AUTO] internal-only carrier — fresh judge evidence is projected through OrchestraResult, not provider-response JSON
	freshJudgeSession *FreshJudgeSessionEvidence
}

// @AX:ANCHOR: [AUTO] failure diagnostics wire schema shared by CLI JSON output, spec health projection, and yield reports.
// @AX:REASON: [AUTO] JSON field names and timeout/redaction metadata must stay stable for downstream failure summaries and retry hints.
// FailedProvider records a provider that failed during execution.
type FailedProvider struct {
	Name                    string                    `json:"provider"`                            // Provider name
	Role                    string                    `json:"role,omitempty"`                      // role that timed out or failed, when known
	Attempt                 int                       `json:"attempt,omitempty"`                   // one-based role/round attempt
	ModelFamily             string                    `json:"model_family,omitempty"`              // provider model-family identity
	ExecutedBackend         string                    `json:"executed_backend,omitempty"`          // backend that observed the failed attempt
	TerminalState           string                    `json:"terminal_state,omitempty"`            // policy transition observed before or during dispatch
	DegradedReasons         []string                  `json:"degraded_reasons,omitempty"`          // machine-readable policy degradation evidence
	ExitCode                int                       `json:"exit_code,omitempty"`                 // provider exit code when available
	TimedOut                bool                      `json:"timed_out,omitempty"`                 // true when the failed attempt timed out
	Error                   string                    `json:"error"`                               // Error message
	FailureClass            string                    `json:"failure_class"`                       // timeout, capacity_exhausted, rate_limited, binary_or_transport, execution_error
	TimeoutSource           string                    `json:"timeout_source,omitempty"`            // source used to resolve timeout duration
	ConfiguredDuration      time.Duration             `json:"configured_duration,omitempty"`       // configured timeout duration
	ElapsedDuration         time.Duration             `json:"elapsed_duration,omitempty"`          // observed provider duration
	OtherProvidersContinued bool                      `json:"other_providers_continued,omitempty"` // true when a sibling provider completed
	PreflightFailed         bool                      `json:"preflight_failed,omitempty"`          // true when execution stopped before round start
	Receipt                 string                    `json:"receipt,omitempty"`                   // reliability receipt path, if persisted
	NextRemediation         string                    `json:"next_remediation,omitempty"`          // exact next step surfaced in summaries
	CollectionMode          string                    `json:"collection_mode,omitempty"`           // hook, poll, file_ipc, subprocess_stdout
	CorrelationRunID        string                    `json:"correlation_run_id,omitempty"`        // run identifier for artifact lookup
	StderrPreview           string                    `json:"stderr_preview,omitempty"`            // sanitized stderr excerpt for postmortem summaries
	OutputPreview           string                    `json:"output_preview,omitempty"`            // sanitized stdout excerpt for postmortem summaries
	Usage                   []telemetry.UsageEnvelope `json:"usage,omitempty"`
	UsageCapability         UsageCapability           `json:"usage_capability"`
	Execution               *ProviderExecution        `json:"execution,omitempty"` // launch provenance of the failed attempt, when a process started
}

// OrchestraResult는 오케스트레이션 최종 결과이다.
type OrchestraResult struct {
	Strategy        Strategy                  // 사용된 전략
	Responses       []ProviderResponse        // 개별 프로바이더 응답
	Merged          string                    // 병합된 최종 결과
	Duration        time.Duration             // 전체 실행 시간
	Summary         string                    // 전략별 요약 (합의율, 파이프라인 단계 등)
	FailedProviders []FailedProvider          // Providers that failed during execution
	RoundHistory    [][]ProviderResponse      // Per-round provider responses for debate strategy
	RunID           string                    // reliability correlation run ID
	Degraded        bool                      // true when one or more providers were skipped/degraded
	Reliability     *ReliabilitySummary       // persisted receipts / bundle summary
	Usage           []telemetry.UsageEnvelope // deduplicated model-call receipts
	UsageAggregate  telemetry.UsageAggregate  // additive usage summary
	UsageCapability UsageCapability           // aggregate execution-path capability
	Yield           *YieldOutput              // structured round metadata when execution yields to the caller
	RunReceipt      *OrchestrationRunReceipt  `json:"run_receipt,omitempty"`
	Workspace       *WorkspaceEvidence        `json:"workspace,omitempty"` // pre/post provider-execution worktree comparison

	// Additive orchestration_run_receipt.v1 projection. Legacy fields above
	// remain the compatibility API while these fields make execution policy and
	// terminal evidence machine-readable.
	ReceiptSchema          string                          `json:"receipt_schema,omitempty"`
	RequestedStrategy      Strategy                        `json:"requested_strategy,omitempty"`
	EffectiveStrategy      Strategy                        `json:"effective_strategy,omitempty"`
	RequestedProviders     []string                        `json:"requested_providers,omitempty"`
	ConfiguredProviders    []string                        `json:"configured_providers,omitempty"`
	ResolvedProviders      []string                        `json:"resolved_providers,omitempty"`
	AttemptedProviders     []string                        `json:"attempted_providers,omitempty"`
	UsableProviders        []string                        `json:"usable_providers,omitempty"`
	FailedProviderNames    []string                        `json:"failed_providers,omitempty"`
	DegradedReasons        []string                        `json:"degraded_reasons,omitempty"`
	JudgeStatus            string                          `json:"judge_status,omitempty"`
	AnalysisVerdict        string                          `json:"analysis_verdict,omitempty"`
	GateStatus             string                          `json:"gate_status,omitempty"`
	TerminalState          string                          `json:"terminal_state,omitempty"`
	DispatchCount          int                             `json:"dispatch_count"`
	QuorumRequired         int                             `json:"quorum_required,omitempty"`
	QuorumMet              bool                            `json:"quorum_met"`
	QuorumUsable           int                             `json:"quorum_usable"`
	ConsensusMetrics       *ConsensusMetrics               `json:"consensus_metrics,omitempty"`
	Veto                   bool                            `json:"critical_veto,omitempty"`
	JudgeSeparation        *JudgeSeparationEvidence        `json:"judge_separation,omitempty"`
	FreshJudgeSession      *FreshJudgeSessionEvidence      `json:"fresh_judge_session,omitempty"`
	JudgeProviderSelection *JudgeProviderSelectionEvidence `json:"judge_provider_selection,omitempty"`
}

// OrchestraConfig는 오케스트레이션 실행 설정이다.
type OrchestraConfig struct {
	Providers                    []ProviderConfig            // 참여 프로바이더 목록
	ProviderBackends             map[string]ExecutionBackend // provider-specific backends used by direct execution
	RequestedProviders           []string                    // names requested before config/capability resolution
	ConfiguredProviders          []string                    // policy denominator before installation filtering
	Strategy                     Strategy                    // 실행 전략
	Prompt                       string                      // 전달할 프롬프트
	TimeoutSeconds               int                         // 타임아웃 (초)
	JudgeProvider                string                      // debate 전략에서 최종 판정 프로바이더
	InvokingProvider             string                      // canonical provider that invoked the orchestration, when known
	JudgeSelectionSource         string                      // explicit, invoking_provider, or configured_fallback
	JudgeConfig                  *ProviderConfig             // optional judge config when the judge is not a participant
	RequireJudgeFamilySeparation bool                        // fail closed unless required judge uses a known, distinct model family
	DebateRounds                 int                         // Number of debate rounds (1=no rebuttal, 2=with rebuttal). 0 defaults to 1.
	KeepRelayOutput              bool                        // when true, preserve temp relay output files after execution
	ConsensusThreshold           float64                     // consensus threshold (0 uses default 0.66)
	MinimumProviders             int                         // policy floor for quorum; 0 uses configured-provider majority
	MinimumAgreementRatio        float64                     // consensus gate floor on ConsensusMetrics.AgreementRatio; 0 disables the gate
	NoJudge                      bool                        // R4: skip judge verdict phase when true
	ContextAware                 bool                        // R8: when true, skip topic isolation so providers can read project files
	RoundPreset                  string                      // round preset: "fast", "standard", "deep" (for T8)
	WorkingDir                   string                      // requested working directory recorded in reliability receipts and used by routed backends
	ProviderWorkDir              string                      // provider process cwd; empty inherits the orchestrator cwd
	// ReadOnly records that the caller dispatches the providers under the
	// read-only projection. Nothing in this package reads it since the pane
	// launch retired (SPEC-PANERM-001): read-only is enforced before dispatch
	// by the CLI's provider argv projection (applyCommandReadOnlyPolicy,
	// validateReadOnlyProviderArgv). It stays because SPEC-REVIEWRO-001 REQ-17
	// callers and their tests assert that intent on the config.
	ReadOnly     bool
	RunID        string // optional run correlation ID; autogenerated when empty
	FallbackMode ReliabilityFallbackMode
	// ReliabilityStore is initialized internally when reliability artifacts are enabled.
	ReliabilityStore *reliabilityStore
}
