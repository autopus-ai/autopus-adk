package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandGHRun is one row of the run list JSON, spelled as gh prints it. Every
// string is untrusted: rows are only compared, and a workflow name reaches a
// series ID only through the identifier filter.
type bandGHRun struct {
	DatabaseID   int64  `json:"databaseId"`
	Attempt      int    `json:"attempt"`
	Conclusion   string `json:"conclusion"`
	Status       string `json:"status"`
	HeadBranch   string `json:"headBranch"`
	Event        string `json:"event"`
	WorkflowName string `json:"workflowName"`
	CreatedAt    string `json:"createdAt"`
}

// bandCIFetch is the outcome of the CI network step. Reason is the REQ-05
// code of a skipped ingest, which carries no observations. DefaultBranch is
// the remote default branch the step resolved, "" when the lookup failed;
// the local patch base of SPEC-SIGMABAND-002 (Git Execution Policy item 4)
// is its remote-tracking ref.
type bandCIFetch struct {
	Reason        string
	Target        *bandGHTarget            // resolved repository; nil when resolution failed
	DefaultBranch string                   // resolved default branch; "" when unknown
	Rows          int                      // rows gh returned
	Excluded      int                      // rows outside the trusted filter
	Invalid       int                      // trusted rows rejected at ingest (invalid_value)
	Observations  []healthband.Observation // one per run id, oldest first
	SeriesReasons map[string][]string      // identifier_sanitized per filtered series
}

// bandCIConclusions maps the counted conclusions to values (1 = failure).
// Every other conclusion is excluded: cancelled, action_required, skipped,
// neutral, stale, and the empty conclusion of a run still in progress.
var bandCIConclusions = map[string]float64{"success": 0, "failure": 1, "timed_out": 1, "startup_failure": 1}

// resolveRepo resolves the repository band reads: origin names it, gh must
// be on PATH, and the host rule holds. github.com must pass gh auth status
// (else gh_unauthenticated); any other host counts as GitHub only when it is
// neither localhost nor an IP literal and passes gh auth status run without
// an injected GH_HOST, which gh answers only for a host in its hosts config
// (else remote_not_github). Only then do later calls get GH_HOST.
func (c bandGHClient) resolveRepo(ctx context.Context, projectDir string) (bandGHTarget, string) {
	originCommand := bandCommand{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: projectDir}
	origin, err := c.output(ctx, originCommand, bandTextOutputCap)
	if origin = strings.TrimSpace(origin); err != nil || origin == "" {
		return bandGHTarget{}, healthband.ReasonNoRemote
	}
	target, ok := parseBandOrigin(origin)
	if !ok {
		return bandGHTarget{}, healthband.ReasonRemoteNotGitHub
	}
	if _, err := c.runner.LookPath("gh"); err != nil {
		return bandGHTarget{}, healthband.ReasonGHMissing
	}
	err = c.capture(ctx, c.callTimeout, c.ghAuthStatus(target, projectDir), io.Discard)
	switch {
	case err == nil:
		return target, ""
	case errors.Is(err, exec.ErrNotFound):
		return bandGHTarget{}, healthband.ReasonGHMissing
	case target.Host == bandGitHubHost:
		return bandGHTarget{}, healthband.ReasonGHUnauthenticated
	default:
		return bandGHTarget{}, healthband.ReasonRemoteNotGitHub
	}
}

// fetchCI is the network step of CI ingest (Durability Protocol item 1). It
// runs outside the store lock and never fails band: an unavailable source
// returns its REQ-05 reason. Only a limit outside REQ-19 is an error.
func (c bandGHClient) fetchCI(ctx context.Context, projectDir string, limit int) (bandCIFetch, error) {
	if err := validateBandLimit(limit); err != nil {
		return bandCIFetch{}, err
	}
	target, reason := c.resolveRepo(ctx, projectDir)
	if reason != "" {
		return bandCIFetch{Reason: reason}, nil
	}
	fetch := bandCIFetch{Target: &target}
	branchCommand := c.gh(target, projectDir, "api", "repos/"+target.Slug(), "--hostname", target.Host, "--jq", ".default_branch")
	branch, err := c.output(ctx, branchCommand, bandTextOutputCap)
	if branch = strings.TrimSpace(branch); err != nil || branch == "" || branch == "null" || strings.ContainsAny(branch, "\r\n") {
		fetch.Reason = healthband.ReasonDefaultBranchUnknown
		return fetch, nil
	}
	fetch.DefaultBranch = branch
	// No status filter: successes are evidence too (REQ-04).
	listCommand := c.gh(target, projectDir, "run", "list", "-R", target.Slug(), "--limit", strconv.Itoa(limit), "--json", bandRunListFields)
	listing, err := c.output(ctx, listCommand, bandRunListCap)
	var runs []bandGHRun
	if err == nil {
		err = json.Unmarshal([]byte(listing), &runs)
	}
	if err != nil {
		fetch.Reason = healthband.ReasonGHFetchFailed
		return fetch, nil
	}
	fetch.addRuns(runs, branch)
	return fetch, nil
}

