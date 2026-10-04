// Package autopilot chains the intent-anchored QA steps behind one command:
// generate scenarios from a SPEC's acceptance criteria, promote the ones that
// hold up, compile them, then run the self-healing loop.
//
// It adds no judgement of its own. Every gate stays where it already lives:
// generation checks that each expected value traces to a criterion, promotion
// refuses unconfirmed agent assertions and overwrites, and the loop's guard
// refuses fixes that move an oracle. The one decision autopilot owns is when
// to stop and ask a person, which is after generation unless the caller
// passed --auto.
package autopilot

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/generate"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
	"github.com/insajin/autopus-adk/pkg/qa/promote"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// Stages, in order. Report.Stage names the last one that ran.
const (
	StageGenerate = "generate"
	StageConfirm  = "confirm"
	StagePromote  = "promote"
	StageCompile  = "compile"
	StageLoop     = "loop"
)

// CodeDeclined is returned when the person answered no at the confirmation.
const CodeDeclined = "qa_go_declined"

// DefaultLane runs compiled @journey specs; it is used when a project's packs
// name it. Otherwise the first lane any pack declares, else fast.
const DefaultLane = "browser-staging"

// Options configures one run. An empty SpecID skips generation and runs the
// loop over the scenarios already in place.
type Options struct {
	ProjectDir    string
	SpecID        string
	Agent         agentexec.Target
	Lane          string
	Auto          bool
	MaxIterations int
	AgentTimeout  time.Duration
	// Confirm is asked after generation when Auto is false. It returns true to
	// continue. A nil Confirm with Auto false declines, so an unattended run
	// can never proceed by accident.
	Confirm func(generate.Report) bool
	// Progress, when set, receives one line per stage as it starts.
	Progress func(stage, line string)
	// OnGenerated, when set, sees the generation report before the
	// confirmation, so coverage is shown whether or not anyone is asked.
	OnGenerated func(generate.Report)
}

// Deps are the steps autopilot calls; nil fields use the real packages.
type Deps struct {
	Generate func(context.Context, generate.Options) (generate.Report, error)
	Promote  func(string, promote.Options) (promote.Report, error)
	Compile  func(string, bool) (scenario.Result, error)
	Loop     func(context.Context, qaloop.Options) (qaloop.Report, error)
}

// Report is what one run did, stage by stage.
type Report struct {
	Spec     string           `json:"spec,omitempty"`
	Agent    string           `json:"agent"`
	Lane     string           `json:"lane"`
	Stage    string           `json:"stage"`
	Generate *generate.Report `json:"generate,omitempty"`
	Promote  *promote.Report  `json:"promote,omitempty"`
	Compiled int              `json:"compiled"`
	Loop     *qaloop.Report   `json:"loop,omitempty"`
}

// Error is a stop before the loop finished, with the stage that stopped.
type Error struct {
	Stage string
	Code  string
	Err   error
}

func (e *Error) Error() string { return e.Stage + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Run executes the chain.
func Run(ctx context.Context, opts Options, deps Deps) (Report, error) {
	deps = deps.withDefaults()
	if strings.TrimSpace(opts.Lane) == "" {
		opts.Lane = DetectLane(opts.ProjectDir)
	}
	report := Report{Spec: strings.TrimSpace(opts.SpecID), Agent: string(opts.Agent), Lane: opts.Lane}
	progress := func(stage, line string) {
		report.Stage = stage
		if opts.Progress != nil {
			opts.Progress(stage, line)
		}
	}
	if report.Spec != "" {
		progress(StageGenerate, fmt.Sprintf("generating scenarios from %s with %s", report.Spec, opts.Agent))
		gen, err := deps.Generate(ctx, generate.Options{ProjectDir: opts.ProjectDir, SpecID: report.Spec, Target: opts.Agent})
		report.Generate = &gen
		if err != nil {
			return report, &Error{Stage: StageGenerate, Code: generate.ErrorCode(err), Err: err}
		}
		if opts.OnGenerated != nil {
			opts.OnGenerated(gen)
		}
		if !opts.Auto {
			progress(StageConfirm, "waiting for confirmation")
			if opts.Confirm == nil || !opts.Confirm(gen) {
				return report, &Error{Stage: StageConfirm, Code: CodeDeclined,
					Err: fmt.Errorf("stopped before promotion; candidates stay under %s (pass --auto to run unattended)", filepath.ToSlash(scenario.CandidatesDirRel))}
			}
		}
		progress(StagePromote, "promoting candidates")
		pr, err := deps.Promote(opts.ProjectDir, promote.Options{All: true})
		report.Promote = &pr
		if err != nil {
			return report, &Error{Stage: StagePromote, Code: promote.ErrorCode(err), Err: err}
		}
	}
	progress(StageCompile, "compiling scenarios")
	compiled, err := deps.Compile(opts.ProjectDir, false)
	if err != nil {
		return report, &Error{Stage: StageCompile, Code: "qa_go_compile_failed", Err: err}
	}
	report.Compiled = len(compiled.Compiled)
	progress(StageLoop, fmt.Sprintf("running lane %s; failures are triaged and fixed on a loop branch", opts.Lane))
	lr, err := deps.Loop(ctx, qaloop.Options{
		ProjectDir: opts.ProjectDir, Lane: opts.Lane, Agent: opts.Agent,
		MaxIterations: opts.MaxIterations, AgentTimeout: opts.AgentTimeout,
		SeedPaths: seedPaths(opts.ProjectDir, compiled), SeedMessage: seedSubject(report.Spec),
	})
	report.Loop = &lr
	if err != nil {
		return report, &Error{Stage: StageLoop, Code: qaloop.ErrorCode(err), Err: err}
	}
	return report, nil
}

// DetectLane picks the lane `auto qa go` runs when --lane is omitted.
func DetectLane(projectDir string) string {
	packs, err := journey.LoadDir(projectDir)
	if err != nil || len(packs) == 0 {
		return "fast"
	}
	first := ""
	for _, pack := range packs {
		for _, lane := range pack.Lanes {
			if lane == DefaultLane {
				return DefaultLane
			}
			if first == "" {
				first = lane
			}
		}
	}
	if first == "" {
		return "fast"
	}
	return first
}

// seedPaths are the QA files the loop commits first: active scenarios and
// test scenarios (never the candidates directories, which hold what was not
// promoted) and the compiled specs.
func seedPaths(projectDir string, compiled scenario.Result) []string {
	var out []string
	for _, dir := range []string{scenario.DirRel, testscenario.DirRel} {
		matches, _ := filepath.Glob(filepath.Join(projectDir, dir, "*.yaml"))
		for _, m := range matches {
			if rel, err := filepath.Rel(projectDir, m); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
	}
	if compiled.TestDir != "" {
		out = append(out, filepath.ToSlash(filepath.Join(compiled.TestDir, scenario.GeneratedDirName)))
	}
	sort.Strings(out)
	return out
}

func seedSubject(specID string) string {
	if specID == "" {
		return "test(qa): QA 시나리오를 갱신한다"
	}
	return "test(qa): " + specID + " 시나리오를 추가한다"
}

func (d Deps) withDefaults() Deps {
	if d.Generate == nil {
		d.Generate = generate.Run
	}
	if d.Promote == nil {
		d.Promote = promote.Promote
	}
	if d.Compile == nil {
		d.Compile = scenario.CompileProject
	}
	if d.Loop == nil {
		d.Loop = func(ctx context.Context, opts qaloop.Options) (qaloop.Report, error) {
			return qaloop.Run(ctx, opts, qaloop.DefaultDeps())
		}
	}
	return d
}
