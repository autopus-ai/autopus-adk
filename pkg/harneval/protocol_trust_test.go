package harneval

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedAssertions are the black-box assertion ids of every fixture task.
var signedAssertions = []string{"exit_status", "stdout", "output_file"}

// trustedInputs are well-formed trusted inputs for the named scheduled tasks.
func trustedInputs(tasks ...string) TrustedInputs {
	assertions := map[string][]string{}
	for _, id := range tasks {
		assertions[id] = signedAssertions
	}
	return TrustedInputs{
		BaselineCommit: strings.Repeat("c0", 20), RunnerTreeDigest: strings.Repeat("7e", 32),
		BaselineSurfaceDigest: strings.Repeat("ba", 32), CandidateSurfaceDigest: strings.Repeat("ca", 32),
		BindingDigest: strings.Repeat("b1", 32), OracleAssertions: assertions,
	}
}

// requireTrustError asserts err is a *TrustError with the reason and detail.
func requireTrustError(t *testing.T, err error, reason, detail string) {
	t.Helper()
	var trust *TrustError
	require.True(t, errors.As(err, &trust), "error %v is not a TrustError", err)
	assert.Equal(t, reason, trust.Reason, err.Error())
	assert.Equal(t, detail, trust.Detail, err.Error())
	assert.Equal(t, reason+": "+detail, err.Error())
}

// loadedSet loads the standard fixture set after mutate edits it.
func loadedSet(t *testing.T, mutate func(f *fixture)) *Set {
	t.Helper()
	f := newFixture(t)
	f.standard()
	if mutate != nil {
		mutate(f)
	}
	set, err := LoadSet(f.root)
	require.NoError(t, err)
	return set
}

