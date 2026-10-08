package harneval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedLaneFixture is a session the trusted runner wrote with --signed-lane
// on its fixture world: four black-box tasks and one white-box task at K=2,
// with the fake codex. scripts/benchmarks/harness/test_golden_wire_fixture.py
// regenerates it. The golden set is committed as LoadSet reads it before T15
// gives the task schema oracle_mode and black_box_oracle; black_box.json
// keeps the assertion ids of the black-box task definitions.
const signedLaneFixture = "testdata/signed-lane"

// fixtureLane is the lane file the bind job's auto computed for the session.
type fixtureLane struct {
	RunID            int64  `json:"run_id"`
	RunAttempt       int    `json:"run_attempt"`
	BindingDigest    string `json:"binding_digest"`
	BaselineCommit   string `json:"baseline_commit"`
	RunnerTreeDigest string `json:"runner_tree_digest"`
}

func readFixtureJSON(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(signedLaneFixture, name))
	require.NoError(t, err)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(target), name)
}

// fixtureTrust is what the signer computes beside main's golden set: the
// lane values, both arm surface digests computed here by Go over the arm
// trees the runner digested in Python, and the black-box assertion ids.
func fixtureTrust(t *testing.T) (TrustedProtocol, RunMeta) {
	t.Helper()
	var lane fixtureLane
	readFixtureJSON(t, "lane.json", &lane)
	assertions := map[string][]string{}
	readFixtureJSON(t, "black_box.json", &assertions)
	set, err := LoadSet(filepath.Join(signedLaneFixture, "set"))
	require.NoError(t, err)
	inputs := TrustedInputs{BaselineCommit: lane.BaselineCommit, RunnerTreeDigest: lane.RunnerTreeDigest,
		BindingDigest: lane.BindingDigest, OracleAssertions: assertions}
	for arm, target := range map[string]*string{ArmBaseline: &inputs.BaselineSurfaceDigest, ArmCandidate: &inputs.CandidateSurfaceDigest} {
		*target, err = SurfaceDigest(filepath.Join(signedLaneFixture, "surfaces", arm))
		require.NoError(t, err)
	}
	trusted, err := RebuildTrustedProtocol(set, inputs)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(signedLaneFixture, "run-meta.json"))
	require.NoError(t, err)
	meta, err := DecodeRunMeta(data)
	require.NoError(t, err)
	return trusted, meta
}

// fixtureAttested is what the live-eval job attests over the received
// bytes: bound ten minutes before started_at, the session result two hours
// after it.
func fixtureAttested(t *testing.T, in SignerInput, trusted TrustedProtocol, meta RunMeta) AttestedSession {
	t.Helper()
	protocol, err := DecodeProtocol(in.Protocol)
	require.NoError(t, err)
	started, err := time.Parse(time.RFC3339, protocol.StartedAt)
	require.NoError(t, err)
	bound := BoundPredicate{RunID: meta.RunID, RunAttempt: meta.RunAttempt, BindingDigest: trusted.BindingDigest, Event: EventBound}
	result := SessionResultPredicate{BoundPredicate: bound, ProtocolSHA256: sha256Hex(in.Protocol),
		RecordsSHA256: sha256Hex(in.Records), CalibrationSHA256: sha256Hex(in.Calibration)}
	result.Event = EventSessionResult
	for _, data := range in.OracleResults {
		result.OracleResultSHA256 = append(result.OracleResultSHA256, sha256Hex(data))
	}
	return AttestedSession{Bound: bound, BoundLogTime: started.Add(-10 * time.Minute), Result: result,
		ResultLogTime: started.Add(2 * time.Hour)}
}

