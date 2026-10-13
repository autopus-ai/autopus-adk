package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

// ompAutopusRoute is one agent's route in the pin-less pre-plan: the model
// and thinking level the profile gives it before any pin.
type ompAutopusRoute struct{ selector, thinking string }

// ompAutopusEdit is one autopus-mode pin edit (SPEC-MODELUX-003
// REQ-MUX3-13): the pre-plan's routes, the installed model choices, and the
// pins chosen so far in --agent value form, keyed by canonical agent.
type ompAutopusEdit struct {
	routes  map[string]ompAutopusRoute
	choices []ompCustomModelChoice
	pins    map[string]string
}

// ompAutopus pins models per canonical agent when OMP serves the 16 Autopus
// agents. It plans the profile once without pins, so every select can name
// the route a pin would replace; a blocker or a failure there ends the tab
// before the method select with nothing written or activated. The chosen
// pins then travel the plan, preview, confirmation, and apply path of the
// native pins.
func (f *composeFormRun) ompAutopus(name, family string, runner omp.OMPModelCatalogRunner) error {
	base, err := newOMPProfileApplyOptions(name, family, nil, false, false)
	if err != nil {
		return f.ompFailed(err)
	}
	pre, _, err := f.ompPlan(base, runner)
	if err != nil {
		return f.ompFailed(err)
	}
	if len(pre.Blockers) > 0 {
		return f.ompFailed(ompProfileUnavailableError{blockers: pre.Blockers})
	}
	perAgent, err := composeSelect(f.cmd, f.form.Method, "",
		[]composeOption[bool]{{f.form.MethodGroups, false}, {f.form.MethodAgents, true}}, false)
	if err != nil {
		return err
	}
	choices, _, err := installedOMPCustomModelChoices(commandContext(f.cmd), f.cmd, runner)
	if err == nil && len(choices) == 0 {
		err = ompCatalogUnavailableError{reason: "no_usable_model"}
	}
	if err != nil {
		return f.ompFailed(composeOMPCatalogError{err: err})
	}
	edit := &ompAutopusEdit{routes: ompAutopusRoutes(pre), choices: choices, pins: map[string]string{}}
	step := f.ompAutopusGroups
	if perAgent {
		step = f.ompAutopusAgents
	}
	if err := step(edit); err != nil {
		return f.ompFailed(err)
	}
	pins := edit.ordered()
	if len(pins) == 0 {
		fmt.Fprintln(f.out, "No agent was pinned; nothing changed.")
		return nil
	}
	opts, err := newOMPProfileApplyOptions(name, family, pins, false, false)
	if err != nil {
		return f.ompFailed(err)
	}
	preview, candidate, err := f.ompPlan(opts, runner)
	if err != nil {
		return f.ompFailed(err)
	}
	renderOMPQualitySummary(f.out, preview)
	if len(preview.Blockers) > 0 {
		return f.ompFailed(ompProfileUnavailableError{blockers: preview.Blockers})
	}
	return f.ompConfirm(opts, runner, candidate)
}

// ompAutopusRoutes keys the pre-plan's effective routes by agent.
func ompAutopusRoutes(pre ompProfileApplyPreviewPayload) map[string]ompAutopusRoute {
	routes := make(map[string]ompAutopusRoute, len(pre.Agents))
	for _, row := range pre.Agents {
		routes[row.Agent] = ompAutopusRoute{selector: row.EffectiveSelector, thinking: row.EffectiveThinking}
	}
	return routes
}

// ompAutopusGroups asks every role group in Role Group Contract order; a
// group's answer applies to each member.
func (f *composeFormRun) ompAutopusGroups(edit *ompAutopusEdit) error {
	for _, group := range composeRoleGroups {
		label := group.Label(f.shared.lang)
		if err := f.ompAutopusAsk(edit, label+" — "+strings.Join(group.Agents, ", "), label, group.Agents); err != nil {
			return err
		}
	}
	return nil
}

// ompAutopusAgents lists the 16 canonical agents behind 완료, each with its
// pin or the profile default, until the operator is done.
func (f *composeFormRun) ompAutopusAgents(edit *ompAutopusEdit) error {
	for {
		options := []composeOption[string]{{f.form.ListDone, ""}}
		for _, group := range composeRoleGroups {
			for _, agent := range group.Agents {
				label, pinned := edit.pins[agent]
				if !pinned {
					label = f.form.OMPProfileDefault
				}
				options = append(options, composeOption[string]{agent + " — " + label, agent})
			}
		}
		agent, err := composeSelect(f.cmd, f.form.List, "", options, "")
		if err != nil || agent == "" {
			return err
		}
		capability, err := ompCustomAgentCapability(agent)
		if err != nil {
			return err
		}
		if err := f.ompAutopusAsk(edit, agent+" ("+capability+")", agent, []string{agent}); err != nil {
			return err
		}
	}
}

