package cli

import (
	"maps"
	"strings"
)

// groups asks one select per role group in Role Group Contract order, each
// option labelled with the cells its candidate resolves (REQ-MUX2-05,
// REQ-MUX2-06). Keeping a group leaves its members' tiers; each later
// group's labels already include the earlier answers.
func (f *composeFormRun) groups(working map[string]string, labeler composeLabeler) (map[string]string, error) {
	edited := maps.Clone(working)
	for _, group := range composeRoleGroups {
		title := group.Label(f.shared.lang) + " — " + strings.Join(group.Agents, ", ")
		tier, err := composeSelect(f.cmd, title, "", labeler.groupOptions(edited, group, f.form.Keep), "")
		if err != nil {
			return nil, err
		}
		if tier != "" {
			for _, agent := range group.Agents {
				edited[agent] = tier
			}
		}
	}
	return edited, nil
}

// agents lists the 16 canonical agents in Role Group Contract order behind
// 완료, asks the picked agent's value, and lists again with every label
// refreshed, since one agent's tier can move the binding and with it the
// other agents' cells (REQ-MUX2-08). 완료 ends the edit.
func (f *composeFormRun) agents(working map[string]string, labeler composeLabeler) (map[string]string, error) {
	edited := maps.Clone(working)
	for {
		options := []composeOption[string]{{f.form.ListDone, ""}}
		for _, group := range composeRoleGroups {
			for _, agent := range group.Agents {
				options = append(options, composeOption[string]{labeler.agentEntry(edited, agent), agent})
			}
		}
		agent, err := composeSelect(f.cmd, f.form.List, "", options, "")
		if err != nil {
			return nil, err
		}
		if agent == "" {
			return edited, nil
		}
		tier, err := composeSelect(f.cmd, agent, "", labeler.agentOptions(edited, agent, f.form.Keep), "")
		if err != nil {
			return nil, err
		}
		if tier != "" {
			edited[agent] = tier
		}
	}
}
