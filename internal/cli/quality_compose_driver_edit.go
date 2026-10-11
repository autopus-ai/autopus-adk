package cli

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeDriverStep edits a composition in place. It returns false when
// input ends inside a step that cannot finish without its answers, which
// keeps the session.
type composeDriverStep func(composition map[string]string) bool

// edit runs step over a copy of composition and plans the result. ok is false
// when the session keeps: input ended inside step, or the edit equals the
// starting composition, which writes nothing and asks nothing (REQ-MUX-07).
func (d *composeDriverState) edit(step composeDriverStep, composition map[string]string) (composeDriverPlan, bool, error) {
	working := maps.Clone(composition)
	if working == nil {
		working = map[string]string{}
	}
	if !step(working) {
		fmt.Fprintln(d.out, d.Text.Cancelled)
		return composeDriverPlan{}, false, nil
	}
	plan, err := d.Planner.PlanEdit(working)
	if err != nil {
		return composeDriverPlan{}, false, err
	}
	if plan.NoChange {
		fmt.Fprintln(d.out, d.Text.NoChange)
		return composeDriverPlan{}, false, nil
	}
	return plan, true, nil
}

// editAgain is the edit menu `e` opens. g or a edits the candidate's
// composition further, and Enter returns to the same preview. Input end or a
// third invalid answer keeps.
func (d *composeDriverState) editAgain(plan composeDriverPlan) (composeDriverPlan, bool, error) {
	for range composeDriverMaxInvalid {
		fmt.Fprintln(d.out, d.Text.EditMenu)
		answer, ok := d.ask()
		if !ok {
			fmt.Fprintln(d.out, d.Text.Cancelled)
			return composeDriverPlan{}, false, nil
		}
		switch strings.ToLower(answer) {
		case "":
			return plan, true, nil
		case "g":
			return d.edit(d.groupStep, plan.Composition)
		case "a":
			return d.edit(d.advancedStep, plan.Composition)
		}
	}
	fmt.Fprintln(d.out, d.Text.InvalidKeep)
	return composeDriverPlan{}, false, nil
}

// groupStep asks one tier per role group in contract order. A group keeps
// every member's tier unless the operator selects one (REQ-MUX-03).
func (d *composeDriverState) groupStep(composition map[string]string) bool {
	fmt.Fprintln(d.out, d.Text.GroupsHeading)
	for index := range d.Groups.Groups(composition) {
		group := d.Groups.Groups(composition)[index]
		fmt.Fprintf(d.out, "%s  %s  %s\n",
			group.Label, strings.Join(group.Members, ", "), fmt.Sprintf(d.Text.GroupCurrent, group.Display))
		tier, ok := d.askTier()
		if !ok {
			return false
		}
		if tier != "" {
			d.Groups.SetGroupTier(composition, index, tier)
		}
	}
	return true
}

// askTier reads one group answer: a number or a tier name in
// config.QualityTiers() order selects that tier. Enter keeps the group, and
// so does the third invalid answer (Role Group Contract); both return "".
func (d *composeDriverState) askTier() (string, bool) {
	tiers := config.QualityTiers()
	for range composeDriverMaxInvalid {
		fmt.Fprintf(d.out, "  %s\n", d.tierChoices(tiers))
		answer, ok := d.ask()
		if !ok {
			return "", false
		}
		if answer == "" {
			return "", true
		}
		if tier, valid := composeDriverTierAnswer(answer, tiers); valid {
			return tier, true
		}
	}
	return "", true
}

func composeDriverTierAnswer(answer string, tiers []string) (string, bool) {
	for index, tier := range tiers {
		if answer == strconv.Itoa(index+1) {
			return tier, true
		}
	}
	return config.NormalizeQualityTier(answer)
}

func (d *composeDriverState) tierChoices(tiers []string) string {
	choices := make([]string, 0, len(tiers)+1)
	for index, tier := range tiers {
		choices = append(choices, fmt.Sprintf("%d) %s", index+1, tier))
	}
	return strings.Join(append(choices, d.Text.GroupKeep), " ")
}

// advancedStep applies `agent=tier` lines until an empty line or input end
// (REQ-MUX-04). parseQualityToolTierAssignment validates each line; a
// rejected line prints its error and keeps every earlier edit. Input end
// finishes like an empty line: the confirmation that follows reads the same
// end and cancels, so nothing is written.
func (d *composeDriverState) advancedStep(composition map[string]string) bool {
	fmt.Fprintln(d.out, d.Text.AdvancedHeading)
	for {
		line, ok := d.ask()
		if !ok || line == "" {
			return true
		}
		agent, tier, err := parseQualityToolTierAssignment(line)
		if err != nil {
			fmt.Fprintf(d.out, "  %v\n", err)
			continue
		}
		composition[agent] = tier
		fmt.Fprintf(d.out, "  %s = %s   %s\n", agent, tier, d.groupOf(composition, agent))
	}
}

// groupOf names agent's group with its display recomputed after an edit.
func (d *composeDriverState) groupOf(composition map[string]string, agent string) string {
	for _, group := range d.Groups.Groups(composition) {
		if slices.Contains(group.Members, agent) {
			return group.Label + ": " + group.Display
		}
	}
	return ""
}
