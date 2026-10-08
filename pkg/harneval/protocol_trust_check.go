package harneval

import (
	"encoding/json"
	"fmt"
	"time"
)

// protocolMismatch returns the path of the first trusted field, in the
// REQ-HR-02 order, that the protocol does not carry exactly, or "" when it
// carries all of them. The decoder has already made the policy's revision,
// baseline ref, and model equal to the protocol's own, so a change to one of
// those is reported at its policy path.
func protocolMismatch(p Protocol, t TrustedProtocol) string {
	fields := []struct {
		path  string
		equal bool
	}{
		{"policy.k", p.Policy.K == t.Policy.K},
		{"policy.threshold_bp", p.Policy.ThresholdBP == t.Policy.ThresholdBP},
		{"policy.completeness_floor", p.Policy.CompletenessFloor == t.Policy.CompletenessFloor},
		{"policy.max_agent_runs", p.Policy.MaxAgentRuns == t.Policy.MaxAgentRuns},
		{"policy.trial_timeout_seconds", p.Policy.TrialTimeoutSeconds == t.Policy.TrialTimeoutSeconds},
		{"policy.workspace_revision", p.Policy.WorkspaceRevision == t.Policy.WorkspaceRevision},
		{"policy.baseline_ref", p.Policy.BaselineRef == t.Policy.BaselineRef},
		{"policy.model", p.Policy.Model == t.Policy.Model},
		{"pins.generator_version", p.Pins.GeneratorVersion == t.Pins.GeneratorVersion},
		{"pins.project_name", p.Pins.ProjectName == t.Pins.ProjectName},
		{"pins.codex_model_catalog", p.Pins.CodexModelCatalog == t.Pins.CodexModelCatalog},
		{"pins.codex_cli_version", p.Pins.CodexCLIVersion == t.Pins.CodexCLIVersion},
		{"pins.opencode_cli_version", p.Pins.OpencodeCLIVersion == t.Pins.OpencodeCLIVersion},
		{"model", p.Model == t.Model},
		{"cli_version", p.CLIVersion == t.CLIVersion},
		{"workspace_revision", p.WorkspaceRevision == t.WorkspaceRevision},
		{"baseline_ref", p.BaselineRef == t.BaselineRef},
		{"baseline_commit", p.BaselineCommit == t.BaselineCommit},
		{"agent_set_digest", p.AgentSetDigest == t.AgentSetDigest},
	}
	for _, field := range fields {
		if !field.equal {
			return field.path
		}
	}
	if path := corpusMismatch(p.CorpusDigests, t.CorpusDigests); path != "" {
		return path
	}
	if p.RunnerTreeDigest != t.RunnerTreeDigest {
		return "runner_tree_digest"
	}
	if path := orderMismatch(p.Order, t.Order); path != "" {
		return path
	}
	for _, field := range []struct {
		path  string
		equal bool
	}{
		{"baseline_surface_digest", p.BaselineSurfaceDigest == t.BaselineSurfaceDigest},
		{"candidate_surface_digest", p.CandidateSurfaceDigest == t.CandidateSurfaceDigest},
		{"binding_digest", p.BindingDigest == t.BindingDigest},
	} {
		if !field.equal {
			return field.path
		}
	}
	return ""
}

// corpusMismatch compares the protocol's corpus rows, each strictly decoded,
// with the trusted rows in order.
func corpusMismatch(rows []json.RawMessage, want []CorpusDigest) string {
	if len(rows) != len(want) {
		return "corpus_digests"
	}
	for index, row := range rows {
		var got CorpusDigest
		switch {
		case strictDecode(row, &got) != nil:
			return fmt.Sprintf("corpus_digests[%d]", index)
		case got.File != want[index].File:
			return fmt.Sprintf("corpus_digests[%d].file", index)
		case got.FileSHA256 != want[index].FileSHA256:
			return fmt.Sprintf("corpus_digests[%d].file_sha256", index)
		}
	}
	return ""
}

// orderMismatch compares the protocol order with the trusted order attempt
// by attempt.
func orderMismatch(got, want []Attempt) string {
	if len(got) != len(want) {
		return "order"
	}
	for index := range got {
		if got[index] != want[index] {
			return fmt.Sprintf("order[%d]", index)
		}
	}
	return ""
}

// runMismatch ties the protocol to the attested run attempt: its run id and
// attempt are the run meta's, and it started no earlier than the attempt's
// bound log time and no later than its session_result log time. Both are
// verified timestamps the runner cannot set, so untrusted data cannot move
// the freshness anchor. started_at has whole seconds, so the bound log time
// is compared at that precision; no clock is read, so an approval delay
// before signing is never a mismatch.
func runMismatch(p Protocol, meta RunMeta, attested AttestedSession) string {
	started, err := time.Parse(time.RFC3339, p.StartedAt)
	switch {
	case p.RunID != meta.RunID:
		return "run_id"
	case p.RunAttempt != meta.RunAttempt:
		return "run_attempt"
	case err != nil || started.Before(attested.BoundLogTime.Truncate(time.Second)) || started.After(attested.ResultLogTime):
		return "started_at"
	}
	return ""
}

// validateSignedLane checks the five signed-lane protocol fields
// (SPEC-HARNEVAL-003 Wire Contracts): a maintainer-host session carries none
// of them and a signed-lane session carries all five, well formed. A zero run
// id or attempt counts as absent.
func validateSignedLane(p Protocol) error {
	present := 0
	for _, set := range []bool{p.RunID != 0, p.RunAttempt != 0, p.BindingDigest != "", p.BaselineCommit != "", p.RunnerTreeDigest != ""} {
		if set {
			present++
		}
	}
	switch {
	case present == 0:
		return nil
	case present < 5:
		return invalidf(DetailFieldInvalid, "signed-lane protocol needs all five of run_id, run_attempt, binding_digest, baseline_commit, and runner_tree_digest")
	case p.RunID < 1 || p.RunAttempt < 1:
		return invalidf(DetailFieldInvalid, "signed-lane protocol needs a positive run_id and run_attempt")
	case !sha256Pattern.MatchString(p.BindingDigest):
		return invalidf(DetailFieldInvalid, "binding_digest %q is not a 64-hex digest", p.BindingDigest)
	case !revisionPattern.MatchString(p.BaselineCommit):
		return invalidf(DetailFieldInvalid, "baseline_commit %q is not a 40-hex commit", p.BaselineCommit)
	case !sha256Pattern.MatchString(p.RunnerTreeDigest):
		return invalidf(DetailFieldInvalid, "runner_tree_digest %q is not a 64-hex digest", p.RunnerTreeDigest)
	}
	return nil
}
