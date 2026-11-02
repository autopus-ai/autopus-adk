package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// statusNextAction is one command status suggests, with where it runs, the
// provider it dispatches with, and what it calls and writes. Status never
// runs it; auto_run is always false so a consumer never has to infer that
// (REQ-SEV-NEXT-01).
type statusNextAction struct {
	ID            string              `json:"id"`
	SpecID        string              `json:"spec_id,omitempty"`
	Dir           string              `json:"dir,omitempty"`
	Command       string              `json:"command"`
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
func buildStatusNextActions(baseDir string, specs []specEntry, evidence []statusEvidence, env statusEnvironment) []statusNextAction {
	var actions []statusNextAction
	if !env.ConfigFound {
		actions = append(actions, statusNextAction{
			ID: "init", Command: "auto init",
			Purpose: "install the Autopus harness here; the /auto commands below need it",
			RunsIn:  runsInTerminal, Provider: "none", ExternalCalls: "no model calls",
			Writes: "autopus.yaml and the harness files of each selected platform",
		})
	}
	if len(env.InstalledCLIs) == 0 {
		actions = append(actions, statusNextAction{
			ID: "doctor", Command: "auto doctor",
			Purpose: "no AI CLI (claude, codex, agy, opencode, omp) is on PATH; install one, then check the setup",
			RunsIn:  runsInTerminal, Provider: "none",
			ExternalCalls: "no model calls; it runs the installed CLIs' version checks", Writes: "none",
		})
	}
	actions = append(actions, specNextActions(baseDir, specs, evidence, env)...)
	reviewCalls := agentSessionCalls
	if env.ReviewGate && len(env.ReviewProviders) > 0 {
		reviewCalls += " and, for the SPEC review, " + strings.Join(env.ReviewProviders, ", ")
	}
	actions = append(actions,
		routeAction("fix", `"<problem>"`, env,
			"fix something small without a SPEC: a reproduction test, a focused patch, and its verification",
			agentSessionCalls, "the working tree"),
		routeAction("plan", `"<outcome>"`, env,
			"plan new work as a SPEC before implementing it", reviewCalls, ".autopus/specs (a new SPEC)"),
	)
	if env.Desktop.State == desktopInstalled {
		actions = append(actions, statusNextAction{
			ID: "desktop_connect", Command: "auto connect", Optional: true,
			Purpose: "connect this machine to the Autopus server through Autopus Desktop; the ADK works without it",
			RunsIn:  runsInTerminal, Provider: "none",
			ExternalCalls: "Autopus server sign-in, workspace selection, and OpenAI OAuth",
			Writes:        "the local Autopus connection config and credentials",
		})
	}
	return actions
}

// specNextActions picks the SPECs that most need a command: a stopped run
// first, then stale verification, then implementation, then review.
func specNextActions(baseDir string, specs []specEntry, evidence []statusEvidence, env statusEnvironment) []statusNextAction {
	type ranked struct {
		rank   int
		action statusNextAction
	}
	var candidates []ranked
	for i, entry := range specs {
		if action, rank, ok := specNextAction(baseDir, entry, evidence[i], env); ok {
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

func specNextAction(baseDir string, entry specEntry, evidence statusEvidence, env statusEnvironment) (statusNextAction, int, bool) {
	platform := env.PipelinePlatform.Platform
	pipelineAction := statusNextAction{
		SpecID: entry.id, Dir: statusActionDir(baseDir, evidence.stateRoot),
		Command: "auto pipeline run " + entry.id, RunsIn: runsInTerminal, Provider: platform,
		ExternalCalls: "model calls through the " + pipelinePlatformLabel(platform) + " CLI",
		Writes:        pipelineRunWrites,
	}
	execution := evidence.Execution
	switch {
	case evidence.Label == evidenceFailed && execution.Resumable:
		pipelineAction.ID, pipelineAction.Command = "resume", execution.ResumeCommand
		if execution.Platform != "" {
			pipelineAction.Provider = execution.Platform
		}
		pipelineAction.ExternalCalls = "model calls through the " + pipelinePlatformLabel(pipelineAction.Provider) +
			" CLI for the phases still owed"
		pipelineAction.Purpose = fmt.Sprintf("resume the stopped run of %s: %s", entry.id, execution.ResumeReason)
		return pipelineAction, 0, true
	case evidence.Label == evidenceFailed:
		pipelineAction.ID = "rerun"
		pipelineAction.Purpose = fmt.Sprintf("start a new run of %s; the stopped run cannot resume: %s", entry.id, execution.ResumeReason)
		return pipelineAction, 1, true
	case evidence.Label == evidenceStale, evidence.Label == evidenceUnverified:
		pipelineAction.ID = "reverify"
		pipelineAction.Purpose = fmt.Sprintf("run %s again to verify the current code (%s: %s)",
			entry.id, evidence.Label, strings.Join(evidence.Verification.Reasons, "; "))
		rank := 2
		if evidence.Label == evidenceUnverified {
			rank = 5
		}
		return pipelineAction, rank, true
	case evidence.Label != evidenceNone:
		return statusNextAction{}, 0, false
	}
	switch normalizeSpecStatus(entry.status) {
	case "approved", "in-progress":
		action := routeAction("go", entry.id, env, fmt.Sprintf("implement %s in an agent session", entry.id),
			agentSessionCalls, "the working tree and the SPEC documents")
		action.ID, action.SpecID = "implement", entry.id
		return action, 3, true
	case "draft":
		if !env.ReviewGate {
			return statusNextAction{}, 0, false
		}
		providers := "the configured review providers"
		if len(env.ReviewProviders) > 0 {
			providers = strings.Join(env.ReviewProviders, ", ")
		}
		return statusNextAction{
			ID: "review", SpecID: entry.id, Command: "auto spec review " + entry.id,
			Purpose: fmt.Sprintf("review the draft %s; a passing review marks it approved", entry.id),
			RunsIn:  runsInTerminal, Provider: providers, ExternalCalls: "model calls to " + providers,
			Writes: "review.md, review-findings.json, and review-receipt.json; spec.md status on approval",
		}, 4, true
	}
	return statusNextAction{}, 0, false
}

// routeAction spells an agent-session route in every configured platform's
// syntax.
func routeAction(route, arg string, env statusEnvironment, purpose, calls, writes string) statusNextAction {
	forms := autoRouteForms(env.ConfiguredPlatforms, route, arg)
	return statusNextAction{
		ID: route, Command: forms[0].Command, Forms: forms, Purpose: purpose,
		RunsIn: runsInAgentSession, Provider: agentSessionProvider, ExternalCalls: calls, Writes: writes,
	}
}

// autoRouteForms spells one /auto route for the given platforms: Codex runs
// its native $codex-auto-<route> skill, the others take /auto <route>.
func autoRouteForms(platforms []string, route, arg string) []statusCommandForm {
	slash, codex := []string{}, []string{}
	for _, platform := range platforms {
		if platform == "codex" {
			codex = append(codex, platform)
		} else {
			slash = append(slash, platform)
		}
	}
	var forms []statusCommandForm
	if len(slash) > 0 || len(codex) == 0 {
		forms = append(forms, statusCommandForm{Platforms: slash, Command: "/auto " + route + " " + arg})
	}
	if len(codex) > 0 {
		forms = append(forms, statusCommandForm{Platforms: codex, Command: "$codex-auto-" + route + " " + arg})
	}
	return forms
}

func statusActionDir(baseDir, root string) string {
	if root == "" {
		return ""
	}
	rel, err := filepath.Rel(baseDir, root)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func renderStatusNext(out io.Writer, actions []statusNextAction) {
	tui.SectionHeader(out, "Next")
	_, _ = fmt.Fprintln(out, "  Nothing below runs by itself.")
	for i, action := range actions {
		_, _ = fmt.Fprintf(out, "  %d. %s\n", i+1, statusActionCommandText(action))
		_, _ = fmt.Fprintf(out, "     %s\n", action.Purpose)
		_, _ = fmt.Fprintf(out, "     %s\n", statusActionFactsText(action))
	}
}

func statusActionCommandText(action statusNextAction) string {
	text := action.Command
	if len(action.Forms) > 0 {
		text = autoRouteFormsText(action.Forms)
	}
	if action.Dir != "" {
		text += " (in " + action.Dir + ")"
	}
	return text
}

// autoRouteFormsText joins the forms, each followed by its platforms.
func autoRouteFormsText(forms []statusCommandForm) string {
	parts := make([]string, 0, len(forms))
	for _, form := range forms {
		if len(form.Platforms) == 0 {
			parts = append(parts, form.Command)
			continue
		}
		parts = append(parts, form.Command+" ("+strings.Join(form.Platforms, ", ")+")")
	}
	return strings.Join(parts, " · ")
}

func statusActionFactsText(action statusNextAction) string {
	var facts []string
	if action.Optional {
		facts = append(facts, "optional")
	}
	facts = append(facts, "runs in "+action.RunsIn, "provider: "+action.Provider,
		"calls: "+action.ExternalCalls, "writes: "+action.Writes)
	return strings.Join(facts, " · ")
}
