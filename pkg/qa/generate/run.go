package generate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/acceptance"
	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// Run-level failure codes. Per-document codes are in Rejection.Code.
const (
	CodeSpecMissing       = "qa_generate_spec_missing"
	CodeAcceptanceMissing = "qa_generate_acceptance_missing"
	CodeNoCriteria        = "qa_generate_no_criteria"
	CodeCandidateConflict = "qa_generate_candidate_conflict"
	CodeCandidateInvalid  = "qa_generate_candidate_invalid"
	CodeFailed            = "qa_generate_failed"
)

// Rejection codes the harness adds on top of the schema validators' own codes.
const (
	CodeYAMLInvalid            = "qa_generate_yaml_invalid"
	CodeSchemaUnknown          = "qa_generate_schema_unknown"
	CodeScenarioNotV2          = "qa_generate_scenario_not_v2"
	CodeIntentNotAcceptance    = "qa_generate_intent_not_acceptance"
	CodeSpecMismatch           = "qa_generate_spec_mismatch"
	CodeAcUnknown              = "qa_generate_ac_unknown"
	CodeIDExists               = "qa_generate_id_exists"
	CodeScenarioRefUnknown     = "qa_generate_scenario_ref_unknown"
	CodeTestScenariosDuplicate = "qa_generate_test_scenarios_duplicate"
)

// Error is a generation failure with a stable reason code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// ErrorCode returns the stable code for err: a generation code, an agentexec
// code such as qa_agent_cli_missing, or qa_generate_failed.
func ErrorCode(err error) string {
	var genErr *Error
	if errors.As(err, &genErr) {
		return genErr.Code
	}
	if code := agentexec.ErrorCode(err); code != "" {
		return code
	}
	return CodeFailed
}

// AgentRunner is the part of agentexec.Runner generation uses.
type AgentRunner interface {
	Run(ctx context.Context, req agentexec.Request) (agentexec.Response, error)
}

// Options configures one generation run. A nil Runner uses agentexec.NewRunner.
type Options struct {
	ProjectDir string
	SpecID     string
	Target     agentexec.Target
	Timeout    time.Duration
	Runner     AgentRunner
}

// Report is what one run produced. Problems carries the acceptance parser's
// findings, such as a criterion with no THEN line, so a gap in the SPEC is not
// mistaken for a gap in the agent's output.
type Report struct {
	Spec           string               `json:"spec"`
	Agent          string               `json:"agent"`
	Criteria       int                  `json:"criteria"`
	PromptCriteria int                  `json:"prompt_criteria"`
	Problems       []acceptance.Problem `json:"problems"`
	Documents      int                  `json:"documents"`
	Written        []string             `json:"written"`
	Scenarios      []string             `json:"scenarios"`
	TestScenarios  []string             `json:"test_scenarios"`
	Rejected       []Rejection          `json:"rejected"`
	Coverage       []CriterionCoverage  `json:"coverage"`
}

// Run parses the SPEC's criteria, asks the agent for documents in generate
// mode, keeps only those that hold up against the criteria, and writes them as
// candidates. The report is returned even with an error, so a caller can still
// show the coverage and rejections of a run that ended in a conflict.
func Run(ctx context.Context, opts Options) (Report, error) {
	specID := strings.TrimSpace(opts.SpecID)
	report := Report{
		Spec: specID, Agent: string(opts.Target), Problems: []acceptance.Problem{}, Written: []string{},
		Scenarios: []string{}, TestScenarios: []string{}, Rejected: []Rejection{}, Coverage: []CriterionCoverage{},
	}
	if specID == "" {
		return report, &Error{Code: CodeSpecMissing, Message: "a SPEC id is required"}
	}
	criteria, problems, err := acceptance.ParseSpec(opts.ProjectDir, specID)
	if err != nil {
		return report, &Error{Code: CodeAcceptanceMissing, Message: err.Error()}
	}
	report.Problems = append(report.Problems, problems...)
	report.Criteria = len(criteria)
	if len(criteria) == 0 {
		return report, &Error{Code: CodeNoCriteria, Message: fmt.Sprintf(
			"%s declares no acceptance criteria; nothing can be generated without an oracle", acceptance.SpecPath(opts.ProjectDir, specID))}
	}
	report.PromptCriteria = len(criteria)
	if report.PromptCriteria > MaxPromptCriteria {
		report.PromptCriteria = MaxPromptCriteria
	}
	existing := existingScenarioIDs(opts.ProjectDir)
	prompt := BuildPrompt(PromptInput{
		SpecID: specID, Criteria: criteria, ExistingScenarioIDs: existing, Journeys: journeyHints(opts.ProjectDir),
	})
	runner := opts.Runner
	if runner == nil {
		runner = agentexec.NewRunner()
	}
	resp, err := runner.Run(ctx, agentexec.Request{
		Target: opts.Target, Mode: agentexec.ModeGenerate, Prompt: prompt, WorkDir: opts.ProjectDir, Timeout: opts.Timeout,
	})
	if err != nil {
		return report, err
	}
	docs := ExtractYAMLDocuments(resp.Stdout)
	report.Documents = len(docs)
	ids := make([]string, 0, len(criteria))
	for _, c := range criteria {
		ids = append(ids, c.ID)
	}
	outcome := Evaluate(specID, ids, docs, existing)
	for _, accepted := range outcome.Scenarios {
		report.Scenarios = append(report.Scenarios, accepted.ID)
	}
	for _, accepted := range outcome.TestScenarios {
		report.TestScenarios = append(report.TestScenarios, accepted.ID)
	}
	report.Rejected = outcome.Rejected
	report.Coverage = outcome.Coverage
	written, err := WriteCandidates(opts.ProjectDir, outcome)
	report.Written = written
	return report, err
}

// existingScenarioIDs lists active and candidate scenario ids. A file that does
// not parse still reserves its file name, so a new candidate cannot collide
// with it either.
func existingScenarioIDs(projectDir string) []string {
	seen := map[string]bool{}
	for _, dir := range []string{scenario.Dir(projectDir), scenario.CandidatesDir(projectDir)} {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
		for _, path := range paths {
			seen[strings.TrimSuffix(filepath.Base(path), ".yaml")] = true
			if loaded, err := scenario.LoadFile(path); err == nil && loaded.ID != "" {
				seen[loaded.ID] = true
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// journeyHints offers the packs that declare GUI origins: only those can run a
// user scenario. An unreadable journey directory just means no hints.
func journeyHints(projectDir string) []JourneyHint {
	packs, err := journey.LoadDir(projectDir)
	if err != nil {
		return nil
	}
	var hints []JourneyHint
	for _, pack := range packs {
		if len(pack.GUI.AllowedOrigins) > 0 {
			hints = append(hints, JourneyHint{ID: pack.ID, Origin: pack.GUI.AllowedOrigins[0]})
		}
	}
	return hints
}