func TestDecodeRunMeta_GitHubRunAttempt(t *testing.T) {
	t.Parallel()
	valid := `{"run_id":18234567890,"run_attempt":2,"run_created_at":"2026-10-07T00:50:00Z","attempt_started_at":"2026-10-07T00:51:30Z"}`

	meta, err := DecodeRunMeta([]byte(valid))

	require.NoError(t, err)
	assert.Equal(t, RunMeta{RunID: 18234567890, RunAttempt: 2, RunCreatedAt: "2026-10-07T00:50:00Z", AttemptStartedAt: "2026-10-07T00:51:30Z"}, meta)
	tests := []struct{ name, detail, body string }{
		{"unknown field", DetailUnknownField, `{"run_id":1,"run_attempt":1,"run_created_at":"2026-10-07T00:50:00Z","attempt_started_at":"2026-10-07T00:51:30Z","conclusion":"success"}`},
		{"trailing data", DetailTrailingData, valid + "{}"},
		{"run id a string", DetailFieldInvalid, `{"run_id":"18234567890","run_attempt":2,"run_created_at":"2026-10-07T00:50:00Z","attempt_started_at":"2026-10-07T00:51:30Z"}`},
		{"run id zero", DetailFieldInvalid, `{"run_id":0,"run_attempt":2,"run_created_at":"2026-10-07T00:50:00Z","attempt_started_at":"2026-10-07T00:51:30Z"}`},
		{"attempt zero", DetailFieldInvalid, `{"run_id":5,"run_attempt":0,"run_created_at":"2026-10-07T00:50:00Z","attempt_started_at":"2026-10-07T00:51:30Z"}`},
		{"created without zone", DetailFieldInvalid, `{"run_id":5,"run_attempt":1,"run_created_at":"2026-10-07T00:50:00","attempt_started_at":"2026-10-07T00:51:30Z"}`},
		{"attempt start missing", DetailFieldInvalid, `{"run_id":5,"run_attempt":1,"run_created_at":"2026-10-07T00:50:00Z"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeRunMeta([]byte(tt.body))

			requireInvalid(t, err, tt.detail)
		})
	}
}

// TestRebuildTrustedProtocol_GoldenSet_FixesEveryTrustedField: the trusted
// protocol takes the policy, pins, model, CLI pin, revision, and baseline ref
// from the manifest, the agent set digest and the corpus rows from the set,
// and schedules the active agent tasks in the balanced order golden.py uses.
func TestRebuildTrustedProtocol_GoldenSet_FixesEveryTrustedField(t *testing.T) {
	t.Parallel()
	const corpusB = `[{"id":"b01","prompt":"fix that"}]` + "\n"
	set := loadedSet(t, func(f *fixture) {
		f.write("bench/corpus_b.json", corpusB)
		second := agentTask("GT-AG-002")
		second["corpus_ref"] = map[string]any{"file": "bench/corpus_b.json", "task_id": "b01", "file_sha256": sha256Of(corpusB)}
		f.writeJSON(agentPath("GT-AG-002"), second)
		f.writeJSON(agentPath("GT-AG-000"), retiredTask(agentTask("GT-AG-000")))
	})
	inputs := trustedInputs("GT-AG-001", "GT-AG-002")

	trusted, err := RebuildTrustedProtocol(set, inputs)

	require.NoError(t, err)
	assert.Equal(t, LivePolicy{
		K: 2, ThresholdBP: -1000, CompletenessFloor: 0.9, MaxAgentRuns: 96, TrialTimeoutSeconds: 180,
		WorkspaceRevision: strings.Repeat("a", 40), BaselineRef: "v0.50.122", Model: "gpt-test",
	}, trusted.Policy)
	assert.Equal(t, Pins{GeneratorVersion: "v0.50.123", ProjectName: "harneval-fixture",
		CodexCLIVersion: "codex-cli 0.160.0", OpencodeCLIVersion: "1.18.7"}, trusted.Pins)
	assert.Equal(t, "gpt-test", trusted.Model)
	assert.Equal(t, "codex-cli 0.160.0", trusted.CLIVersion, "golden.py refuses a codex whose version is not the pin")
	assert.Equal(t, strings.Repeat("a", 40), trusted.WorkspaceRevision)
	assert.Equal(t, "v0.50.122", trusted.BaselineRef)
	assert.Equal(t, AgentSetDigest(set), trusted.AgentSetDigest)
	assert.Equal(t, []CorpusDigest{
		{File: "bench/corpus_a.json", FileSHA256: sha256Of(fixtureCorpus)},
		{File: "bench/corpus_b.json", FileSHA256: sha256Of(corpusB)},
	}, trusted.CorpusDigests, "one row per corpus file of the active agent tasks, by file")
	b, c := ArmBaseline, ArmCandidate
	assert.Equal(t, []Attempt{
		{"GT-AG-001", b, 0}, {"GT-AG-001", c, 0}, {"GT-AG-002", c, 0}, {"GT-AG-002", b, 0},
		{"GT-AG-001", c, 1}, {"GT-AG-001", b, 1}, {"GT-AG-002", b, 1}, {"GT-AG-002", c, 1},
	}, trusted.Order, "the retired task and the surface tasks are not scheduled")
	assert.Equal(t, inputs, trusted.TrustedInputs)
}

// TestRebuildTrustedProtocol_PolicyBeyondOneJob_IsPolicyOutOfRange: every
// trial may take the whole trial timeout, so max_agent_runs of them must fit
// in 19800 seconds, and the order may not schedule more than max_agent_runs.
func TestRebuildTrustedProtocol_PolicyBeyondOneJob_IsPolicyOutOfRange(t *testing.T) {
	t.Parallel()
	policy := func(t *testing.T, runs, timeout int) *Set {
		return loadedSet(t, func(f *fixture) {
			manifest := validManifest()
			live := manifest["live"].(map[string]any)
			live["max_agent_runs"], live["trial_timeout_seconds"] = runs, timeout
			f.writeJSON(ManifestPath, manifest)
		})
	}
	tests := []struct {
		name          string
		runs, timeout int
		detail        string
	}{
		{"one second over the job", 99, 201, "max_agent_runs 99 x trial_timeout_seconds 201 exceeds 19800 seconds"},
		{"timeout alone beyond the job", 1, 19801, "max_agent_runs 1 x trial_timeout_seconds 19801 exceeds 19800 seconds"},
		{"more trials than max_agent_runs", 3, 400, "order schedules 4 trials, more than max_agent_runs 3"},
		{"exactly the job", 99, 200, ""},
		{"the initial policy", 48, 400, ""},
		{"every scheduled trial at the job's end", 4, 4950, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := RebuildTrustedProtocol(policy(t, tt.runs, tt.timeout), trustedInputs("GT-AG-001"))

			if tt.detail == "" {
				assert.NoError(t, err)
				return
			}
			requireTrustError(t, err, ReasonPolicyOutOfRange, tt.detail)
		})
	}
}

// TestRebuildTrustedProtocol_MalformedInputs_AreInvalid: trusted inputs that
// are not well-formed digests, or black-box assertions that do not cover the
// scheduled tasks exactly, are a defect of the signer's own inputs.
func TestRebuildTrustedProtocol_MalformedInputs_AreInvalid(t *testing.T) {
	t.Parallel()
	set := loadedSet(t, nil)
	tests := []struct {
		name, fragment string
		change         func(in *TrustedInputs)
	}{
		{"baseline commit a sha256", "baseline_commit", func(in *TrustedInputs) { in.BaselineCommit = strings.Repeat("c0", 32) }},
		{"runner tree digest short", "runner_tree_digest", func(in *TrustedInputs) { in.RunnerTreeDigest = "7e" }},
		{"baseline surface digest blank", "baseline_surface_digest", func(in *TrustedInputs) { in.BaselineSurfaceDigest = "" }},
		{"candidate surface digest upper case", "candidate_surface_digest", func(in *TrustedInputs) { in.CandidateSurfaceDigest = strings.Repeat("CA", 32) }},
		{"assertions name an unscheduled task", "GT-AG-009", func(in *TrustedInputs) { in.OracleAssertions["GT-AG-009"] = signedAssertions }},
		{"assertions empty", "GT-AG-001", func(in *TrustedInputs) { in.OracleAssertions["GT-AG-001"] = []string{} }},
		{"assertion id repeated", "stdout", func(in *TrustedInputs) { in.OracleAssertions["GT-AG-001"] = []string{"stdout", "stdout"} }},
		{"assertion id blank", `" "`, func(in *TrustedInputs) { in.OracleAssertions["GT-AG-001"] = []string{"stdout", " "} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inputs := trustedInputs("GT-AG-001")
			tt.change(&inputs)

			_, err := RebuildTrustedProtocol(set, inputs)

			requireInvalid(t, err, DetailFieldInvalid)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
	noAgent := loadedSet(t, func(f *fixture) { f.writeJSON(agentPath("GT-AG-001"), retiredTask(agentTask("GT-AG-001"))) })
	for name, check := range map[string]struct {
		set    *Set
		inputs TrustedInputs
	}{"no active agent task": {noAgent, trustedInputs()}, "no black-box task": {set, TrustedInputs{}}} {
		_, err := RebuildTrustedProtocol(check.set, check.inputs)
		requireInvalid(t, err, DetailFieldInvalid)
		assert.Contains(t, err.Error(), "no active black-box agent task", name)
	}
}

// TestRebuildTrustedProtocol_WhiteBoxTask_StaysOutOfTheSignedLane: an active
// agent task without a black-box oracle in main's task definition is neither
// scheduled nor pinned by a corpus row, as the runner's signed lane leaves it
// to the SPEC-HARNEVAL-001 advisory lane (REQ-HR-08, S8).
func TestRebuildTrustedProtocol_WhiteBoxTask_StaysOutOfTheSignedLane(t *testing.T) {
	t.Parallel()
	const corpusB = `[{"id":"b01","prompt":"fix that"}]` + "\n"
	set := loadedSet(t, func(f *fixture) {
		f.write("bench/corpus_b.json", corpusB)
		whiteBox := agentTask("GT-AG-002")
		whiteBox["corpus_ref"] = map[string]any{"file": "bench/corpus_b.json", "task_id": "b01", "file_sha256": sha256Of(corpusB)}
		f.writeJSON(agentPath("GT-AG-002"), whiteBox)
	})

	trusted, err := RebuildTrustedProtocol(set, trustedInputs("GT-AG-001"))

	require.NoError(t, err)
	assert.Equal(t, BalancedOrder([]string{"GT-AG-001"}, 2), trusted.Order)
	assert.Equal(t, []CorpusDigest{{File: "bench/corpus_a.json", FileSHA256: sha256Of(fixtureCorpus)}}, trusted.CorpusDigests)
	assert.Equal(t, AgentSetDigest(set), trusted.AgentSetDigest, "the agent set digest still covers every agent task")
}