// TestSignedLaneFixture_RunnerSessionVerifiesAndJudges: the bytes the Python
// runner writes pass every signer check and re-derive trial by trial, so the
// wire contract holds across the two implementations. The white-box task
// stays out of the order, and the 001 verdict judges the attested records:
// the candidate never passes the task the baseline always passes.
func TestSignedLaneFixture_RunnerSessionVerifiesAndJudges(t *testing.T) {
	t.Parallel()
	trusted, meta := fixtureTrust(t)
	in, err := LoadSignerInput(filepath.Join(signedLaneFixture, "session"))
	require.NoError(t, err)
	require.Len(t, in.OracleResults, 4, "identical oracle results share one content-addressed file")

	session, err := VerifySignedSession(in, trusted, meta, fixtureAttested(t, in, trusted, meta))

	require.NoError(t, err)
	assert.Equal(t, BalancedOrder([]string{"GT-AGENT-X01", "GT-AGENT-X02", "GT-AGENT-X03", "GT-AGENT-X04"}, 2), session.Protocol.Order,
		"GT-AGENT-X05 is white-box and stays in the advisory lane")
	type judged struct {
		outcome, signal  string
		ran, buildFailed bool
	}
	want := map[string][2]judged{
		"GT-AGENT-X01": {{OutcomePass, "accepted", true, false}, {OutcomeFail, SignalExpectationMismatch, true, false}},
		"GT-AGENT-X02": {{OutcomeFail, "agent_exit_nonzero", true, false}, {OutcomeFail, "agent_exit_nonzero", true, false}},
		"GT-AGENT-X03": {{OutcomeFail, SignalArtifactTimeout, false, false}, {OutcomeFail, SignalArtifactTimeout, false, false}},
		"GT-AGENT-X04": {{OutcomeFail, SignalArtifactBuildFailed, false, true}, {OutcomeFail, SignalArtifactBuildFailed, false, true}},
	}
	require.Len(t, session.Records, 16)
	for _, record := range session.Records {
		arm := 0
		if record.Arm == ArmCandidate {
			arm = 1
		}
		got := judged{record.Outcome, record.Signal, record.Oracle.Ran, record.Oracle.BuildFailed}
		assert.Equal(t, want[record.TaskID][arm], got, "%s/%s/%d", record.TaskID, record.Arm, record.Trial)
	}
	verdict, err := ComputeVerdict(session)
	require.NoError(t, err)
	assert.Equal(t, [2]string{VerdictRegression, ReasonHardFlip}, [2]string{verdict.Verdict, verdict.Reason})
	assert.Equal(t, []string{"GT-AGENT-X01"}, verdict.HardFlips)
	assert.Equal(t, [4]int{2, 8, 0, 8}, [4]int{verdict.Arms.Baseline.Passes, verdict.Arms.Baseline.Valid,
		verdict.Arms.Candidate.Passes, verdict.Arms.Candidate.Valid})
	assert.InDelta(t, -0.25, verdict.RegressionDelta, 1e-9)
	assert.Equal(t, CalibrationPassed, verdict.Calibration.Status)
}

// TestSignedLaneFixture_RecordTheTableDoesNotGive_IsRefused: the check is not
// vacuous on the runner's bytes. A record re-attested with a signal its
// oracle result does not support is outcome_derivation_mismatch.
func TestSignedLaneFixture_RecordTheTableDoesNotGive_IsRefused(t *testing.T) {
	t.Parallel()
	trusted, meta := fixtureTrust(t)
	in, err := LoadSignerInput(filepath.Join(signedLaneFixture, "session"))
	require.NoError(t, err)
	mismatch := `"outcome":"fail","signal":"expectation_mismatch","oracle":{"ran":true,"build_failed":false,"expected_passed":1,"expected_failed":2}`
	require.Equal(t, 2, bytes.Count(in.Records, []byte(mismatch)), "the candidate trials of GT-AGENT-X01")
	in.Records = bytes.Replace(in.Records, []byte(mismatch),
		[]byte(`"outcome":"pass","signal":"accepted","oracle":{"ran":true,"build_failed":false,"expected_passed":1,"expected_failed":2}`), 1)

	_, err = VerifySignedSession(in, trusted, meta, fixtureAttested(t, in, trusted, meta))

	var refusal *TrustError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, ReasonOutcomeDerivationMismatch, refusal.Reason)
	assert.Contains(t, refusal.Detail, "records pass/accepted, the table gives fail/expectation_mismatch")
}
