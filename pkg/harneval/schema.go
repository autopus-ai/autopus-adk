// Package harneval evaluates the generated harness surfaces of the five
// supported platforms against a versioned golden-task set (SPEC-HARNEVAL-001).
//
// The deterministic lane loads the set strictly, refuses stale templates,
// generates every declared variant in-process with all host probes pinned,
// evaluates typed assertions, and compares the outcome with a committed
// baseline. Nothing in this package calls a model, the network, or a host CLI.
package harneval

// Wire-contract schema identifiers.
const (
	SetSchemaV1      = "harness_golden_set.v1"
	TaskSchemaV1     = "harness_golden_task.v1"
	BaselineSchemaV1 = "harness_eval_baseline.v1"
	ResultSchemaV1   = "harness_eval_result.v1"
)

// Repository-relative locations of the golden set. The quarantine area under
// candidates/ belongs to SPEC-HARNEVAL-002 and is never evaluated.
const (
	EvalRoot       = "evals/harness"
	ManifestPath   = "evals/harness/manifest.json"
	BaselinePath   = "evals/harness/baseline.json"
	QuarantinePath = "evals/harness/candidates"
)

// Task kinds, task states, and baseline row results.
const (
	KindSurface = "surface"
	KindAgent   = "agent"

	StateActive  = "active"
	StateRetired = "retired"

	ResultPass   = "pass"
	ResultFail   = "fail"
	ResultNotRun = "not_run"
)

// Failure reasons of a deterministic run (REQ-HE-03, REQ-HE-04).
// generation_failed is not named by the requirements: it closes the gap of a
// platform adapter that returns an error, which no other reason covers.
const (
	ReasonInvalid            = "invalid"
	ReasonBaselineMissing    = "baseline_missing"
	ReasonTemplatesStale     = "templates_stale"
	ReasonHostProbeUnpinned  = "host_probe_unpinned"
	ReasonGenerationFailed   = "generation_failed"
	ReasonVacuous            = "vacuous"
	ReasonRegression         = "regression"
	ReasonExpectationChanged = "expectation_changed"
	ReasonSetDigestMismatch  = "set_digest_mismatch"
	ReasonTaskMissing        = "task_missing"
)

// Detail codes carried with reason invalid. The first twelve are named by
// REQ-HE-01; the last three cover defects the requirement leaves unnamed:
// a JSON syntax or type error, a non-assertion field outside its contract,
// and a file or directory that cannot be read.
const (
	DetailUnknownField          = "unknown_field"
	DetailTrailingData          = "trailing_data"
	DetailUnknownAssertionKind  = "unknown_assertion_kind"
	DetailAssertionFieldInvalid = "assertion_field_invalid"
	DetailDuplicateTaskID       = "duplicate_task_id"
	DetailNoAssertions          = "no_assertions"
	DetailPolicyOutOfRange      = "policy_out_of_range"
	DetailUncleanPath           = "unclean_path"
	DetailSymlinkNotAllowed     = "symlink_not_allowed"
	DetailReservedActivePath    = "reserved_active_path"
	DetailCorpusDigestMismatch  = "corpus_digest_mismatch"
	DetailExpectedTestsMissing  = "expected_tests_missing"
	DetailMalformedJSON         = "malformed_json"
	DetailFieldInvalid          = "field_invalid"
	DetailReadFailed            = "read_failed"
)

// DetailRegenFailed marks a template regeneration comparison that could not
// complete; it is reported with reason templates_stale.
const DetailRegenFailed = "regen_failed"

// Assertion kinds (closed set).
const (
	AssertFileExists      = "file_exists"
	AssertFileAbsent      = "file_absent"
	AssertContains        = "contains"
	AssertNotContains     = "not_contains"
	AssertJSONPathPresent = "json_path_present"
	AssertJSONPathAbsent  = "json_path_absent"
	AssertRouteDetail     = "route_detail"
	AssertSectionParity   = "section_parity"
)

// OverridePreCommitArch is the only variant override key the set may use.
const OverridePreCommitArch = "hooks.pre_commit_arch"

// Platforms is the closed platform set in generation order.
var Platforms = []string{"claude-code", "codex", "antigravity-cli", "opencode", "omp"}

// Manifest is the harness_golden_set.v1 document.
type Manifest struct {
	SchemaVersion string     `json:"schema_version"`
	SetVersion    string     `json:"set_version"`
	ActivePaths   []string   `json:"active_paths"`
	Floors        Floors     `json:"floors"`
	Live          LivePolicy `json:"live"`
	Pins          Pins       `json:"pins"`
}

