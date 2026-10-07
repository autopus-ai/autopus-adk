package orchestra

import "time"

const reliabilitySchemaVersion = "orch-reliability/v1"

// CorrelationIDs ties receipts, events, and bundles to one execution attempt.
type CorrelationIDs struct {
	RunID      string `json:"run_id"`
	RoundID    string `json:"round_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	AttemptID  string `json:"attempt_id,omitempty"`
}

// SanitizedArtifact stores diagnostic value without persisting raw secrets.
type SanitizedArtifact struct {
	ByteLength int    `json:"byte_length"`
	Hash       string `json:"hash"`
	Preview    string `json:"preview,omitempty"`
}

// ProviderCapabilityReceipt captures the evidence used for launch decisions.
type ProviderCapabilityReceipt struct {
	LaunchMode                string   `json:"launch_mode"`
	PromptTransportMode       string   `json:"prompt_transport_mode"`
	CollectionModes           []string `json:"collection_modes,omitempty"`
	SupportsPromptReceipt     bool     `json:"supports_prompt_receipt"`
	SupportsCollectionReceipt bool     `json:"supports_collection_receipt"`
	SupportsCWDCheck          bool     `json:"supports_cwd_check"`
}

// ProviderPreflightReceipt is written before round execution begins.
type ProviderPreflightReceipt struct {
	SchemaVersion string                    `json:"schema_version"`
	Timestamp     time.Time                 `json:"timestamp"`
	Correlation   CorrelationIDs            `json:"correlation"`
	Provider      string                    `json:"provider"`
	Status        string                    `json:"status"`
	LaunchMode    string                    `json:"launch_mode"`
	TransportMode string                    `json:"transport_mode"`
	RequestedCWD  string                    `json:"requested_cwd,omitempty"`
	EffectiveCWD  string                    `json:"effective_cwd,omitempty"`
	FailureCode   string                    `json:"failure_code,omitempty"`
	Reason        string                    `json:"reason,omitempty"`
	NextStep      string                    `json:"next_step,omitempty"`
	Capability    ProviderCapabilityReceipt `json:"capability"`
}

// PromptTransportReceipt records prompt transport metadata and integrity state.
// @AX:ANCHOR: [AUTO] public JSON wire contract for redacted prompt transport status and stable failure classification
// @AX:REASON: [AUTO] persisted artifacts and diagnostics depend on compatible failure_code and sanitized prompt metadata
type PromptTransportReceipt struct {
	SchemaVersion string            `json:"schema_version"`
	Timestamp     time.Time         `json:"timestamp"`
	Correlation   CorrelationIDs    `json:"correlation"`
	Provider      string            `json:"provider"`
	TransportMode string            `json:"transport_mode"`
	Status        string            `json:"status"`
	Mismatch      string            `json:"mismatch,omitempty"`
	FailureCode   string            `json:"failure_code,omitempty"`
	Prompt        SanitizedArtifact `json:"prompt"`
}

// CollectionReceipt records how output was collected for a provider.
type CollectionReceipt struct {
	SchemaVersion  string            `json:"schema_version"`
	Timestamp      time.Time         `json:"timestamp"`
	Correlation    CorrelationIDs    `json:"correlation"`
	Provider       string            `json:"provider"`
	CollectionMode string            `json:"collection_mode"`
	Provenance     string            `json:"provenance"`
	Status         string            `json:"status"`
	Partial        bool              `json:"partial,omitempty"`
	Error          string            `json:"error,omitempty"`
	Output         SanitizedArtifact `json:"output"`
}

// ReliabilityEvent surfaces actionable contract failures.
type ReliabilityEvent struct {
	SchemaVersion string         `json:"schema_version"`
	Timestamp     time.Time      `json:"timestamp"`
	Correlation   CorrelationIDs `json:"correlation"`
	Kind          string         `json:"kind"`
	Severity      string         `json:"severity"`
	Message       string         `json:"message"`
	NextStep      string         `json:"next_step,omitempty"`
}

// FailureBundle exports the persisted evidence needed to reconstruct a run.
// @AX:ANCHOR: [AUTO] public persisted bundle schema shared by runtime diagnostics and artifact consumers
// @AX:REASON: [AUTO] receipt collections and events are additive ordered projections; readers depend on their field names
type FailureBundle struct {
	SchemaVersion      string                     `json:"schema_version"`
	Timestamp          time.Time                  `json:"timestamp"`
	RunID              string                     `json:"run_id"`
	Degraded           bool                       `json:"degraded"`
	Summary            string                     `json:"summary"`
	NextStep           string                     `json:"next_step,omitempty"`
	PreflightReceipts  []ProviderPreflightReceipt `json:"preflight_receipts,omitempty"`
	PromptReceipts     []PromptTransportReceipt   `json:"prompt_receipts,omitempty"`
	CollectionReceipts []CollectionReceipt        `json:"collection_receipts,omitempty"`
	Events             []ReliabilityEvent         `json:"events,omitempty"`
}

// ReliabilitySummary points callers to persisted artifacts and counts.
type ReliabilitySummary struct {
	RunID              string `json:"run_id"`
	ArtifactDir        string `json:"artifact_dir"`
	FailureBundle      string `json:"failure_bundle,omitempty"`
	PreflightFailures  int    `json:"preflight_failures"`
	PromptMismatches   int    `json:"prompt_mismatches"`
	CollectionFailures int    `json:"collection_failures"`
	OpenEvents         int    `json:"open_events"`
}
