// Package healthband holds the pure core of `auto react band`
// (SPEC-SIGMABAND-001): metric observations, the block-based mean ± σ
// detector, the bounded append-only store, and the untrusted-input
// sanitizer. Subprocess calls live in internal/cli behind fakeable seams.
package healthband

import (
	"time"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Schemas of the store files (Data Contracts).
const (
	SchemaObservation    = "autopus.metric_observation.v1"
	SchemaBandEvaluation = "autopus.band_evaluation.v1"
	SchemaBandState      = "autopus.band_state.v1"
)

// Series ID prefixes and observation sources.
const (
	SeriesPrefixCI     = "ci.failure_rate:"
	SeriesPrefixCanary = "canary.failure_rate:"
	SourceGH           = "gh"
	SourceCanary       = "canary"
)

// Observation is one metric sample (Detector Contract item 1). Value is 0 or
// 1 (1 = failure). The order key is (ObservedAt, Tiebreak) ascending: CI uses
// createdAt and the numeric run_id, canary a nanosecond time and the
// store-global sequence of its c<sequence> sample key.
type Observation struct {
	Schema     string    `json:"schema"`
	Series     string    `json:"series"`
	SampleKey  string    `json:"sample_key"`
	ObservedAt time.Time `json:"observed_at"`
	Tiebreak   int64     `json:"tiebreak"`
	Value      float64   `json:"value"`
	Attempt    int       `json:"attempt"`
	Source     string    `json:"source"`
}

// Detector constants are fixed in v1 (Detector Contract item 9).
const (
	BlockSize      = 4    // K: observations per block
	BaselineWindow = 30   // W: most baseline blocks before the current block
	MinBaseline    = 20   // N_min: fewer baseline blocks give insufficient_samples
	VarianceFloor  = 0.25 // sd_eff = max(sd, 1/K)
	TierEpsilon    = 1e-9 // tier boundaries belong to the upper tier
	maxTier        = 3
)

// Constants is the constants record stored in every evaluation event.
type Constants struct {
	K     int     `json:"k"`
	W     int     `json:"w"`
	NMin  int     `json:"n_min"`
	Floor float64 `json:"floor"`
	Eps   float64 `json:"eps"`
}

// DefaultConstants returns the fixed v1 constants.
func DefaultConstants() Constants {
	return Constants{K: BlockSize, W: BaselineWindow, NMin: MinBaseline, Floor: VarianceFloor, Eps: TierEpsilon}
}

// Reason codes. Events, state, envelopes, and text rows carry these codes,
// numbers, filtered IDs, and manifest hashes only (Untrusted Input item 9).
const (
	// Detector.
	ReasonNoCurrentBlock       = "no_current_block"
	ReasonInsufficientSamples  = "insufficient_samples"
	ReasonZeroVariance         = "zero_variance"
	ReasonVarianceFloorApplied = "variance_floor_applied"
	ReasonBelowBaseline        = "below_baseline"
	ReasonLateObservation      = "late_observation"
	// Store read and ingest.
	ReasonMalformed           = "malformed"
	ReasonUnknownSchema       = "unknown_schema"
	ReasonInvalidValue        = "invalid_value"
	ReasonNoChecksExecuted    = "no_checks_executed"
	ReasonIdentifierSanitized = "identifier_sanitized"
	ReasonStoreLocked         = "store_locked"
	ReasonSeriesNotFound      = "series_not_found"
	// CI source (REQ-05).
	ReasonGHMissing            = "gh_missing"
	ReasonGHUnauthenticated    = "gh_unauthenticated"
	ReasonGHFetchFailed        = "gh_fetch_failed"
	ReasonNoRemote             = "no_remote"
	ReasonRemoteNotGitHub      = "remote_not_github"
	ReasonDefaultBranchUnknown = "default_branch_unknown"
	// Episodes and claims (Decision Table, Durability Protocol).
	ReasonEpisodeClosed           = "episode_closed"
	ReasonEpisodeAlreadyDiagnosed = "episode_already_diagnosed"
	ReasonSupersededInBatch       = "superseded_in_batch"
	ReasonLateResult              = "late_result"
	ReasonClaimUnknown            = "claim_unknown"
	// BS writer.
	ReasonBSLockTimeout = "bs_lock_timeout"
	ReasonBSIDExhausted = "bs_id_exhausted"
	// Untrusted input; the values equal the promptlayer invalidation reasons.
	ReasonInjectionRisk = promptlayer.InvalidationInjectionRisk
	ReasonSecretRisk    = promptlayer.InvalidationSecretRisk
	ReasonSizeCap       = promptlayer.InvalidationSizeCap
)

// Evaluation is one detector result. N and X are present only when a current
// block exists; Mu, SD, SDEff, Z, and Tier only when N ≥ N_min. Absent values
// stay nil and are omitted from JSON, never written as null or a placeholder.
type Evaluation struct {
	Series    string     `json:"series"`
	SampleKey string     `json:"sample_key"`
	N         *int       `json:"n,omitempty"`
	X         *float64   `json:"x,omitempty"`
	Mu        *float64   `json:"mu,omitempty"`
	SD        *float64   `json:"sd,omitempty"`
	SDEff     *float64   `json:"sd_eff,omitempty"`
	Z         *float64   `json:"z,omitempty"`
	Tier      *int       `json:"tier,omitempty"`
	Reasons   []string   `json:"reasons,omitempty"`
	Constants *Constants `json:"constants,omitempty"`
}

// Event kinds and the three-value action enum (Decision Table).
const (
	EventKindEvaluation   = "evaluation"
	EventKindActionResult = "action_result"
	ActionLog             = "log"
	ActionDiagnose        = "diagnose"
	ActionSuppressed      = "suppressed"
)

// Claim kinds and statuses. A failed claim's status is ClaimFailedPrefix
// followed by its reason, for example "failed:bs_lock_timeout".
const (
	ClaimKindDiagnose = "diagnose"
	ClaimClaimed      = "claimed"
	ClaimDone         = "done"
	ClaimInterrupted  = "interrupted"
	ClaimFailedPrefix = "failed:"
)

// Claim is one due action owned by one run. Events carry id, kind, owner,
// and lease; the checkpoint adds status and, once interrupted, its time.
type Claim struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status,omitempty"`
	Owner         string     `json:"owner"`
	LeaseUntil    time.Time  `json:"lease_until"`
	InterruptedAt *time.Time `json:"interrupted_at,omitempty"`
}

