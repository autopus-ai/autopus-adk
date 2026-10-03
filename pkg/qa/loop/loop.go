package loop

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// stopState is why the loop ended plus the evidence for the report.
type stopState struct {
	reason StopReason
	code   string
	detail string
}

type runner struct {
	opts       Options
	deps       Deps
	git        gitRepo
	projectDir string
	// prefix is the project directory relative to the repository top, with a
	// trailing slash, or "" when the project is the repository.
	prefix   string
	testDir  string
	detached bool
	report   Report
	flaky    map[string]bool
	failures map[string]run.AdapterResult
}

// Run executes the loop. It returns the report whenever a loop branch was
// created, and an *Error unless the lane ended passed or passed_with_flaky.
// A refused precondition returns before any branch or file is created.
func Run(ctx context.Context, opts Options, deps Deps) (Report, error) {
	opts, err := normalize(opts)
	if err != nil {
		return Report{}, err
	}
	r, err := prepare(opts, deps.withDefaults())
	if err != nil {
		return Report{}, err
	}
	stop := r.iterate(ctx)
	r.report.StopReason, r.report.StopCode, r.report.StopDetail = stop.reason, stop.code, stop.detail
	return r.finish()
}

func normalize(opts Options) (Options, error) {
	if strings.TrimSpace(opts.ProjectDir) == "" {
		opts.ProjectDir = "."
	}
	if strings.TrimSpace(opts.Lane) == "" {
		opts.Lane = "fast"
	}
	if opts.MaxIterations == 0 {
		opts.MaxIterations = DefaultMaxIterations
	}
	if opts.AgentTimeout <= 0 {
		opts.AgentTimeout = DefaultAgentTimeout
	}
	switch {
	case opts.MaxIterations < 0:
		return opts, &Error{Code: CodeInvalidOptions, Message: "max iterations must be at least 1"}
	case strings.TrimSpace(string(opts.Agent)) == "":
		return opts, &Error{Code: CodeInvalidOptions, Message: "an agent target is required"}
	}
	return opts, nil
}

// prepare checks the repository and creates the loop branch. Nothing is
// written before the tracked tree is known to be clean.
func prepare(opts Options, deps Deps) (*runner, error) {
	projectDir, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return nil, &Error{Code: CodeInvalidOptions, Message: err.Error()}
	}
	top, err := runGit(projectDir, deps.GitEnv, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, &Error{Code: CodeNotGitRepo, Message: projectDir + " is not inside a git work tree"}
	}
	prefix, err := runGit(projectDir, deps.GitEnv, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, &Error{Code: CodeGitFailed, Message: err.Error()}
	}
	g := gitRepo{top: strings.TrimSpace(top), env: deps.GitEnv}
	if _, err := g.line("rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return nil, &Error{Code: CodeNotGitRepo, Message: "the repository has no commit to branch from"}
	}
	dirty, err := g.trackedDirty()
	if err != nil {
		return nil, &Error{Code: CodeGitFailed, Message: err.Error()}
	}
	if len(dirty) > 0 {
		return nil, &Error{Code: CodeDirtyWorktree, Message: "commit or stash tracked changes first: " + strings.Join(dirty, "; ")}
	}
	ref, detached, err := g.currentRef()
	if err != nil {
		return nil, &Error{Code: CodeGitFailed, Message: err.Error()}
	}
	started := deps.Now().UTC()
	runID := fmt.Sprintf("qaloop-%s-%s", started.Format("20060102150405"), deps.RunSuffix())
	r := &runner{
		opts: opts, deps: deps, git: g, projectDir: projectDir,
		prefix: strings.TrimSpace(prefix), detached: detached,
		flaky: map[string]bool{}, failures: map[string]run.AdapterResult{},
		report: Report{
			Schema: ReportSchema, RunID: runID, Branch: BranchPrefix + runID, OriginalRef: ref,
			Lane: opts.Lane, Agent: string(opts.Agent), MaxIterations: opts.MaxIterations,
			Iterations: []Iteration{}, StartedAt: started,
		},
	}
	testDir, _ := scenario.TestDir(projectDir)
	r.testDir = filepath.ToSlash(testDir)
	if err := g.ensureExcluded("/" + r.prefix + ReportDirRel + "/"); err != nil {
		return nil, &Error{Code: CodeGitFailed, Message: err.Error()}
	}
	if _, err := g.run("switch", "-q", "-c", r.report.Branch); err != nil {
		return nil, &Error{Code: CodeGitFailed, Message: err.Error()}
	}
	return r, nil
}

