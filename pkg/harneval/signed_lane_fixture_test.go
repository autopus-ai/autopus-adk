package harneval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedLaneFixture is a session the trusted runner wrote with --signed-lane
// on its fixture world: twelve black-box tasks and one white-box task at
// K=2, with the fake codex. scripts/benchmarks/harness/test_golden_wire_fixture.py
// regenerates it and lists the REQ-HR-08 rows it reaches and why the others
// are out of an honest runner's reach. The golden set is committed with the
// T15 task schema (oracle_mode, black_box_oracle, GT-AGENT-X07's positive
// control) and its oracle fixtures, so LoadSet decodes it and
// OracleAssertionIDs gives the trusted assertion ids.
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
// trees the runner digested in Python, and the black-box assertion ids of
// the set's own task definitions.
func fixtureTrust(t *testing.T) (TrustedProtocol, RunMeta) {
	t.Helper()
	var lane fixtureLane
	readFixtureJSON(t, "lane.json", &lane)
	set, err := LoadSet(filepath.Join(signedLaneFixture, "set"))
	require.NoError(t, err)
	inputs := TrustedInputs{BaselineCommit: lane.BaselineCommit, RunnerTreeDigest: lane.RunnerTreeDigest,
		BindingDigest: lane.BindingDigest, OracleAssertions: OracleAssertionIDs(set)}
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

	session, err := VerifySignedSession(in, trusted, meta, fixtureAttested(t, in, trusted, meta))

	require.NoError(t, err)
	var ids []string
	for index := 1; index <= 12; index++ {
		ids = append(ids, fmt.Sprintf("GT-AGENT-X%02d", index))
	}
	assert.Equal(t, BalancedOrder(ids, 2), session.Protocol.Order, "GT-AGENT-X13 is white-box and stays in the advisory lane")
	assert.Equal(t, []string{"exit", "stdout", PositiveControlExitID, PositiveControlStdoutID},
		trusted.OracleAssertions["GT-AGENT-X07"], "the positive control's ids come from main's definition")
	type judged struct {
		outcome, signal  string
		ran, buildFailed bool
	}
	both := func(j judged) [2]judged { return [2]judged{j, j} }
	want := map[string][2]judged{
		"GT-AGENT-X01": {{OutcomePass, "accepted", true, false}, {OutcomeFail, SignalExpectationMismatch, true, false}},
		"GT-AGENT-X02": both(judged{OutcomeFail, "agent_exit_nonzero", true, false}),
		"GT-AGENT-X03": both(judged{OutcomeFail, SignalArtifactTimeout, false, false}),
		"GT-AGENT-X04": both(judged{OutcomeFail, SignalOutputLinkRejected, false, false}),
		"GT-AGENT-X05": both(judged{OutcomeFail, SignalArtifactBuildFailed, false, true}),
		"GT-AGENT-X06": both(judged{OutcomeFail, "scope_violation", false, false}),
		"GT-AGENT-X07": both(judged{OutcomeFail, SignalExpectationMismatch, true, false}),
		"GT-AGENT-X08": both(judged{OutcomeFail, "agent_timeout", true, false}),
		"GT-AGENT-X09": both(judged{OutcomeFail, "agent_launch_failed", true, false}),
		"GT-AGENT-X10": both(judged{OutcomeFail, "observation_failed", false, false}),
		"GT-AGENT-X11": both(judged{OutcomeFail, SignalOutputTooLarge, false, false}),
		"GT-AGENT-X12": both(judged{OutcomeError, "warmup_failed", false, false}),
	}
	require.Len(t, session.Records, 48)
	for _, record := range session.Records {
		arm := 0
		if record.Arm == ArmCandidate {
			arm = 1
		}
		got := judged{record.Outcome, record.Signal, record.Oracle.Ran, record.Oracle.BuildFailed}
		assert.Equal(t, want[record.TaskID][arm], got, "%s/%s/%d", record.TaskID, record.Arm, record.Trial)
		if record.TaskID == "GT-AGENT-X07" {
			assert.Equal(t, [2]int{2, 2}, [2]int{record.Oracle.ExpectedPassed, record.Oracle.ExpectedFailed},
				"the fix that refuses every request passes the task's own assertions and fails its positive control")
		}
	}
	verdict, err := ComputeVerdict(session)
	require.NoError(t, err)
	assert.Equal(t, [2]string{VerdictRegression, ReasonHardFlip}, [2]string{verdict.Verdict, verdict.Reason})
	assert.Equal(t, []string{"GT-AGENT-X01"}, verdict.HardFlips)
	assert.Equal(t, [4]int{2, 22, 0, 22}, [4]int{verdict.Arms.Baseline.Passes, verdict.Arms.Baseline.Valid,
		verdict.Arms.Candidate.Passes, verdict.Arms.Candidate.Valid})
	assert.InDelta(t, -2.0/22, verdict.RegressionDelta, 1e-9)
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
