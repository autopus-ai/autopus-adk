package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompAutopusSession is the model choice that drops an agent's pin, and for
// task, makes it follow planner. No selector is ever this value.
const ompAutopusSession = "\x00session"

// ompAutopusTab is one run of the autopus-mode OMP tab (SPEC-MODELUX-003
// REQ-MUX3-13, OMP Autopus Tab Contract). start holds every agent's route
// before the edit, final every canonical agent's after it; an empty route is
// the OMP session model.
type ompAutopusTab struct {
	start, final ompAgentRoutes
	labels       map[string]string
	thinking     map[string]string
	// profile is the profile an applied edit converts from; ownTask reports
	// an agents.task the start policy holds, and taskEdit the operator's
	// choice for it (empty: follow planner).
	profile  string
	ownTask  bool
	taskEdit *[]config.RoleModelCandidateConf
	choices  []ompCustomModelChoice
}

// ompAutopus runs the OMP tab in autopus mode. It asks no profile or family:
// it starts from the current pins or the current profile's effective routes,
// lets the operator pick a model from any provider per role group or agent,
// and saves the result as a pins-only policy through the OMP plan, write, and
// activation path. A pre-plan that cannot start ends the tab before the
// method select, with nothing written or activated.
func (f *composeFormRun) ompAutopus() error {
	runner := f.options.OMP.deps.newRunner()
	tab, err := f.ompAutopusStart(runner)
	if err != nil {
		return f.ompFailed(err)
	}
	perAgent, err := composeSelect(f.cmd, f.form.Method, "",
		[]composeOption[bool]{{f.form.MethodGroups, false}, {f.form.MethodAgents, true}}, false)
	if err != nil {
		return err
	}
	tab.choices, err = installedOMPAttestedModelChoices(commandContext(f.cmd), f.out, runner, ompBuiltinDeclaredFamilies(), f.form.OMPAttestedOnly)
	if err == nil && len(tab.choices) == 0 {
		err = ompCatalogUnavailableError{reason: "no_usable_model"}
	}
	if err != nil {
		return f.ompFailed(composeOMPCatalogError{err: err})
	}
	step := f.ompAutopusGroups
	if perAgent {
		step = f.ompAutopusAgents
	}
	if err := step(tab); err != nil {
		return f.ompFailed(err)
	}
	if !tab.changed() {
		fmt.Fprintln(f.out, f.Text.NoChange)
		return nil
	}
	return f.ompAutopusApply(tab, runner)
}

// ompAutopusStart reads the start routes: the pins of a pins-only policy,
// the effective routes of a pin-less plan of the current profile, or the
// session model everywhere when there is no policy.
func (f *composeFormRun) ompAutopusStart(runner omp.OMPModelCatalogRunner) (*ompAutopusTab, error) {
	tab := &ompAutopusTab{start: ompAgentRoutes{}, final: ompAgentRoutes{}, labels: map[string]string{}, thinking: map[string]string{}}
	policy := f.shared.cfg.RoleModelPolicy
	_, tab.ownTask = policy.Agents[config.OMPAutopusDefaultAgent]
	if policy.Profile != "" {
		opts, err := newOMPProfileApplyOptions(policy.Profile, "", nil, false, false)
		if err != nil {
			return nil, err
		}
		pre, _, err := f.ompPlan(opts, runner)
		if err != nil {
			return nil, err
		}
		if len(pre.Blockers) > 0 {
			return nil, ompProfileUnavailableError{blockers: pre.Blockers}
		}
		tab.profile = policy.Profile
		for _, row := range pre.Agents {
			if row.EffectiveSelector == "" {
				continue // a degraded built-in route already runs on the session model
			}
			candidates := make([]config.RoleModelCandidateConf, 0, len(row.Candidates))
			for _, candidate := range row.Candidates {
				candidates = append(candidates, config.RoleModelCandidateConf(candidate))
			}
			tab.setStart(row.Agent, candidates, row.EffectiveSelector, row.EffectiveThinking)
		}
	} else {
		for agent, override := range policy.Agents {
			if len(override.Candidates) > 0 {
				first := override.Candidates[0]
				tab.setStart(agent, override.Candidates, first.Selector, first.Thinking)
			}
		}
		if planner := tab.start["planner"]; !tab.ownTask && len(planner) > 0 {
			tab.setStart(config.OMPAutopusDefaultAgent, planner, planner[0].Selector, planner[0].Thinking)
		}
	}
	for _, agent := range config.CanonicalAgentNames() {
		tab.final[agent] = tab.start[agent]
	}
	return tab, nil
}

