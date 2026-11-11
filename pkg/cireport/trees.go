package cireport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// fetchTrees looks up the trees the duplicate check compares: the head
// commits of the sampled push runs first, then those of the successful PR
// runs of the same workflows, newest first, up to MaxTreeLookups.
func fetchTrees(ctx context.Context, api API, opts CollectOptions, ds *Dataset) error {
	limit := opts.MaxTreeLookups
	if limit <= 0 {
		limit = defaultTreeLookups
	}
	shas := treeCandidates(ds.Sampled, ds.Runs)
	if len(shas) > limit {
		ds.Notes = append(ds.Notes, fmt.Sprintf("looked up %d of %d commit trees; duplicate work may be undercounted", limit, len(shas)))
		shas = shas[:limit]
	}
	for _, sha := range shas {
		data, err := api.Get(ctx, fmt.Sprintf("repos/%s/git/commits/%s", opts.Repo, sha))
		if errors.Is(err, ErrBudget) {
			return err
		}
		if err != nil {
			// A commit that is gone (force-pushed away) cannot be compared.
			continue
		}
		var commit struct {
			Tree struct {
				SHA string `json:"sha"`
			} `json:"tree"`
		}
		if json.Unmarshal(data, &commit) == nil && commit.Tree.SHA != "" {
			ds.Trees[sha] = commit.Tree.SHA
		}
	}
	return nil
}

func treeCandidates(sampled, all []Run) []string {
	seen := map[string]bool{}
	var shas []string
	add := func(sha string) {
		if sha != "" && validSHA(sha) && !seen[sha] {
			seen[sha] = true
			shas = append(shas, sha)
		}
	}
	workflows := map[int64]bool{}
	for _, r := range sampled {
		if r.Event == "push" {
			add(r.HeadSHA)
			workflows[r.WorkflowID] = true
		}
	}
	prs := append([]Run(nil), all...)
	sort.SliceStable(prs, func(i, j int) bool { return prs[i].CreatedAt.After(prs[j].CreatedAt) })
	for _, r := range prs {
		if isPREvent(r.Event) && r.Conclusion == "success" && workflows[r.WorkflowID] {
			add(r.HeadSHA)
		}
	}
	return shas
}

func validSHA(sha string) bool {
	for _, c := range sha {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return len(sha) > 0 && len(sha) <= 64
}

func isPREvent(event string) bool { return event == "pull_request" || event == "pull_request_target" }