// Floors are the minimum active task counts below which a run is vacuous.
// SignedAgentTasks is the signed live lane's floor of black-box agent tasks
// (SPEC-HARNEVAL-003 REQ-HR-08); a manifest that declares none leaves it 0,
// and the signer signs nothing but vacuous for it.
type Floors struct {
	SurfaceTasks     int `json:"surface_tasks"`
	AgentTasks       int `json:"agent_tasks"`
	SignedAgentTasks int `json:"signed_agent_tasks,omitempty"`
}

// LivePolicy is the advisory live-lane policy frozen into each protocol.
type LivePolicy struct {
	K                   int     `json:"k"`
	ThresholdBP         int     `json:"threshold_bp"`
	CompletenessFloor   float64 `json:"completeness_floor"`
	MaxAgentRuns        int     `json:"max_agent_runs"`
	TrialTimeoutSeconds int     `json:"trial_timeout_seconds"`
	WorkspaceRevision   string  `json:"workspace_revision"`
	BaselineRef         string  `json:"baseline_ref"`
	Model               string  `json:"model"`
}

// Pins fix every generation input that would otherwise come from the binary
// build or the host. CodexModelCatalog is a repository-relative file path; an
// empty value pins "no catalog" rather than allowing a probe.
type Pins struct {
	GeneratorVersion   string `json:"generator_version"`
	ProjectName        string `json:"project_name"`
	CodexModelCatalog  string `json:"codex_model_catalog"`
	CodexCLIVersion    string `json:"codex_cli_version"`
	OpencodeCLIVersion string `json:"opencode_cli_version"`
}

// Task is the harness_golden_task.v1 document. Path is the repository-relative
// file the task was loaded from and is not part of the wire format.
type Task struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	Kind          string      `json:"kind"`
	Category      string      `json:"category"`
	Intent        string      `json:"intent"`
	Outcome       string      `json:"outcome"`
	Variants      []Variant   `json:"variants"`
	Assertions    []Assertion `json:"assertions"`
	CorpusRef     *CorpusRef  `json:"corpus_ref,omitempty"`
	ExpectedTests []string    `json:"expected_tests,omitempty"`
	Provenance    Provenance  `json:"provenance"`
	Status        TaskStatus  `json:"status"`
	Path          string      `json:"-"`
}

// Variant is one config override set a task is evaluated under.
type Variant struct {
	Name      string          `json:"name"`
	Overrides map[string]bool `json:"overrides"`
}

// Assertion is one typed check against a generated platform surface. Fields
// that a kind does not use must stay empty.
type Assertion struct {
	Kind          string    `json:"kind"`
	Platform      string    `json:"platform,omitempty"`
	Path          string    `json:"path,omitempty"`
	Needle        string    `json:"needle,omitempty"`
	JSONPath      string    `json:"json_path,omitempty"`
	ValueContains *string   `json:"value_contains,omitempty"`
	Route         string    `json:"route,omitempty"`
	Detail        string    `json:"detail,omitempty"`
	Files         []FileRef `json:"files,omitempty"`
	Heading       string    `json:"heading,omitempty"`
}

// FileRef names one file of a section_parity assertion.
type FileRef struct {
	Platform string `json:"platform"`
	Path     string `json:"path"`
}

// CorpusRef pins an agent task to a benchmark corpus file by raw-byte digest.
type CorpusRef struct {
	File       string `json:"file"`
	TaskID     string `json:"task_id"`
	FileSHA256 string `json:"file_sha256"`
}

// Provenance records where a task came from.
type Provenance struct {
	Kind        string `json:"kind"`
	Ref         string `json:"ref"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// TaskStatus is the lifecycle state; a retired task is a tombstone.
type TaskStatus struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

// Baseline is the committed harness_eval_baseline.v1 document.
type Baseline struct {
	SchemaVersion string        `json:"schema_version"`
	SetVersion    string        `json:"set_version"`
	SetDigest     string        `json:"set_digest"`
	Rows          []BaselineRow `json:"rows"`
}

// BaselineRow is one task outcome frozen at baseline time. Kind keeps the
// historical value so a task deleted without a tombstone is still classified.
type BaselineRow struct {
	ID                       string `json:"id"`
	Kind                     string `json:"kind"`
	State                    string `json:"state"`
	Result                   string `json:"result"`
	ExpectationDigest        string `json:"expectation_digest"`
	AcceptedRegressionReason string `json:"accepted_regression_reason,omitempty"`
	RetiredReason            string `json:"retired_reason,omitempty"`
}
