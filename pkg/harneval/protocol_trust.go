package harneval

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"
)

// Signer refusal reasons (SPEC-HARNEVAL-003 REQ-HR-02, REQ-HR-09). Each stops
// the signer before anything is signed or written.
const (
	ReasonAttestationDigestMismatch = "attestation_digest_mismatch"
	ReasonProtocolMismatch          = "protocol_mismatch"
	ReasonRecordsProtocolMismatch   = "records_protocol_mismatch"
	ReasonOutcomeDerivationMismatch = "outcome_derivation_mismatch"
	ReasonPolicyOutOfRange          = "policy_out_of_range"
)

// SignedLaneAgentSeconds bounds max_agent_runs x trial_timeout_seconds: the
// six-hour job limit minus thirty minutes of preparation.
const SignedLaneAgentSeconds = 19800

// TrustError is a signer refusal. Detail names the first field path, record,
// or document that failed the check.
type TrustError struct {
	Reason string
	Detail string
}

func (e *TrustError) Error() string { return e.Reason + ": " + e.Detail }

func trustErr(reason, format string, args ...any) *TrustError {
	return &TrustError{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// RunMeta is the --run-meta document: the run attempt the signer serves, as
// the Actions API reports it. Only the run id and attempt are compared with
// the protocol; the protocol start is bounded by attested log times instead.
type RunMeta struct {
	RunID            int64  `json:"run_id"`
	RunAttempt       int    `json:"run_attempt"`
	RunCreatedAt     string `json:"run_created_at"`
	AttemptStartedAt string `json:"attempt_started_at"`
}

// DecodeRunMeta strictly decodes and validates a run meta document.
func DecodeRunMeta(data []byte) (RunMeta, error) {
	var meta RunMeta
	if err := strictDecode(data, &meta); err != nil {
		return meta, err
	}
	if meta.RunID < 1 || meta.RunAttempt < 1 {
		return meta, invalidf(DetailFieldInvalid, "run meta needs a positive run_id and run_attempt")
	}
	for _, at := range []struct{ name, value string }{
		{"run_created_at", meta.RunCreatedAt}, {"attempt_started_at", meta.AttemptStartedAt},
	} {
		if _, err := time.Parse(time.RFC3339, at.value); err != nil {
			return meta, invalidf(DetailFieldInvalid, "run meta %s %q is not an RFC 3339 time", at.name, at.value)
		}
	}
	return meta, nil
}

// CorpusDigest is one row of a protocol's corpus_digests.
type CorpusDigest struct {
	File       string `json:"file"`
	FileSHA256 string `json:"file_sha256"`
}

// TrustedInputs are the trusted values the signer computes from the main
// checkout beside the golden set: the commit the baseline tag points to, the
// runner tree digest, both arm surface digests, the binding digest, and the
// black-box assertion ids of each scheduled task in main's task definitions.
// RebuildTrustedProtocol carries BindingDigest as given, so a caller may
// compute it from the rebuilt protocol; VerifySignedSession requires it.
type TrustedInputs struct {
	BaselineCommit         string
	RunnerTreeDigest       string
	BaselineSurfaceDigest  string
	CandidateSurfaceDigest string
	BindingDigest          string
	OracleAssertions       map[string][]string
}

// TrustedProtocol is the protocol the signer rebuilds from the main checkout.
// A received protocol must carry every one of these values exactly.
type TrustedProtocol struct {
	Policy            LivePolicy
	Pins              Pins
	Model             string
	CLIVersion        string
	WorkspaceRevision string
	BaselineRef       string
	AgentSetDigest    string
	CorpusDigests     []CorpusDigest
	Order             []Attempt
	TrustedInputs
}

// RebuildTrustedProtocol rebuilds the trusted protocol from the main
// checkout's golden set and the inputs computed beside it. Like the trusted
// runner, it schedules every active agent task in the balanced order, takes
// one corpus row per pinned corpus file, and expects the codex CLI pin as
// cli_version, since the runner refuses any other codex. The policy must fit
// one job (policy_out_of_range).
func RebuildTrustedProtocol(set *Set, inputs TrustedInputs) (TrustedProtocol, error) {
	tasks := []Task{}
	for _, task := range set.Tasks {
		if task.Kind == KindAgent && task.Status.State == StateActive {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return TrustedProtocol{}, invalidf(DetailFieldInvalid, "golden set has no active agent task to schedule")
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	live := set.Manifest.Live
	trusted := TrustedProtocol{
		Policy: live, Pins: set.Manifest.Pins, Model: live.Model, CLIVersion: set.Manifest.Pins.CodexCLIVersion,
		WorkspaceRevision: live.WorkspaceRevision, BaselineRef: live.BaselineRef,
		AgentSetDigest: AgentSetDigest(set), CorpusDigests: CorpusDigestRows(tasks),
		Order: BalancedOrder(ids, live.K), TrustedInputs: inputs,
	}
	return trusted, trusted.validate()
}

// BalancedOrder is the trusted runner's order: for each trial, the tasks by
// id, the baseline arm first when the trial plus the task index is even.
func BalancedOrder(taskIDs []string, k int) []Attempt {
	ids := slices.Clone(taskIDs)
	sort.Strings(ids)
	order := make([]Attempt, 0, 2*len(ids)*k)
	for trial := 0; trial < k; trial++ {
		for index, id := range ids {
			first, second := ArmBaseline, ArmCandidate
			if (trial+index)%2 == 1 {
				first, second = second, first
			}
			order = append(order, Attempt{TaskID: id, Arm: first, Trial: trial}, Attempt{TaskID: id, Arm: second, Trial: trial})
		}
	}
	return order
}

// CorpusDigestRows is one {file, file_sha256} row per corpus file the agent
// tasks pin, ordered by file.
func CorpusDigestRows(tasks []Task) []CorpusDigest {
	pinned := map[string]string{}
	for _, task := range tasks {
		if task.CorpusRef != nil {
			pinned[task.CorpusRef.File] = task.CorpusRef.FileSHA256
		}
	}
	rows := make([]CorpusDigest, 0, len(pinned))
	for file, sum := range pinned {
		rows = append(rows, CorpusDigest{File: file, FileSHA256: sum})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].File < rows[j].File })
	return rows
}

// validate checks the trusted protocol's own limits and inputs. Every trial
// may run to its timeout, so max_agent_runs of them must fit
// SignedLaneAgentSeconds and the order may not schedule more. The binding
// digest is checked where it is used.
func (t TrustedProtocol) validate() error {
	if err := validateLivePolicy(t.Policy); err != nil {
		return err
	}
	runs, timeout := t.Policy.MaxAgentRuns, t.Policy.TrialTimeoutSeconds
	if timeout > SignedLaneAgentSeconds || runs > SignedLaneAgentSeconds/timeout {
		return trustErr(ReasonPolicyOutOfRange, "max_agent_runs %d x trial_timeout_seconds %d exceeds %d seconds",
			runs, timeout, SignedLaneAgentSeconds)
	}
	if len(t.Order) > runs {
		return trustErr(ReasonPolicyOutOfRange, "order schedules %d trials, more than max_agent_runs %d", len(t.Order), runs)
	}
	if !revisionPattern.MatchString(t.BaselineCommit) {
		return invalidf(DetailFieldInvalid, "trusted baseline_commit %q is not a 40-hex commit", t.BaselineCommit)
	}
	for _, digest := range []struct{ name, value string }{
		{"runner_tree_digest", t.RunnerTreeDigest}, {"baseline_surface_digest", t.BaselineSurfaceDigest},
		{"candidate_surface_digest", t.CandidateSurfaceDigest}, {"agent_set_digest", t.AgentSetDigest},
	} {
		if !sha256Pattern.MatchString(digest.value) {
			return invalidf(DetailFieldInvalid, "trusted %s %q is not a 64-hex digest", digest.name, digest.value)
		}
	}
	return t.validateAssertions()
}

// validateAssertions requires a non-empty set of distinct, non-blank
// black-box assertion ids for exactly the scheduled tasks.
func (t TrustedProtocol) validateAssertions() error {
	scheduled := scheduledTasks(t.Order)
	if len(scheduled) == 0 {
		return invalidf(DetailFieldInvalid, "trusted order schedules no trial")
	}
	for _, task := range slices.Sorted(maps.Keys(t.OracleAssertions)) {
		if _, found := slices.BinarySearch(scheduled, task); !found {
			return invalidf(DetailFieldInvalid, "trusted black-box assertions name unscheduled task %s", task)
		}
	}
	for _, task := range scheduled {
		ids, covered := t.OracleAssertions[task]
		if !covered || len(ids) == 0 {
			return invalidf(DetailFieldInvalid, "task %s has no trusted black-box assertion id", task)
		}
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if blank(id) || seen[id] {
				return invalidf(DetailFieldInvalid, "task %s black-box assertion id %q is blank or repeated", task, id)
			}
			seen[id] = true
		}
	}
	return nil
}