// iterate runs fix iterations until a stop condition holds. The run after
// the last permitted fix only verifies it; it never calls the agent.
func (r *runner) iterate(ctx context.Context) stopState {
	var previous []string
	fixedLast := false
	for n := 1; ; n++ {
		it := Iteration{N: n, Failures: []triage.Verdict{}}
		stop := r.triageRun(&it)
		if stop == nil {
			fingerprints := fingerprintsOf(it.Failures)
			switch {
			case fixedLast && slices.Equal(fingerprints, previous):
				stop = &stopState{StopNoProgress, CodeNoProgress, "the failure fingerprints did not change after the previous fix: " + strings.Join(fingerprints, ", ")}
			case n > r.opts.MaxIterations:
				stop = &stopState{StopMaxIterations, CodeMaxIterations, fmt.Sprintf("still failing after %d fix iteration(s)", r.opts.MaxIterations)}
			default:
				stop = r.fix(ctx, &it)
				previous, fixedLast = fingerprints, it.Commit != ""
			}
		}
		r.report.Iterations = append(r.report.Iterations, it)
		if stop != nil {
			return *stop
		}
	}
}

// triageRun runs the lane, re-runs each failure once, and classifies it. It
// returns nil when a failure is fixable and no failure forbids fixing.
func (r *runner) triageRun(it *Iteration) *stopState {
	result, runErr := r.deps.Run(r.runOptions(""))
	it.QARunID, it.RunStatus = result.RunID, result.Status
	clear(r.failures)
	passed := 0
	for _, ar := range result.AdapterResults {
		switch ar.Status {
		case "failed", "blocked":
			r.failures[ar.JourneyID] = ar
			it.Failures = append(it.Failures, r.classify(ar))
		case "passed":
			passed++
		}
	}
	if len(it.Failures) == 0 {
		switch {
		case runErr != nil:
			return &stopState{StopBlockedEnv, CodeBlockedEnv, "qa run failed without a failed journey: " + runErr.Error()}
		case passed == 0:
			return &stopState{StopBlockedEnv, CodeBlockedEnv, fmt.Sprintf("lane %q ran no journey that passed or failed", r.opts.Lane)}
		}
		return r.passedStop()
	}
	var environment, unknown []string
	fixable := 0
	for _, v := range it.Failures {
		label := v.JourneyID + " (" + oneLine(v.Signal) + ")"
		switch v.Class {
		case triage.ClassFlaky:
			r.flaky[v.JourneyID] = true
		case triage.ClassEnvironment:
			environment = append(environment, label)
		case triage.ClassProductDefect, triage.ClassTestDefect, triage.ClassTestDrift:
			fixable++
		default:
			unknown = append(unknown, label)
		}
	}
	switch {
	case len(environment) > 0:
		return &stopState{StopBlockedEnv, CodeBlockedEnv, "environment failures: " + strings.Join(environment, ", ")}
	case len(unknown) > 0:
		return &stopState{StopUnknownFailure, CodeUnknownFailure, "unclassified failures: " + strings.Join(unknown, ", ")}
	case fixable == 0:
		return r.passedStop()
	}
	return nil
}

func (r *runner) classify(ar run.AdapterResult) triage.Verdict {
	var rerun *bool
	if ar.Status == "failed" {
		again, _ := r.deps.Run(r.runOptions(ar.JourneyID))
		passed := journeyPassed(again, ar.JourneyID)
		rerun = &passed
	}
	verdict := r.deps.Classify(r.input(ar, rerun))
	verdict.JourneyID = firstNonEmpty(verdict.JourneyID, ar.JourneyID)
	if verdict.Class == "" {
		verdict.Class = triage.ClassUnknown
	}
	return verdict
}