// addRuns keeps trusted evidence only (REQ-04): completed push or schedule
// runs on the default branch with a counted conclusion, one per run id with
// the highest attempt (equal attempts keep the first row), oldest first by
// (createdAt, numeric run id). gh pages a long list itself, so a row that
// shifts across a page boundary can repeat; it is kept once. A run whose
// newest attempt is excluded, for example cancelled, adds nothing, so an
// earlier stored attempt keeps its value: schema v1 has no tombstone.
func (f *bandCIFetch) addRuns(runs []bandGHRun, defaultBranch string) {
	f.Rows = len(runs)
	index := make(map[int64]int, len(runs))
	for _, run := range runs {
		value, counted := bandCIConclusions[run.Conclusion]
		if !counted || run.Status != "completed" || (run.Event != "push" && run.Event != "schedule") || run.HeadBranch != defaultBranch {
			f.Excluded++
			continue
		}
		observation, sanitized, ok := bandCIObservation(run, value)
		if !ok {
			f.Invalid++
			continue
		}
		if at, seen := index[run.DatabaseID]; seen {
			if observation.Attempt > f.Observations[at].Attempt {
				f.Observations[at] = observation
			}
			continue
		}
		index[run.DatabaseID] = len(f.Observations)
		f.Observations = append(f.Observations, observation)
		if sanitized && f.SeriesReasons[observation.Series] == nil {
			if f.SeriesReasons == nil {
				f.SeriesReasons = make(map[string][]string)
			}
			f.SeriesReasons[observation.Series] = []string{healthband.ReasonIdentifierSanitized}
		}
	}
	healthband.SortObservations(f.Observations)
}

// bandCIObservation maps one trusted run to an observation and reports
// whether its workflow name was filtered. A row that breaks the observation
// contract (no positive run id, attempt below 1, unparsable createdAt) is
// rejected here, at ingest, as invalid_value.
func bandCIObservation(run bandGHRun, value float64) (healthband.Observation, bool, bool) {
	created, err := time.Parse(time.RFC3339, run.CreatedAt)
	series, sanitized := healthband.CISeriesID(run.WorkflowName)
	observation := healthband.Observation{
		Schema: healthband.SchemaObservation, Series: series, SampleKey: strconv.FormatInt(run.DatabaseID, 10),
		ObservedAt: created.UTC(), Tiebreak: run.DatabaseID, Value: value, Attempt: run.Attempt, Source: healthband.SourceGH,
	}
	if err != nil || run.DatabaseID <= 0 || observation.Validate() != nil {
		return healthband.Observation{}, false, false
	}
	return observation, sanitized, true
}

// mergeBandCI is the phase A ingest step: under the store lock it appends
// the fetched observations whose run id is new or whose attempt is higher
// than every stored one, so re-ingesting a payload appends nothing. It
// returns the appended observations, which phase A plans as fresh.
func mergeBandCI(locked *healthband.Locked, fetch bandCIFetch) ([]healthband.Observation, error) {
	if fetch.Reason != "" || len(fetch.Observations) == 0 {
		return nil, nil
	}
	return locked.MergeObservations(healthband.CIRunsFile, fetch.Observations)
}

// failedRunEvidence fetches the failed-step log of one run attempt (the
// attempt of the evaluated observation), keeps its last 4 MiB, and returns
// it only after the Untrusted Input Contract. A failed or timed-out fetch
// returns no text, not even a partial one.
func (c bandGHClient) failedRunEvidence(ctx context.Context, target bandGHTarget, projectDir string, runID int64, attempt int) (healthband.Evidence, error) {
	if runID <= 0 || attempt < 1 {
		return healthband.Evidence{}, fmt.Errorf("react band: invalid run %d attempt %d", runID, attempt)
	}
	command := c.gh(target, projectDir, "run", "view", strconv.FormatInt(runID, 10), "-R", target.Slug(), "--attempt", strconv.Itoa(attempt), "--log-failed")
	tail := healthband.NewTailBuffer(c.logCapture)
	if err := c.capture(ctx, c.logTimeout, command, tail); err != nil {
		return healthband.Evidence{}, err
	}
	captured, dropped := tail.Captured()
	return healthband.SanitizeCILog(captured, dropped, projectDir), nil
}
