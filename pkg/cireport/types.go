// Package cireport measures a GitHub Actions repository from the jobs API
// timestamps and lints its workflow files: queue and run time per job,
// end-to-end time per event, runner busy time, discarded minutes, duplicate
// and superseded runs, and the workflow settings that waste or expose
// runners. It is read-only and deterministic for a given set of responses.
package cireport

import "time"

// Run is one workflow run from GET /repos/{o}/{r}/actions/runs.
type Run struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	WorkflowID   int64     `json:"workflow_id"`
	Path         string    `json:"path"`
	Event        string    `json:"event"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	HeadSHA      string    `json:"head_sha"`
	HeadBranch   string    `json:"head_branch"`
	RunAttempt   int       `json:"run_attempt"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RunStartedAt time.Time `json:"run_started_at"`
	PullRequests []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}

// PR is the first pull request number of the run, or 0.
func (r Run) PR() int {
	if len(r.PullRequests) == 0 {
		return 0
	}
	return r.PullRequests[0].Number
}

// Job is one job attempt from GET .../runs/{id}/jobs?filter=all.
type Job struct {
	ID           int64     `json:"id"`
	RunID        int64     `json:"run_id"`
	RunAttempt   int       `json:"run_attempt"`
	Name         string    `json:"name"`
	WorkflowName string    `json:"workflow_name"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at"`
	RunnerName   string    `json:"runner_name"`
	Labels       []string  `json:"labels"`
}

// RunSeconds is how long the job ran, 0 when it never started or finished.
func (j Job) RunSeconds() float64 {
	if j.StartedAt.IsZero() || j.CompletedAt.IsZero() || j.CompletedAt.Before(j.StartedAt) {
		return 0
	}
	return j.CompletedAt.Sub(j.StartedAt).Seconds()
}

// QueueSeconds is how long the job waited for a runner (started - created).
func (j Job) QueueSeconds() (float64, bool) {
	if j.StartedAt.IsZero() || j.CreatedAt.IsZero() || j.StartedAt.Before(j.CreatedAt) {
		return 0, false
	}
	return j.StartedAt.Sub(j.CreatedAt).Seconds(), true
}

// SelfHosted reports whether the job ran on a self-hosted runner.
func (j Job) SelfHosted() bool {
	for _, l := range j.Labels {
		if l == "self-hosted" {
			return true
		}
	}
	return false
}

// Dataset is everything Collect fetched for one report.
type Dataset struct {
	Repo  string
	Since time.Time
	Until time.Time
	// Runs are the completed runs listed in the window; Sampled the subset
	// whose jobs were fetched, and Scale = len(Runs)/len(Sampled).
	Runs    []Run
	Sampled []Run
	Scale   float64
	Jobs    map[int64][]Job
	// Trees maps a commit SHA to its tree SHA, for the commits looked up.
	Trees map[string]string
	// Public is the repo visibility, nil when it could not be read.
	Public    *bool
	Requests  int
	Truncated bool
	Notes     []string
}

// Days is the window length in days, at least one hour's worth.
func (d Dataset) Days() float64 {
	return max(d.Until.Sub(d.Since).Hours()/24, 1.0/24)
}