// input prefers the manifest's evidence and fills what it lacks from the
// adapter result, which is all a blocked journey without a manifest has.
func (r *runner) input(ar run.AdapterResult, rerun *bool) triage.Input {
	var in triage.Input
	if ar.QAMESHManifestPath != "" {
		if loaded, err := r.deps.LoadInput(r.projectDir, ar.QAMESHManifestPath); err == nil {
			in = loaded
		}
	}
	in.JourneyID = firstNonEmpty(in.JourneyID, ar.JourneyID)
	in.Adapter = firstNonEmpty(in.Adapter, ar.Adapter)
	in.Status = firstNonEmpty(in.Status, ar.Status)
	in.FailureText = firstNonEmpty(in.FailureText, ar.FailureSummary)
	in.ProjectDir = firstNonEmpty(in.ProjectDir, r.projectDir)
	if in.SetupGapCode == "" && ar.SetupGap != nil {
		in.SetupGapCode = firstNonEmpty(ar.SetupGap.Reason, "setup_gap")
	}
	in.RerunPassed = rerun
	return in
}

func journeyPassed(result run.Result, journeyID string) bool {
	found := false
	for _, ar := range result.AdapterResults {
		if ar.JourneyID != journeyID {
			continue
		}
		if ar.Status != "passed" {
			return false
		}
		found = true
	}
	return found
}

func (r *runner) runOptions(journeyID string) run.Options {
	return run.Options{
		ProjectDir: r.projectDir, Profile: r.opts.Profile, Lane: r.opts.Lane,
		JourneyID: firstNonEmpty(journeyID, r.opts.JourneyID),
	}
}

func (r *runner) passedStop() *stopState {
	if len(r.flaky) > 0 {
		return &stopState{reason: StopPassedWithFlaky, detail: "quarantined flaky journeys: " + strings.Join(sortedKeys(r.flaky), ", ")}
	}
	return &stopState{reason: StopPassed}
}

func fingerprintsOf(verdicts []triage.Verdict) []string {
	set := map[string]bool{}
	for _, v := range verdicts {
		if v.Class != triage.ClassFlaky {
			set[v.Fingerprint()] = true
		}
	}
	return sortedKeys(set)
}

// finish restores the original ref, writes both reports, and turns a stop
// other than passed into an error that names the branch and the report.
func (r *runner) finish() (Report, error) {
	args := []string{"switch", "-q", r.report.OriginalRef}
	if r.detached {
		args = []string{"switch", "-q", "--detach", r.report.OriginalRef}
	}
	_, switchErr := r.git.run(args...)
	r.report.Restored = switchErr == nil
	r.report.FinishedAt = r.deps.Now().UTC()
	r.report.Quarantined = sortedKeys(r.flaky)
	r.report.FinalStatus = finalStatus(r.report.StopReason)
	dir := filepath.Join(r.projectDir, filepath.FromSlash(ReportDirRel), r.report.RunID)
	r.report.ReportPath = filepath.Join(dir, "report.json")
	r.report.ReportMDPath = filepath.Join(dir, "report.md")
	writeErr := writeReports(dir, r.report)

	code := r.report.StopCode
	var problems []string
	if r.report.FinalStatus != "passed" {
		problems = append(problems, fmt.Sprintf("%s: %s", r.report.StopReason, r.report.StopDetail))
	}
	if switchErr != nil {
		code = firstNonEmpty(code, CodeGitFailed)
		problems = append(problems, "could not restore "+r.report.OriginalRef+": "+switchErr.Error())
	}
	if writeErr != nil {
		code = firstNonEmpty(code, CodeReportFailed)
		problems = append(problems, "could not write the report: "+writeErr.Error())
	}
	if len(problems) == 0 {
		return r.report, nil
	}
	return r.report, &Error{Code: code, Message: fmt.Sprintf("%s (loop branch %s, report %s)", strings.Join(problems, "; "), r.report.Branch, r.report.ReportPath)}
}