// ompAutopusAsk asks the model, then the thinking level when more than one
// applies, for agents, and records the result. Keeping both defaults leaves
// the agents on the profile.
func (f *composeFormRun) ompAutopusAsk(edit *ompAutopusEdit, title, subject string, agents []string) error {
	models, err := edit.modelOptions(f.form.OMPProfileDefault, agents)
	if err != nil {
		return err
	}
	selector, err := composeSelect(f.cmd, title, "", models, "")
	if err != nil {
		return err
	}
	levels, keep := edit.levels(selector, agents)
	thinking := ""
	switch {
	case len(levels) > 1:
		options := make([]composeOption[string], 0, len(levels)+1)
		fallback := levels[0]
		if keep {
			options = append(options, composeOption[string]{fmt.Sprintf(f.form.OMPThinkingDefault, edit.defaultThinking(agents)), ""})
			fallback = ""
		}
		for _, level := range levels {
			options = append(options, composeOption[string]{level, level})
		}
		if thinking, err = composeSelect(f.cmd, fmt.Sprintf(f.form.OMPThinking, subject), "", options, fallback); err != nil {
			return err
		}
	case len(levels) == 1 && selector != "":
		thinking = levels[0]
	}
	edit.set(agents, selector, thinking)
	return nil
}

// modelOptions lists the profile default and the installed models every
// agent accepts under the native tab's catalog rule: a model without
// capability metadata is offered to all of them.
func (e *ompAutopusEdit) modelOptions(profileDefault string, agents []string) ([]composeOption[string], error) {
	capabilities := make([]string, 0, len(agents))
	for _, agent := range agents {
		capability, err := ompCustomAgentCapability(agent)
		if err != nil {
			return nil, err
		}
		capabilities = append(capabilities, capability)
	}
	options := []composeOption[string]{{profileDefault, ""}}
	for _, choice := range e.choices {
		refused := slices.ContainsFunc(capabilities, func(capability string) bool {
			return requireOMPCustomCapability(choice, true, capability, e.choices) != nil
		})
		if !refused {
			options = append(options, composeOption[string]{choice.selector(), choice.selector()})
		}
	}
	return options, nil
}

// levels returns the thinking levels to offer (Effort Option Contract, OMP
// row): the chosen model's, or with the profile default the levels every
// agent's pre-plan model reports. keep reports whether every agent's
// pre-plan thinking is among them, so the profile default can be offered.
func (e *ompAutopusEdit) levels(selector string, agents []string) ([]string, bool) {
	if selector != "" {
		levels := e.reported(selector)
		keep := !slices.ContainsFunc(agents, func(agent string) bool {
			return !slices.Contains(levels, e.routes[agent].thinking)
		})
		return levels, keep
	}
	var levels []string
	for i, agent := range agents {
		reported := e.reported(e.routes[agent].selector)
		if i == 0 {
			levels = slices.Clone(reported)
			continue
		}
		levels = slices.DeleteFunc(levels, func(level string) bool { return !slices.Contains(reported, level) })
	}
	return levels, true
}

// reported is the thinking levels the catalog reports for selector.
func (e *ompAutopusEdit) reported(selector string) []string {
	index := slices.IndexFunc(e.choices, func(choice ompCustomModelChoice) bool { return choice.selector() == selector })
	if index < 0 {
		return nil
	}
	return e.choices[index].Thinking
}

// defaultThinking names the agents' pre-plan thinking levels, each once.
func (e *ompAutopusEdit) defaultThinking(agents []string) string {
	var levels []string
	for _, agent := range agents {
		if level := e.routes[agent].thinking; !slices.Contains(levels, level) {
			levels = append(levels, level)
		}
	}
	return strings.Join(levels, ", ")
}

// set records one answer for agents. A chosen model pins it with the chosen
// level or the agent's pre-plan thinking; a changed thinking alone pins the
// agent's pre-plan model with it; anything else returns the agent to the
// profile.
func (e *ompAutopusEdit) set(agents []string, selector, thinking string) {
	for _, agent := range agents {
		delete(e.pins, agent)
		route := e.routes[agent]
		switch {
		case selector != "" && thinking != "":
			e.pins[agent] = selector + ":" + thinking
		case selector != "":
			e.pins[agent] = selector + ":" + route.thinking
		case thinking != "" && thinking != route.thinking && route.selector != "":
			e.pins[agent] = route.selector + ":" + thinking
		}
	}
}

// ordered returns the pins in --agent form and Role Group Contract order.
func (e *ompAutopusEdit) ordered() []string {
	var pins []string
	for _, group := range composeRoleGroups {
		for _, agent := range group.Agents {
			if pin, ok := e.pins[agent]; ok {
				pins = append(pins, agent+"="+pin)
			}
		}
	}
	return pins
}
