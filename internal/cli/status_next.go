package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// statusNextAction is one command status suggests: the absolute directory it
// runs in, where it runs, the provider it dispatches with, and what it calls
// and writes. Status never runs it; auto_run is always false so a consumer
// never has to infer that (REQ-SEV-NEXT-01).
type statusNextAction struct {
	ID            string              `json:"id"`
	SpecID        string              `json:"spec_id,omitempty"`
	Cwd           string              `json:"cwd"`
	Command       string              `json:"command"`
	Shell         string              `json:"shell,omitempty"`
	Forms         []statusCommandForm `json:"forms,omitempty"`
	Purpose       string              `json:"purpose"`
	RunsIn        string              `json:"runs_in"`
	Provider      string              `json:"provider"`
	ExternalCalls string              `json:"external_calls"`
	Writes        string              `json:"writes"`
	Optional      bool                `json:"optional"`
	AutoRun       bool                `json:"auto_run"`
}

// statusCommandForm spells a route for the platforms that share a syntax.
type statusCommandForm struct {
	Platforms []string `json:"platforms"`
	Command   string   `json:"command"`
}

const (
	runsInTerminal       = "terminal"
	runsInAgentSession   = "agent_session"
	statusMaxSpecActions = 3
	agentSessionProvider = "the agent session's model"
	agentSessionCalls    = "model calls of the agent session"
	pipelineRunWrites    = "the working tree and .autopus/pipeline-state"
)

// buildStatusNextActions derives the next commands from the evidence and the
// install state: setup gaps first, then up to three SPECs that need a run or
// review, then work with and without a SPEC, then optional Desktop.
func buildStatusNextActions(specs []specEntry, evidence []statusEvidence, env statusEnvironment) []statusNextAction {
	var actions []statusNextAction
	add := func(action statusNextAction, ok bool) {
		if ok {
			actions = append(actions, action)
		}
	}
	if !env.ConfigFound {
		add(terminalAction("init", "", env.Dir, "auto init",
			"install the Autopus harness here; the /auto commands below need it",
			"none", "no model calls", "autopus.yaml and the harness files of each selected platform"))
	}
	if env.PipelinePlatform.Platform == "" {
		purpose := "no AI CLI (claude, codex, agy, opencode, omp) is on PATH; install one, then check the setup"
		if len(env.InstalledCLIs) > 0 {
			purpose = "auto pipeline run needs claude, codex, agy, or omp, and none is installed; " +
				"install one, then check the setup"
		}
		add(terminalAction("doctor", "", env.Dir, "auto doctor", purpose,
			"none", "no model calls; it runs the installed CLIs' version checks", "none"))
	}
	actions = append(actions, specNextActions(specs, evidence, env)...)
	add(fixAction(env))
	add(planAction(env))
	if env.Desktop.State == desktopInstalled {
		action, ok := desktopConnectAction(env.Dir)
		add(action, ok)
	}
	return actions
}

// fixAction is a small change without a SPEC, shared with init.
func fixAction(env statusEnvironment) (statusNextAction, bool) {
	return routeAction("fix", `"<problem>"`, env.ConfiguredPlatforms, env.Dir,
		"fix something small without a SPEC: a reproduction test, a focused patch, and its verification",
		agentSessionCalls, "the working tree")
}

// planAction is new work planned as a SPEC, shared with init. Planning runs
// the SPEC review when the review gate is on.
func planAction(env statusEnvironment) (statusNextAction, bool) {
	calls := agentSessionCalls
	if env.ReviewGate && len(env.ReviewProviders) > 0 {
		calls += " and, for the SPEC review, " + statusSingleLine(strings.Join(env.ReviewProviders, ", "))
	}
	return routeAction("plan", `"<outcome>"`, env.ConfiguredPlatforms, env.Dir,
		"plan new work as a SPEC before implementing it", calls, ".autopus/specs (a new SPEC)")
}

// desktopConnectAction is the optional Desktop connection, shared with init.
func desktopConnectAction(dir string) (statusNextAction, bool) {
	action, ok := terminalAction("desktop_connect", "", dir, "auto connect",
		"connect this machine to the Autopus server through Autopus Desktop; the ADK works without it",
		"none", "Autopus server sign-in, workspace selection, and OpenAI OAuth",
		"the local Autopus connection config and credentials")
	action.Optional = true
	return action, ok
}

// specNextActions picks the SPECs that most need a command: a stopped run
// first, then stale verification, then implementation, then review.
func specNextActions(specs []specEntry, evidence []statusEvidence, env statusEnvironment) []statusNextAction {
	type ranked struct {
		rank   int
		action statusNextAction
	}
	var candidates []ranked
	for i, entry := range specs {
		if action, rank, ok := specNextAction(entry, evidence[i], env); ok {
			candidates = append(candidates, ranked{rank: rank, action: action})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].rank < candidates[j].rank })
	var actions []statusNextAction
	for i := 0; i < len(candidates) && i < statusMaxSpecActions; i++ {
		actions = append(actions, candidates[i].action)
	}
	return actions
}

