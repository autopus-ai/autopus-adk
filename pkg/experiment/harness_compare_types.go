package experiment

import "github.com/insajin/autopus-adk/pkg/telemetry"

// HarnessIdentity holds conditions that must match across arms for one task.
// Harness configuration is deliberately outside this shared identity.
type HarnessIdentity struct {
	Provider        string `json:"provider"`
	ProviderVersion string `json:"provider_version"`
	Model           string `json:"model"`
	ModelVersion    string `json:"model_version"`
	Effort          string `json:"effort"`
	CacheStratum    string `json:"cache_stratum"`
	TaskHash        string `json:"task_hash"`
	OracleHash      string `json:"oracle_hash"`
	EnvironmentHash string `json:"environment_hash"`
	BudgetHash      string `json:"budget_hash"`
	Revision        string `json:"revision"`
}

type HarnessObservation struct {
	TaskID            string               `json:"task_id"`
	Arm               string               `json:"arm"`
	HarnessRevision   string               `json:"harness_revision"`
	HarnessConfigHash string               `json:"harness_config_hash"`
	Identity          HarnessIdentity      `json:"identity"`
	Accepted          *bool                `json:"accepted"`
	ElapsedMS         *int64               `json:"elapsed_ms"`
	HumanCorrections  *int64               `json:"human_corrections"`
	Runs              []telemetry.AgentRun `json:"runs"`
}

type HarnessEvidence struct {
	Version         int                  `json:"version"`
	ExpectedTaskIDs []string             `json:"expected_task_ids"`
	Observations    []HarnessObservation `json:"observations"`
}

type HarnessArmReport struct {
	KnownActualTokens int64    `json:"known_actual_tokens"`
	MeasuredTasks     int      `json:"measured_tasks"`
	HarnessRevision   string   `json:"harness_revision"`
	HarnessConfigHash string   `json:"harness_config_hash"`
	Arm               string   `json:"arm"`
	Expected          int      `json:"expected"`
	Observed          int      `json:"observed"`
	Accepted          int      `json:"accepted"`
	Rejected          int      `json:"rejected"`
	UnknownAcceptance int      `json:"unknown_acceptance"`
	ActualTokens      *int64   `json:"actual_tokens"`
	ElapsedMS         *int64   `json:"elapsed_ms"`
	HumanCorrections  *int64   `json:"human_corrections"`
	MissingTasks      []string `json:"missing_tasks"`
}

type HarnessPairReport struct {
	Complete         bool              `json:"complete"`
	ExclusionReasons map[string]string `json:"exclusion_reasons"`
	Baseline         string            `json:"baseline"`
	Candidate        string            `json:"candidate"`
	TaskCount        int               `json:"task_count"`
	TaskIDs          []string          `json:"task_ids"`
	ExcludedTasks    []string          `json:"excluded_tasks"`
	TokenDelta       *int64            `json:"token_delta"`
	ElapsedDeltaMS   *int64            `json:"elapsed_delta_ms"`
}

type HarnessReport struct {
	Version  int                 `json:"version"`
	Mode     string              `json:"mode"`
	Complete bool                `json:"complete"`
	Arms     []HarnessArmReport  `json:"arms"`
	Pairs    []HarnessPairReport `json:"pairs"`
}
