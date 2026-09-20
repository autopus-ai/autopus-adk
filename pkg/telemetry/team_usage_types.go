package telemetry

// TeamAgent is a caller-declared expected participant, not discovered capacity.
type TeamAgent struct {
	AgentID       string `json:"agent_id"`
	ParentAgentID string `json:"parent_agent_id,omitempty"`
	Role          string `json:"role"`
}
type TeamUsageObservation struct {
	AgentID         string          `json:"agent_id"`
	UsageScope      string          `json:"usage_scope"`
	CaptureComplete bool            `json:"capture_complete"`
	Usage           []UsageEnvelope `json:"usage"`
}
type TeamUsageEvidence struct {
	Version        int                    `json:"version"`
	TeamRunID      string                 `json:"team_run_id"`
	ExpectedAgents []TeamAgent            `json:"expected_agents"`
	Observations   []TeamUsageObservation `json:"observations"`
}

// TeamUsageMetrics distinguishes a complete total from known partial spend.
type TeamUsageMetrics struct {
	KnownEstimatedCostUSD float64  `json:"known_estimated_cost_usd"`
	Complete              bool     `json:"complete"`
	CostComplete          bool     `json:"cost_complete"`
	ActualTokens          *int64   `json:"actual_tokens"`
	KnownActualTokens     int64    `json:"known_actual_tokens"`
	ActualCostUSD         *float64 `json:"actual_cost_usd"`
	KnownActualCostUSD    float64  `json:"known_actual_cost_usd"`
	EstimatedTokens       *int64   `json:"estimated_tokens"`
	UniqueModelCallCount  int      `json:"unique_model_call_count"`
}
type TeamAgentUsage struct {
	AgentID         string `json:"agent_id"`
	ParentAgentID   string `json:"parent_agent_id,omitempty"`
	Role            string `json:"role"`
	UsageScope      string `json:"usage_scope"`
	Observed        bool   `json:"observed"`
	CaptureComplete bool   `json:"capture_complete"`
	TeamUsageMetrics
}
type TeamUsageReport struct {
	Version              int              `json:"version"`
	TeamRunID            string           `json:"team_run_id"`
	ExpectedAgents       int              `json:"expected_agents"`
	ObservedAgents       int              `json:"observed_agents"`
	MissingAgents        []string         `json:"missing_agents"`
	ExcludedRollups      []string         `json:"excluded_rollups"`
	AttributionConflicts []string         `json:"attribution_conflicts"`
	ByAgent              []TeamAgentUsage `json:"by_agent"`
	TeamUsageMetrics
}