// specNextAction builds a SPEC's command from validated values only: a SPEC
// ID the pipeline accepts and a known platform (REQ-SEV-NEXT-04).
func specNextAction(entry specEntry, evidence statusEvidence, env statusEnvironment) (statusNextAction, int, bool) {
	if pipeline.ValidateSpecID(entry.id) != nil {
		return statusNextAction{}, 0, false
	}
	execution, platform := evidence.Execution, env.PipelinePlatform.Platform
	pipelineAction := func(id, command, provider, purpose string) (statusNextAction, bool) {
		action, ok := terminalAction(id, entry.id, evidence.stateRoot, command, purpose, provider,
			"model calls through the "+pipelinePlatformLabel(provider)+" CLI", pipelineRunWrites)
		return action, ok
	}
	switch {
	case evidence.Label == evidenceFailed && execution.ExecutionOwner == "unknown" &&
		execution.Resume != nil && execution.Resume.Admitted:
		// The run can resume, but only the user knows under which owner; a new
		// run would throw its progress away. The evidence note says what to run.
		return statusNextAction{}, 0, false
	case evidence.Label == evidenceFailed && execution.Resume != nil && execution.Resume.Command != "":
		resume := execution.Resume
		action, ok := pipelineAction("resume", resume.Command, execution.Platform,
			fmt.Sprintf("resume the stopped run of %s: %s", entry.id, statusResumeSummary(resume)))
		return action, 0, ok
	case evidence.Label == evidenceFailed && platform != "":
		why := "the stopped run cannot resume"
		if resume := execution.Resume; resume != nil && !resume.Admitted {
			why += ": " + resume.Reason
		} else if resume != nil && resume.CommandNote != "" {
			why += ": " + resume.CommandNote
		}
		action, ok := pipelineAction("rerun", "auto pipeline run "+entry.id+" --platform "+platform, platform,
			fmt.Sprintf("start a new run of %s; %s", entry.id, why))
		return action, 1, ok
	case (evidence.Label == evidenceStale || evidence.Label == evidenceUnverified) && platform != "":
		action, ok := pipelineAction("reverify", "auto pipeline run "+entry.id+" --platform "+platform, platform,
			fmt.Sprintf("run %s again to verify the current code (%s: %s)",
				entry.id, evidence.Label, strings.Join(evidence.Verification.Reasons, "; ")))
		rank := 2
		if evidence.Label == evidenceUnverified {
			rank = 5
		}
		return action, rank, ok
	case evidence.Label != evidenceNone:
		return statusNextAction{}, 0, false
	}
	moduleRoot := statusAbsDir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(entry.path)))))
	env = moduleEnvironment(env, moduleRoot)
	switch normalizeSpecStatus(entry.status) {
	case "approved", "in-progress":
		action, ok := routeAction("go", entry.id, env.ConfiguredPlatforms, moduleRoot,
			fmt.Sprintf("implement %s in an agent session", entry.id),
			agentSessionCalls, "the working tree and the SPEC documents")
		action.ID, action.SpecID = "implement", entry.id
		return action, 3, ok
	case "draft":
		if !env.ReviewGate {
			return statusNextAction{}, 0, false
		}
		providers := "the configured review providers"
		if len(env.ReviewProviders) > 0 {
			providers = statusSingleLine(strings.Join(env.ReviewProviders, ", "))
		}
		action, ok := terminalAction("review", entry.id, moduleRoot, "auto spec review "+entry.id,
			fmt.Sprintf("review the draft %s; a passing review marks it approved", entry.id), providers,
			"model calls to "+providers, "review.md, review-findings.json, and review-receipt.json; spec.md status on approval")
		return action, 4, ok
	}
	return statusNextAction{}, 0, false
}

// moduleEnvironment is env as seen from a module SPEC's root: a module with
// its own autopus.yaml routes and reviews by its own platforms and review
// providers. What is installed stays the machine's.
func moduleEnvironment(env statusEnvironment, moduleRoot string) statusEnvironment {
	if moduleRoot == env.Dir {
		return env
	}
	if _, err := os.Stat(filepath.Join(moduleRoot, "autopus.yaml")); err != nil {
		return env
	}
	cfg, err := config.LoadPreview(moduleRoot)
	if err != nil {
		return env
	}
	env.ConfiguredPlatforms = append([]string{}, cfg.Platforms...)
	env.ReviewGate = cfg.Spec.ReviewGate.Enabled
	env.ReviewProviders = append([]string{}, cfg.Spec.ReviewGate.Providers...)
	return env
}

// terminalAction is a command run in a shell from cwd. Its shell line changes
// into cwd first, quoted, so it works from any directory.
func terminalAction(id, specID, cwd, command, purpose, provider, calls, writes string) (statusNextAction, bool) {
	if !commandSafeDir(cwd) {
		return statusNextAction{}, false
	}
	return statusNextAction{
		ID: id, SpecID: specID, Cwd: cwd, Command: command, Shell: "cd " + shellQuote(cwd) + " && " + command,
		Purpose: purpose, RunsIn: runsInTerminal, Provider: provider, ExternalCalls: calls, Writes: writes,
	}, true
}

// routeAction is an agent-session route in every configured platform's
// syntax; the session is opened in cwd.
func routeAction(route, arg string, platforms []string, cwd, purpose, calls, writes string) (statusNextAction, bool) {
	if !commandSafeDir(cwd) {
		return statusNextAction{}, false
	}
	forms := autoRouteForms(platforms, route, arg)
	return statusNextAction{
		ID: route, Cwd: cwd, Command: forms[0].Command, Forms: forms, Purpose: purpose,
		RunsIn: runsInAgentSession, Provider: agentSessionProvider, ExternalCalls: calls, Writes: writes,
	}, true
}

// statusResumeSummary says what an admitted resume restores and dispatches again.
func statusResumeSummary(resume *statusResume) string {
	restores, reruns := "nothing", "none"
	if len(resume.Restores) > 0 {
		restores = strings.Join(resume.Restores, ", ")
	}
	if len(resume.Reruns) > 0 {
		reruns = strings.Join(resume.Reruns, ", ")
	}
	summary := fmt.Sprintf("the engine admits it; restores %s; dispatches again %s", restores, reruns)
	if resume.CommandNote != "" {
		summary += "; " + resume.CommandNote
	}
	return summary
}
