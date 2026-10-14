package cli

import (
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

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
				// The plan reports a degraded or missing route as a blocker, so
				// this never happens; if it did, converting would drop the
				// route silently, so the tab stops instead.
				return nil, ompProfileUnavailableError{blockers: []string{ompProfileBlocker(row.Agent, "route_unresolved")}}
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

// unattested names every start route candidate no built-in profile
// declares as agent=model, the routes a metadata-light catalog cannot attest
// once they are pins; "" when there is none.
func (tab *ompAutopusTab) unattested(declared map[string]string) string {
	var pairs []string
	for _, agent := range config.OMPAutopusAgentNames() {
		for _, candidate := range tab.start[agent] {
			if pair := agent + "=" + candidate.Selector; declared[candidate.Selector] == "" && !slices.Contains(pairs, pair) {
				pairs = append(pairs, pair)
			}
		}
	}
	return strings.Join(pairs, ", ")
}

func (tab *ompAutopusTab) setStart(agent string, candidates []config.RoleModelCandidateConf, selector, thinking string) {
	tab.start[agent] = append([]config.RoleModelCandidateConf(nil), candidates...)
	tab.labels[agent], tab.thinking[agent] = selector+" · "+thinking, thinking
}