func (tab *ompAutopusTab) setStart(agent string, candidates []config.RoleModelCandidateConf, selector, thinking string) {
	tab.start[agent] = append([]config.RoleModelCandidateConf(nil), candidates...)
	tab.labels[agent], tab.thinking[agent] = selector+" · "+thinking, thinking
}

// ompAutopusApply previews and confirms the pins-only policy the edit saves:
// the conversion line when a profile is replaced, the old-binary line when
// pins remain, then the plan's routes.
func (f *composeFormRun) ompAutopusApply(tab *ompAutopusTab, runner omp.OMPModelCatalogRunner) error {
	pins := ompPinsOnlyTarget(tab.start, tab.final, tab.profile != "", tab.ownTask, tab.taskEdit)
	opts, err := newOMPPinsOnlyApplyOptions(pins)
	if err != nil {
		return f.ompFailed(err)
	}
	if tab.profile != "" {
		fmt.Fprintf(f.out, f.form.OMPConvert+"\n", terminalSafe(tab.profile))
	}
	if len(pins) > 0 {
		fmt.Fprintln(f.out, f.form.OMPOldBinary)
	}
	preview, candidate, err := f.ompPlan(opts, runner)
	if err != nil {
		return f.ompFailed(err)
	}
	if len(preview.Agents) > 0 {
		renderOMPQualitySummary(f.out, preview)
	}
	if len(preview.Blockers) > 0 {
		return f.ompFailed(ompProfileUnavailableError{blockers: preview.Blockers})
	}
	return f.ompConfirm(opts, runner, candidate)
}

// ompAutopusGroups asks every role group once, in Role Group Contract order.
func (f *composeFormRun) ompAutopusGroups(tab *ompAutopusTab) error {
	for _, group := range composeRoleGroups {
		label := group.Label(f.shared.lang)
		if err := f.ompAutopusAsk(tab, label+" — "+strings.Join(group.Agents, ", "), label, group.Agents); err != nil {
			return err
		}
	}
	return nil
}

// ompAutopusAgents lists the 16 canonical agents, and task when the policy
// pins it, behind 완료, each with its current model, until the operator is
// done.
func (f *composeFormRun) ompAutopusAgents(tab *ompAutopusTab) error {
	for {
		options := []composeOption[string]{{f.form.ListDone, ""}}
		for _, group := range composeRoleGroups {
			for _, agent := range group.Agents {
				options = append(options, composeOption[string]{agent + " — " + tab.label(f, []string{agent}), agent})
			}
		}
		if tab.ownTask {
			options = append(options, composeOption[string]{"task — " + tab.label(f, []string{config.OMPAutopusDefaultAgent}), config.OMPAutopusDefaultAgent})
		}
		agent, err := composeSelect(f.cmd, f.form.List, "", options, "")
		if err != nil || agent == "" {
			return err
		}
		capability, err := ompCustomAgentCapability(agent)
		if err != nil {
			return err
		}
		if err := f.ompAutopusAsk(tab, agent+" ("+capability+")", agent, []string{agent}); err != nil {
			return err
		}
	}
}

