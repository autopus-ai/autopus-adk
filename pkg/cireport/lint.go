package cireport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// LintFinding is one workflow setting that wastes or exposes runners.
type LintFinding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Job      string `json:"job,omitempty"`
	Message  string `json:"message"`
}

func (l LintFinding) location() string {
	if l.Job == "" {
		return l.File
	}
	return l.File + ": job " + l.Job
}

var (
	hostedLabel  = regexp.MustCompile(`^(ubuntu|windows|macos)-[A-Za-z0-9._-]+$`)
	releaseJob   = regexp.MustCompile(`(?i)release|publish|deploy|sign|notar`)
	genericLabel = map[string]bool{"self-hosted": true, "linux": true, "macos": true, "windows": true,
		"x64": true, "arm64": true, "arm": true}
	// A concurrency group naming one of these keeps runs of different
	// events or different runs apart.
	groupSeparators = []string{"run_id", "event_name", "pull_request.number", "head_ref", "github.sha", "event.number", "run_number"}
	forkGuards      = []string{"head.repo.full_name", "head.repo.fork", "github.repository_owner", "github.event.pull_request.head.repo"}
	prHeadRefs      = []string{"pull_request.head", "head_ref", "refs/pull/", "github.event.number"}
	nonPREvents     = []string{"push", "release", "workflow_call", "schedule", "merge_group", "workflow_dispatch"}
)

// LintWorkflows checks the workflow files. public is the repository
// visibility, nil when unknown: the fork exposure rule needs it to be
// public. Files that do not parse are returned as problems, not findings.
func LintWorkflows(files map[string][]byte, public *bool) ([]LintFinding, []string) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []LintFinding
	var problems []string
	parsed := map[string]*workflow{}
	for _, name := range names {
		wf, err := parseWorkflow(files[name])
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		parsed[name] = wf
		out = append(out, lintWorkflow(name, wf, public != nil && *public)...)
	}
	out = append(out, sharedReleaseLabels(names, parsed)...)
	return out, problems
}

func lintWorkflow(file string, wf *workflow, public bool) []LintFinding {
	var out []LintFinding
	add := func(rule, sev, job, msg string) {
		out = append(out, LintFinding{Rule: rule, Severity: sev, File: file, Job: job, Message: msg})
	}
	c := wf.concurrency
	if wf.has("pull_request") && (!c.set || c.cancel == "" || c.cancel == "false") {
		add("pr-no-concurrency", SeverityMedium, "", "pull_request runs have no concurrency group with cancel-in-progress; a new push leaves the old run holding its runners")
	}
	if c.cancel == "true" && !containsAny(c.group, groupSeparators) {
		var hit []string
		for _, e := range nonPREvents {
			if wf.has(e) {
				hit = append(hit, e)
			}
		}
		if len(hit) > 0 {
			sev := SeverityMedium
			if wf.has("release", "workflow_call") {
				sev = SeverityHigh
			}
			add("concurrency-cancels-other-events", sev, "", fmt.Sprintf("group %q with cancel-in-progress: true also cancels %s runs; "+
				"key the group on the PR number and cancel only for pull_request (a reusable workflow shares github.workflow with its caller)",
				c.group, strings.Join(hit, ", ")))
		}
	}
	for _, j := range wf.jobs {
		if j.uses != "" {
			continue
		}
		if j.hasSteps && !j.timeout {
			add("missing-timeout", SeverityMedium, j.id, "job has steps but no timeout-minutes; a stuck job holds its runner for the 360-minute default")
		}
		if public && wf.has("pull_request", "pull_request_target") && j.selfHosted() && !containsAny(j.ifCond, forkGuards) {
			add("self-hosted-fork-pr", SeverityHigh, j.id, fmt.Sprintf("public repo PR job runs on self-hosted runner %v, reachable from fork PRs; use a GitHub-hosted runner or guard the job against forks", j.labels))
		}
		if wf.has("pull_request_target") {
			for _, ref := range j.checkoutRefs {
				if containsAny(ref, prHeadRefs) {
					add("prt-checkout-head", SeverityHigh, j.id, fmt.Sprintf("pull_request_target checks out the PR head (%s) with the base repo's secrets and write token", ref))
				}
			}
		}
	}
	return out
}

// selfHosted is true for a self-hosted label, a runner group, or any label
// that is not a GitHub-hosted image name; a label set by an expression is
// unknown and counts as hosted.
func (j wfJob) selfHosted() bool {
	if j.runsOnExpr || len(j.labels) == 0 {
		return false
	}
	for _, l := range j.labels {
		if l == "self-hosted" || strings.HasPrefix(l, "group:") || !hostedLabel.MatchString(l) {
			return true
		}
	}
	return false
}

func (j wfJob) distinctiveLabels() []string {
	var out []string
	for _, l := range j.labels {
		if !genericLabel[strings.ToLower(l)] {
			out = append(out, l)
		}
	}
	return out
}

// sharedReleaseLabels flags a self-hosted release or publish job whose
// distinctive runs-on label also routes an ordinary PR job: the release
// then queues behind PR lanes, and shares their host and caches.
func sharedReleaseLabels(names []string, parsed map[string]*workflow) []LintFinding {
	prLabels := map[string]string{}
	for _, name := range names {
		wf := parsed[name]
		if wf == nil || !wf.has("pull_request") {
			continue
		}
		for _, j := range wf.jobs {
			if j.selfHosted() && !releaseJob.MatchString(j.id+" "+j.name) {
				for _, l := range j.distinctiveLabels() {
					if _, ok := prLabels[l]; !ok {
						prLabels[l] = name + ": " + j.label()
					}
				}
			}
		}
	}
	var out []LintFinding
	for _, name := range names {
		wf := parsed[name]
		if wf == nil {
			continue
		}
		for _, j := range wf.jobs {
			if !j.selfHosted() || !releaseJob.MatchString(j.id+" "+j.name) {
				continue
			}
			for _, l := range j.distinctiveLabels() {
				if lane, ok := prLabels[l]; ok {
					out = append(out, LintFinding{Rule: "shared-release-label", Severity: SeverityMedium, File: name, Job: j.id,
						Message: fmt.Sprintf("runs-on label %q also routes PR lane %s; give release jobs a label of their own", l, lane)})
					break
				}
			}
		}
	}
	return out
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
