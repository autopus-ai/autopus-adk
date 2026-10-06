// Package healthband holds the pure core of `auto react band`
// (SPEC-SIGMABAND-001): metric observations, the block-based mean ± σ
// detector, the bounded append-only store, and the untrusted-input
// sanitizer. Subprocess calls live in internal/cli behind fakeable seams.
package healthband

import "time"

// SchemaObservation is the schema of every metric observation line.
const SchemaObservation = "autopus.metric_observation.v1"

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

// Detector reason codes.
const (
	ReasonNoCurrentBlock       = "no_current_block"
	ReasonInsufficientSamples  = "insufficient_samples"
	ReasonZeroVariance         = "zero_variance"
	ReasonVarianceFloorApplied = "variance_floor_applied"
	ReasonBelowBaseline        = "below_baseline"
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