// ompAutopusAsk asks the model for agents, then its thinking level when the
// model reports more than one, and records the answer.
func (f *composeFormRun) ompAutopusAsk(tab *ompAutopusTab, title, subject string, agents []string) error {
	models, err := tab.modelOptions(f, agents)
	if err != nil {
		return err
	}
	selector, err := composeSelect(f.cmd, title, "", models, "")
	if err != nil || selector == "" {
		return err
	}
	if selector == ompAutopusSession {
		tab.set(f, agents, nil)
		return nil
	}
	choice := tab.choices[slices.IndexFunc(tab.choices, func(c ompCustomModelChoice) bool { return c.selector() == selector })]
	thinking := choice.Thinking[0]
	for _, agent := range agents {
		if current := tab.thinking[agent]; slices.Contains(choice.Thinking, current) {
			thinking = current
			break
		}
	}
	if len(choice.Thinking) > 1 {
		levels := make([]composeOption[string], 0, len(choice.Thinking))
		for _, level := range choice.Thinking {
			levels = append(levels, composeOption[string]{level, level})
		}
		if thinking, err = composeSelect(f.cmd, fmt.Sprintf(f.form.OMPThinking, subject), "", levels, thinking); err != nil {
			return err
		}
	}
	tab.set(f, agents, []config.RoleModelCandidateConf{{Selector: selector, Thinking: thinking, Family: choice.Family}})
	return nil
}

// modelOptions lists 그대로, the session model (follow planner for task), and
// every installed model all agents accept: on a strict catalog the ones whose
// capabilities cover each agent's, on a metadata-light one every attested
// model.
func (tab *ompAutopusTab) modelOptions(f *composeFormRun, agents []string) ([]composeOption[string], error) {
	session := f.form.OMPSessionKeep
	if slices.Equal(agents, []string{config.OMPAutopusDefaultAgent}) {
		session = f.form.OMPFollowPlanner
	}
	options := []composeOption[string]{{f.form.Keep + " — " + tab.label(f, agents), ""}, {session, ompAutopusSession}}
	capabilities := make([]string, 0, len(agents))
	for _, agent := range agents {
		capability, err := ompCustomAgentCapability(agent)
		if err != nil {
			return nil, err
		}
		capabilities = append(capabilities, capability)
	}
	for _, choice := range tab.choices {
		if !slices.ContainsFunc(capabilities, func(capability string) bool {
			return requireOMPCustomCapability(choice, true, capability, tab.choices) != nil
		}) {
			options = append(options, composeOption[string]{choice.selector(), choice.selector()})
		}
	}
	return options, nil
}

// label names the agents' current models, each distinct one once.
func (tab *ompAutopusTab) label(f *composeFormRun, agents []string) string {
	var labels []string
	for _, agent := range agents {
		label := tab.labels[agent]
		if label == "" {
			label = f.form.OMPSessionModel
		}
		if !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ", ")
}

// set records one answer: a single pin, or none for the session model. task
// keeps its own edit, which an empty route turns into following planner.
func (tab *ompAutopusTab) set(f *composeFormRun, agents []string, route []config.RoleModelCandidateConf) {
	for _, agent := range agents {
		label, thinking := "", ""
		if len(route) > 0 {
			label, thinking = route[0].Selector+" · "+route[0].Thinking, route[0].Thinking
		}
		if agent == config.OMPAutopusDefaultAgent {
			edit := append([]config.RoleModelCandidateConf{}, route...)
			tab.taskEdit = &edit
			if label == "" {
				label = f.form.OMPFollowPlanner
			}
		} else {
			tab.final[agent] = route
		}
		tab.labels[agent], tab.thinking[agent] = label, thinking
	}
}

// changed reports an edit that differs from the start.
func (tab *ompAutopusTab) changed() bool {
	for _, agent := range config.CanonicalAgentNames() {
		if !slices.Equal(tab.final[agent], tab.start[agent]) {
			return true
		}
	}
	return tab.taskEdit != nil && !slices.Equal(*tab.taskEdit, tab.start[config.OMPAutopusDefaultAgent])
}
