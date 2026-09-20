// Package taskroute makes advisory execution choices from declared task facts.
package taskroute

type Worker struct {
	Independent bool     `json:"independent"`
	ID          string   `json:"id"`
	OwnedPaths  []string `json:"owned_paths"`
}
type Facts struct {
	Version                 int      `json:"version"`
	Kind                    string   `json:"kind"`
	Paths                   []string `json:"paths"`
	ScopeComplete           *bool    `json:"scope_complete"`
	RequirementsClear       *bool    `json:"requirements_clear"`
	AcceptanceKnown         *bool    `json:"acceptance_known"`
	EstimatedChangedLines   *int     `json:"estimated_changed_lines"`
	Risk                    string   `json:"risk"`
	FailedAttempts          int      `json:"failed_attempts"`
	Requested               string   `json:"requested"`
	Solo                    bool     `json:"solo"`
	NativeParallelAvailable bool     `json:"native_parallel_available"`
	Workers                 []Worker `json:"workers"`
}
type Decision struct {
	Version         int      `json:"version"`
	Route           string   `json:"route"`
	Reasons         []string `json:"reasons"`
	RequiredSteps   []string `json:"required_steps"`
	SuggestedSkills []string `json:"suggested_skills"`
	Execution       string   `json:"execution"`
	ModelPolicy     string   `json:"model_policy"`
	Advisory        bool     `json:"advisory"`
}
