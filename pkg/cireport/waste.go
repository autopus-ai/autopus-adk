package cireport

import (
	"fmt"
	"sort"
)

// Duplicate is a sampled push run whose commit tree a successful PR run of
// the same workflow had already tested.
type Duplicate struct {
	Workflow  string  `json:"workflow"`
	PushRunID int64   `json:"push_run_id"`
	PRRunID   int64   `json:"pr_run_id"`
	SHA       string  `json:"sha"`
	Tree      string  `json:"tree"`
	Minutes   float64 `json:"minutes"`
}

// Superseded is a PR run that kept running after a newer run of the same
// workflow and pull request (or branch) was created.
type Superseded struct {
	Workflow      string  `json:"workflow"`
	RunID         int64   `json:"run_id"`
	NewerRunID    int64   `json:"newer_run_id"`
	PR            int     `json:"pr"`
	Branch        string  `json:"branch"`
	WastedMinutes float64 `json:"wasted_minutes"`
}

// duplicates compares trees, never SHAs alone: a squash or merge commit on
// the default branch has a new SHA but the same tree the PR head had when
// the PR was up to date. A tree that was not looked up is not compared.
func duplicates(ds Dataset) []Duplicate {
	out := []Duplicate{}
	for _, push := range ds.Sampled {
		tree, ok := ds.Trees[push.HeadSHA]
		if push.Event != "push" || !ok {
			continue
		}
		var match *Run
		for i := range ds.Runs {
			pr := ds.Runs[i]
			if !isPREvent(pr.Event) || pr.Conclusion != "success" || pr.WorkflowID != push.WorkflowID ||
				!pr.CreatedAt.Before(push.CreatedAt) || ds.Trees[pr.HeadSHA] != tree {
				continue
			}
			if match == nil || pr.CreatedAt.After(match.CreatedAt) {
				match = &ds.Runs[i]
			}
		}
		if match == nil {
			continue
		}
		var minutes float64
		for _, j := range ds.Jobs[push.ID] {
			minutes += j.RunSeconds() / 60
		}
		out = append(out, Duplicate{Workflow: push.Name, PushRunID: push.ID, PRRunID: match.ID, SHA: push.HeadSHA,
			Tree: tree, Minutes: round3(minutes)})
	}
	return out
}

// superseded walks each workflow's PR runs per pull request in creation
// order. A run that was not cancelled although the next one was created
// before it finished wasted the wall time between the two.
func superseded(runs []Run) []Superseded {
	groups := map[string][]Run{}
	for _, r := range runs {
		if !isPREvent(r.Event) {
			continue
		}
		key := fmt.Sprintf("%d/branch:%s", r.WorkflowID, r.HeadBranch)
		if r.PR() > 0 {
			key = fmt.Sprintf("%d/pr:%d", r.WorkflowID, r.PR())
		}
		groups[key] = append(groups[key], r)
	}
	out := []Superseded{}
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool { return group[i].CreatedAt.Before(group[j].CreatedAt) })
		for i := 0; i+1 < len(group); i++ {
			old, newer := group[i], group[i+1]
			if old.Conclusion == "cancelled" || !newer.CreatedAt.Before(old.UpdatedAt) {
				continue
			}
			out = append(out, Superseded{Workflow: old.Name, RunID: old.ID, NewerRunID: newer.ID, PR: old.PR(),
				Branch: old.HeadBranch, WastedMinutes: round3(old.UpdatedAt.Sub(newer.CreatedAt).Minutes())})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RunID < out[j].RunID })
	return out
}
