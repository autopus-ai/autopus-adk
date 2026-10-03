// Package loop drives `auto qa loop`: it runs a QA lane, re-runs each failure
// once, triages it, lets an agent repair only what the failure's class
// permits, guards the resulting diff, and commits or reverts on a dedicated
// branch until the lane passes or a stop condition holds.
//
// Git stays real on every path: the guard reads the diff git reports, the
// revert restores through git, and the commit runs the project's hooks. The
// lane, the triage, and the agent are injected so tests can drive the loop
// without a browser or a model.
package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/agentexec"
	"github.com/insajin/autopus-adk/pkg/qa/run"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

const (
	// ReportSchema versions report.json.
	ReportSchema = "qamesh.qaloop.report.v1"
	// BranchPrefix namespaces every loop branch.
	BranchPrefix = "autopus/qa-loop/"
	// ReportDirRel is the project-relative directory reports land in.
	ReportDirRel = ".autopus/qa/loop"
	// DefaultMaxIterations bounds the fix iterations when Options leaves it 0.
	DefaultMaxIterations = 3
	// DefaultAgentTimeout bounds one agent run when Options leaves it 0.
	DefaultAgentTimeout = 15 * time.Minute
)

// StopReason names why the loop ended.
type StopReason string

// Stop reasons recorded in report.json.
const (
	StopPassed          StopReason = "passed"
	StopPassedWithFlaky StopReason = "passed_with_flaky"
	StopMaxIterations   StopReason = "max_iterations"
	StopNoProgress      StopReason = "no_progress"
	StopBlockedEnv      StopReason = "blocked_environment"
	StopUnknownFailure  StopReason = "unknown_failure"
	StopGuardRejected   StopReason = "guard_rejected"
	StopCommitRejected  StopReason = "commit_rejected"
	StopAgentFailed     StopReason = "agent_failed"
)

// Stable error codes. Agent failures keep agentexec's own codes.
const (
	CodeDirtyWorktree  = "qa_loop_dirty_worktree"
	CodeNotGitRepo     = "qa_loop_not_git_repo"
	CodeInvalidOptions = "qa_loop_invalid_options"
	CodeGitFailed      = "qa_loop_git_failed"
	CodeReportFailed   = "qa_loop_report_failed"
	CodeGuardRejected  = "qa_loop_guard_rejected"
	CodeCommitRejected = "qa_loop_commit_rejected"
	CodeNoProgress     = "qa_loop_no_progress"
	CodeMaxIterations  = "qa_loop_max_iterations"
	CodeBlockedEnv     = "qa_loop_blocked_environment"
	CodeUnknownFailure = "qa_loop_unknown_failure"
	CodeAgentFailed    = "qa_loop_agent_failed"
)

// Options configures one loop run.
type Options struct {
	ProjectDir    string
	Lane          string
	Profile       string
	JourneyID     string
	Agent         agentexec.Target
	MaxIterations int
	AgentTimeout  time.Duration
}

// RunFunc runs a QA lane. run.Execute fits: it returns an error for a failed
// or blocked run alongside the populated result.
type RunFunc func(opts run.Options) (run.Result, error)

// Classifier assigns one triage verdict to a failed journey.
type Classifier func(in triage.Input) triage.Verdict

// InputLoader builds a triage input from an evidence manifest.
type InputLoader func(projectDir, manifestPath string) (triage.Input, error)

// AgentFunc runs the repair agent headless.
type AgentFunc func(ctx context.Context, req agentexec.Request) (agentexec.Response, error)

// Deps are the seams the loop calls through. A nil field falls back to the
// production implementation.
type Deps struct {
	Run       RunFunc
	Classify  Classifier
	LoadInput InputLoader
	Agent     AgentFunc
	// Compile regenerates specs after an accepted heal.
	Compile func(projectDir string) error
	Now     func() time.Time
	// RunSuffix returns the random tail of the run id.
	RunSuffix func() string
	// GitEnv is appended to the environment of every git subprocess.
	GitEnv []string
}

// DefaultDeps wires the production lane runner, triage, agent, and compiler.
func DefaultDeps() Deps {
	return Deps{
		Run:       run.Execute,
		Classify:  triage.Classify,
		LoadInput: triage.InputFromManifest,
		Agent:     agentexec.NewRunner().Run,
		Compile: func(projectDir string) error {
			_, err := scenario.CompileProject(projectDir, false)
			return err
		},
		Now:       time.Now,
		RunSuffix: randomSuffix,
	}
}

func (d Deps) withDefaults() Deps {
	def := DefaultDeps()
	if d.Run == nil {
		d.Run = def.Run
	}
	if d.Classify == nil {
		d.Classify = def.Classify
	}
	if d.LoadInput == nil {
		d.LoadInput = def.LoadInput
	}
	if d.Agent == nil {
		d.Agent = def.Agent
	}
	if d.Compile == nil {
		d.Compile = def.Compile
	}
	if d.Now == nil {
		d.Now = def.Now
	}
	if d.RunSuffix == nil {
		d.RunSuffix = def.RunSuffix
	}
	return d
}

func randomSuffix() string {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000"
	}
	return hex.EncodeToString(b[:])
}

// Error is a loop failure carrying a stable code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// ErrorCode returns the stable code a loop error carries.
func ErrorCode(err error) string {
	var loopErr *Error
	if errors.As(err, &loopErr) {
		return loopErr.Code
	}
	return "qa_loop_failed"
}
