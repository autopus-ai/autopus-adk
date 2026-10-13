package cli

import "fmt"

// composeEffortTitle titles an effort select with its group or agent; the
// form text spells it the same in every locale (SPEC-MODELUX-003 Form Text).
const composeEffortTitle = "%s effort"

// effort asks the effort select of members after their tier select on a
// Claude Code or Codex tab (SPEC-MODELUX-003 REQ-MUX3-03). 그대로 comes
// first and is the default; each allowed level follows, labelled with the
// cells it leaves and `(자동)` on the automatic one. It asks nothing on 모든
// 도구 같게, when no member can take an effort, or when no level is
// allowed; an unreadable Codex catalog prints its notice, once per tab,
// instead of a select.
func (f *composeFormRun) effort(labeler composeLabeler, tiers map[string]string, subject string, members []string) error {
	tab := labeler.effort
	if tab == nil {
		return nil
	}
	agents := tab.affected(tiers, members)
	if len(agents) == 0 {
		return nil
	}
	levels, readable := tab.levels(tiers, agents)
	if !readable {
		if !tab.unreadNoted {
			fmt.Fprintln(f.out, f.form.EffortUnread)
			tab.unreadNoted = true
		}
		return nil
	}
	if len(levels) == 0 {
		return nil
	}
	options := []composeOption[string]{{f.form.Keep + " — " + labeler.cells(tab.candidate(tiers, tab.working), agents), ""}}
	chosen := make(map[string]map[string]string, len(levels))
	for _, level := range levels {
		efforts, automatic := tab.option(tiers, agents, level)
		label := labeler.cells(tab.candidate(tiers, efforts), agents)
		if automatic {
			label += " " + f.form.EffortAuto
		}
		options = append(options, composeOption[string]{label, level})
		chosen[level] = efforts
	}
	level, err := composeSelect(f.cmd, fmt.Sprintf(composeEffortTitle, subject), "", options, "")
	if err != nil {
		return err
	}
	if level != "" {
		tab.working = chosen[level]
	}
	return nil
}
