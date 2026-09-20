// Package skillpolicy selects skills only from explicit policy and task facts.
package skillpolicy

const (
	PolicySchema = "skill_policy.v1"
	TaskSchema   = "skill_task.v1"
	CasesSchema  = "skill_policy_cases.v1"
	MaxJSONBytes = 1 << 20
)

type Policy struct {
	SchemaVersion string      `json:"schema_version"`
	Candidates    []Candidate `json:"candidates"`
}

type Candidate struct {
	ID                  string              `json:"id"`
	AllowedTaskClasses  []string            `json:"allowed_task_classes"`
	ExcludedTaskClasses []string            `json:"excluded_task_classes,omitempty"`
	RequiredFiles       []string            `json:"required_files,omitempty"`
	SupportedVersions   map[string][]string `json:"supported_versions,omitempty"`
}

type Task struct {
	SchemaVersion    string            `json:"schema_version"`
	Class            string            `json:"class"`
	DeclaredVersions map[string]string `json:"declared_versions,omitempty"`
}

type Evidence struct {
	Source string `json:"source"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}

type Decision struct {
	ID       string     `json:"id"`
	Status   string     `json:"status"`
	Reasons  []string   `json:"reasons"`
	Evidence []Evidence `json:"evidence"`
}

type Result struct {
	SchemaVersion string     `json:"schema_version"`
	Selected      []string   `json:"selected"`
	Decisions     []Decision `json:"decisions"`
}

type Cases struct {
	SchemaVersion string `json:"schema_version"`
	Cases         []Case `json:"cases"`
}

type Case struct {
	Name             string   `json:"name"`
	Task             Task     `json:"task"`
	ExpectedSelected []string `json:"expected_selected"`
}

type CaseResult struct {
	Name             string   `json:"name"`
	Passed           bool     `json:"passed"`
	ExpectedSelected []string `json:"expected_selected"`
	ActualSelected   []string `json:"actual_selected"`
}

type CheckResult struct {
	SchemaVersion string       `json:"schema_version"`
	Passed        bool         `json:"passed"`
	Cases         []CaseResult `json:"cases"`
}