// Event is one band-events.jsonl line. Evaluation fields are inlined; an
// action_result event adds the claim result fields. Empty fields are omitted.
type Event struct {
	Schema string `json:"schema"`
	Seq    int64  `json:"seq"`
	Kind   string `json:"kind"`
	Evaluation
	Action          string                      `json:"action,omitempty"`
	EpisodeID       string                      `json:"episode_id,omitempty"`
	MaxTier         *int                        `json:"max_tier,omitempty"`
	Claims          []Claim                     `json:"claims,omitempty"`
	ClaimID         string                      `json:"claim_id,omitempty"`
	DiagnosisStatus string                      `json:"diagnosis_status,omitempty"`
	BSID            string                      `json:"bs_id,omitempty"`
	BSStatus        string                      `json:"bs_status,omitempty"`
	PromptManifest  []promptlayer.ManifestEntry `json:"prompt_manifest,omitempty"`
}

// Episode is one anomaly episode of a series; its id is "e" plus the
// opening sample key.
type Episode struct {
	ID      string  `json:"id"`
	Open    bool    `json:"open"`
	MaxTier int     `json:"max_tier"`
	BSID    string  `json:"bs_id,omitempty"`
	Claims  []Claim `json:"claims,omitempty"`
}

// SeriesState is the per-series checkpoint.
type SeriesState struct {
	LastKey  string    `json:"last_key"`
	Episodes []Episode `json:"episodes,omitempty"`
}

// Checkpoint is band-state.json. Series is a map, so encoding/json writes its
// keys sorted and equal state always yields equal bytes.
type Checkpoint struct {
	Schema  string                 `json:"schema"`
	LastSeq int64                  `json:"last_seq"`
	Series  map[string]SeriesState `json:"series,omitempty"`
}
