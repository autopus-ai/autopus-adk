package cireport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// Collection bounds. A page holds at most 100 entries.
const (
	pageSize           = 100
	defaultRunPages    = 10
	defaultJobPages    = 3
	defaultTreeLookups = 200
	maxConcurrency     = 4
)

// CollectOptions bound one collection. Zero values take the defaults.
type CollectOptions struct {
	Repo      string
	Since     time.Time
	Until     time.Time
	LimitRuns int
	// Workflow keeps runs whose workflow name or file name matches.
	Workflow       string
	MaxRunPages    int
	MaxTreeLookups int
	Concurrency    int
}

// Collect lists the completed runs of the window, samples them down to
// LimitRuns, fetches the jobs of every attempt of each sampled run, and
// looks up the commit trees the duplicate check needs. A spent request
// budget ends collection early with Truncated set; any other API failure
// is returned.
func Collect(ctx context.Context, api API, opts CollectOptions) (Dataset, error) {
	ds := Dataset{Repo: opts.Repo, Since: opts.Since, Until: opts.Until, Jobs: map[int64][]Job{}, Trees: map[string]string{}}
	budget := func(err error, what string) error {
		if errors.Is(err, ErrBudget) {
			ds.Truncated = true
			ds.Notes = append(ds.Notes, "request budget spent while fetching "+what+"; the report covers what was fetched")
			return nil
		}
		return err
	}
	if data, err := api.Get(ctx, "repos/"+opts.Repo); err == nil {
		var meta struct {
			Private bool `json:"private"`
		}
		if json.Unmarshal(data, &meta) == nil {
			public := !meta.Private
			ds.Public = &public
		}
	} else if err := budget(err, "the repository"); err != nil {
		return ds, err
	}
	runs, err := listRuns(ctx, api, opts, &ds)
	if err = budget(err, "the run list"); err != nil {
		return ds, err
	}
	ds.Runs = runs
	ds.Sampled, ds.Scale = Sample(runs, opts.LimitRuns)
	if len(ds.Sampled) < len(runs) {
		ds.Notes = append(ds.Notes, fmt.Sprintf("sampled %d of %d runs; totals are scaled by %.2f", len(ds.Sampled), len(runs), ds.Scale))
	}
	if err := budget(fetchJobs(ctx, api, opts, &ds), "jobs"); err != nil {
		return ds, err
	}
	if err := budget(fetchTrees(ctx, api, opts, &ds), "commit trees"); err != nil {
		return ds, err
	}
	return ds, nil
}

func listRuns(ctx context.Context, api API, opts CollectOptions, ds *Dataset) ([]Run, error) {
	pages := opts.MaxRunPages
	if pages <= 0 {
		pages = defaultRunPages
	}
	var runs []Run
	for page := 1; page <= pages; page++ {
		q := url.Values{"per_page": {fmt.Sprint(pageSize)}, "page": {fmt.Sprint(page)},
			"created": {opts.Since.UTC().Format("2006-01-02") + ".." + opts.Until.UTC().Format("2006-01-02")}}
		data, err := api.Get(ctx, "repos/"+opts.Repo+"/actions/runs?"+q.Encode())
		if err != nil {
			return runs, err
		}
		var body struct {
			WorkflowRuns []Run `json:"workflow_runs"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return runs, fmt.Errorf("ci report: run list page %d: %w", page, err)
		}
		for _, r := range body.WorkflowRuns {
			// The API filters by date; the window may start mid-day.
			if r.Status == "completed" && !r.CreatedAt.Before(opts.Since) && matchesWorkflow(r, opts.Workflow) {
				runs = append(runs, r)
			}
		}
		if len(body.WorkflowRuns) < pageSize {
			return runs, nil
		}
		if page == pages {
			ds.Notes = append(ds.Notes, fmt.Sprintf("run list stopped at %d pages; older runs in the window are not counted", pages))
		}
	}
	return runs, nil
}

func matchesWorkflow(r Run, want string) bool {
	if want == "" {
		return true
	}
	file := path.Base(r.Path)
	return strings.EqualFold(r.Name, want) || file == want || strings.TrimSuffix(strings.TrimSuffix(file, ".yml"), ".yaml") == want
}

// Sample keeps at most limit runs, evenly spaced in creation order, and
// returns the factor that scales sampled totals back to all runs.
func Sample(runs []Run, limit int) ([]Run, float64) {
	sorted := append([]Run(nil), runs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	if limit <= 0 || len(sorted) <= limit {
		return sorted, 1
	}
	out := make([]Run, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, sorted[i*len(sorted)/limit])
	}
	return out, float64(len(sorted)) / float64(limit)
}

// fetchJobs reads the jobs of every attempt (filter=all) of the sampled
// runs, at most maxConcurrency at a time.
func fetchJobs(ctx context.Context, api API, opts CollectOptions, ds *Dataset) error {
	type result struct {
		id   int64
		jobs []Job
		err  error
	}
	workers := min(max(opts.Concurrency, 1), maxConcurrency)
	if opts.Concurrency <= 0 {
		workers = maxConcurrency
	}
	ids := make(chan int64)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range ids {
				jobs, err := runJobs(ctx, api, opts.Repo, id)
				results <- result{id: id, jobs: jobs, err: err}
			}
		}()
	}
	go func() {
		for _, r := range ds.Sampled {
			ids <- r.ID
		}
		close(ids)
		wg.Wait()
		close(results)
	}()
	var first error
	for r := range results {
		if r.err != nil && (first == nil || errors.Is(first, ErrBudget)) {
			first = r.err
		}
		if len(r.jobs) > 0 {
			ds.Jobs[r.id] = r.jobs
		}
	}
	return first
}

func runJobs(ctx context.Context, api API, repo string, id int64) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= defaultJobPages; page++ {
		q := url.Values{"filter": {"all"}, "per_page": {fmt.Sprint(pageSize)}, "page": {fmt.Sprint(page)}}
		data, err := api.Get(ctx, fmt.Sprintf("repos/%s/actions/runs/%d/jobs?%s", repo, id, q.Encode()))
		if err != nil {
			return jobs, err
		}
		var body struct {
			Jobs []Job `json:"jobs"`
		}
		if err := json.Unmarshal(data, &body); err != nil {
			return jobs, fmt.Errorf("ci report: jobs of run %d: %w", id, err)
		}
		jobs = append(jobs, body.Jobs...)
		if len(body.Jobs) < pageSize {
			break
		}
	}
	return jobs, nil
}
