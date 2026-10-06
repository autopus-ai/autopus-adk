package harneval

import (
	"fmt"
	"sort"
	"strings"
)

// Transition kinds (REQ-HE-03). A task without a change has no transition.
const (
	TransitionRegression         = "regression"
	TransitionImproved           = "improved"
	TransitionNew                = "new"
	TransitionRetired            = "retired"
	TransitionTaskMissing        = "task_missing"
	TransitionExpectationChanged = "expectation_changed"
)

// Transition is one task-level change against the baseline.
type Transition struct {
	TaskID string `json:"task_id"`
	Kind   string `json:"kind"`
}

// CategoryTotal aggregates the executed surface tasks of one category.
type CategoryTotal struct {
	Category string `json:"category"`
	Passed   int    `json:"passed"`
	Total    int    `json:"total"`
}

// compare builds the evaluated result from the executed surface results:
// totals, categories, pass rates, the baseline transitions, and every
// evaluation-stage reason, sorted and unique. Only active surface tasks count
// toward the rates; the PR lane never executes agent tasks.
func compare(set *Set, baseline *Baseline, results map[string]bool) *Result {
	result := &Result{
		SchemaVersion: ResultSchemaV1,
		SetDigest:     SetDigest(set),
		Totals: Totals{
			DeclaredSurface: set.ActiveCount(KindSurface),
			ExecutedSurface: len(results),
			DeclaredAgent:   set.ActiveCount(KindAgent),
		},
	}
	byCategory := map[string]*CategoryTotal{}
	for _, task := range set.Tasks {
		passed, executed := results[task.ID]
		if !executed {
			continue
		}
		total := byCategory[task.Category]
		if total == nil {
			total = &CategoryTotal{Category: task.Category}
			byCategory[task.Category] = total
		}
		total.Total++
		if passed {
			total.Passed++
			result.Totals.PassedSurface++
		}
	}
	for _, total := range byCategory {
		result.Categories = append(result.Categories, *total)
	}
	sort.Slice(result.Categories, func(i, j int) bool { return result.Categories[i].Category < result.Categories[j].Category })
	result.PassRate = ratio(result.Totals.PassedSurface, result.Totals.ExecutedSurface)
	result.BaselinePassRate = baselinePassRate(baseline)
	if result.PassRate != nil && result.BaselinePassRate != nil {
		result.RegressionDelta = *result.PassRate - *result.BaselinePassRate
	}
	result.Transitions = transitions(set, baseline, results)
	var reasons []string
	for _, transition := range result.Transitions {
		if reason, failing := failingTransitions[transition.Kind]; failing {
			reasons = append(reasons, reason)
		}
	}
	if result.SetDigest != baseline.SetDigest {
		reasons = append(reasons, ReasonSetDigestMismatch)
	}
	if details := vacuity(set, results); len(details) > 0 {
		reasons = append(reasons, ReasonVacuous)
		result.Details = sortedUnique(details)
	}
	result.FailureReasons = sortedUnique(reasons)
	result.Status = StatusPass
	if len(result.FailureReasons) > 0 {
		result.Status = StatusFail
	}
	return result
}

// failingTransitions maps the transition kinds that fail a run to their
// reason. new and retired change the set and so surface as
// set_digest_mismatch; improved alone keeps the run passing.
var failingTransitions = map[string]string{
	TransitionRegression:         ReasonRegression,
	TransitionExpectationChanged: ReasonExpectationChanged,
	TransitionTaskMissing:        ReasonTaskMissing,
}

// transitions classifies every task of the set and every baseline row. State
// decides new, retired, and task_missing; the executed result of a task that
// stayed active decides regression and improved; a different expectation
// digest is expectation_changed, independent of the others. The list is in
// task id order, then kind order, since one task can carry two kinds.
func transitions(set *Set, baseline *Baseline, results map[string]bool) []Transition {
	rows := make(map[string]BaselineRow, len(baseline.Rows))
	for _, row := range baseline.Rows {
		rows[row.ID] = row
	}
	var list []Transition
	add := func(id, kind string) { list = append(list, Transition{TaskID: id, Kind: kind}) }
	inSet := make(map[string]bool, len(set.Tasks))
	for _, task := range set.Tasks {
		inSet[task.ID] = true
		row, known := rows[task.ID]
		if known && ExpectationDigest(task) != row.ExpectationDigest {
			add(task.ID, TransitionExpectationChanged)
		}
		active, wasActive := task.Status.State == StateActive, known && row.State == StateActive
		switch {
		case active && !wasActive:
			add(task.ID, TransitionNew)
		case !active && (wasActive || !known):
			add(task.ID, TransitionRetired)
		case active:
			passed, executed := results[task.ID]
			if executed && row.Result == ResultPass && !passed {
				add(task.ID, TransitionRegression)
			} else if executed && row.Result == ResultFail && passed {
				add(task.ID, TransitionImproved)
			}
		}
	}
	for _, row := range baseline.Rows {
		if !inSet[row.ID] && row.State == StateActive {
			add(row.ID, TransitionTaskMissing)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].TaskID != list[j].TaskID {
			return list[i].TaskID < list[j].TaskID
		}
		return list[i].Kind < list[j].Kind
	})
	return list
}

// vacuity explains why a PR-lane run would prove nothing: fewer active
// surface tasks than their floor, an executed surface set that differs from
// the declared active set, or fewer active agent tasks than their floor. The
// PR lane never runs agent tasks, so only their floor applies here.
func vacuity(set *Set, results map[string]bool) []string {
	floors := set.Manifest.Floors
	var details, missing []string
	declared := 0
	for _, task := range set.Tasks {
		if task.Kind != KindSurface || task.Status.State != StateActive {
			continue
		}
		declared++
		if _, executed := results[task.ID]; !executed {
			missing = append(missing, task.ID)
		}
	}
	if declared < floors.SurfaceTasks {
		details = append(details, fmt.Sprintf("active_surface_tasks=%d floor=%d", declared, floors.SurfaceTasks))
	}
	if len(missing) > 0 {
		details = append(details, fmt.Sprintf("executed=%d declared=%d missing=[%s]",
			len(results), declared, strings.Join(missing, ",")))
	}
	if agents := set.ActiveCount(KindAgent); agents < floors.AgentTasks {
		details = append(details, fmt.Sprintf("active_agent_tasks=%d floor=%d", agents, floors.AgentTasks))
	}
	return details
}

// baselinePassRate counts the baseline rows recorded as active surface tasks;
// a row keeps its historical kind, so a deleted task still counts.
func baselinePassRate(baseline *Baseline) *float64 {
	passed, total := 0, 0
	for _, row := range baseline.Rows {
		if row.Kind == KindSurface && row.State == StateActive {
			total++
			if row.Result == ResultPass {
				passed++
			}
		}
	}
	return ratio(passed, total)
}

// ratio is part/whole, or nil when whole is zero, so no NaN is ever written.
func ratio(part, whole int) *float64 {
	if whole == 0 {
		return nil
	}
	value := float64(part) / float64(whole)
	return &value
}
