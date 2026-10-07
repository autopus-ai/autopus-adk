package harneval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Agent task ids of the in-memory sessions.
const (
	taskA = "GT-AGENT-A01"
	taskB = "GT-AGENT-A02"
	taskC = "GT-AGENT-A03"
	taskD = "GT-AGENT-A04"
	taskE = "GT-AGENT-A05"
)

// liveBuilder makes sessions in memory whose documents meet the per-document
// contract: tasks in id order, K trials, the balanced order, both calibrations
// passed, and one record per attempt, a pass unless set.
type liveBuilder struct {
	tasks    []string
	k        int
	floor    float64
	outcomes map[Attempt]string
}

func newLive(k int, tasks ...string) *liveBuilder {
	return &liveBuilder{tasks: tasks, k: k, floor: 0.9, outcomes: map[Attempt]string{}}
}

// set gives one task arm its outcomes in trial order: pass, fail, error,
// build (a fail whose oracle never built, so it never ran), ghost (an error
// whose oracle claims to have run, which no trusted parser writes and
// DecodeRecords rejects, so only an in-memory session holds one), agent (an
// agent that exited nonzero) or launch (an agent that never started); the
// last two are graded on the unrepaired workspace, so their oracle ran.
func (b *liveBuilder) set(task, arm string, outcomes ...string) *liveBuilder {
	for trial, outcome := range outcomes {
		b.outcomes[Attempt{TaskID: task, Arm: arm, Trial: trial}] = outcome
	}
	return b
}

func (b *liveBuilder) withFloor(floor float64) *liveBuilder {
	b.floor = floor
	return b
}

func (b *liveBuilder) session() *Session {
	phase := func() CalibrationPhase {
		tasks := []CalibrationTask{}
		for _, task := range b.tasks {
			tasks = append(tasks, CalibrationTask{TaskID: task, CleanAccepted: true})
		}
		return CalibrationPhase{Status: CalibrationPassed, Tasks: tasks}
	}
	after := phase()
	session := &Session{
		Protocol: Protocol{
			SessionID: sampleSessionID, Calibration: phase(),
			Policy: LivePolicy{K: b.k, ThresholdBP: -1000, CompletenessFloor: b.floor},
		},
		Calibration: &Calibration{SessionID: sampleSessionID, Before: phase(), After: &after},
		Records:     []Record{},
	}
	for trial := 0; trial < b.k; trial++ {
		for index, task := range b.tasks {
			arms := []string{ArmBaseline, ArmCandidate}
			if (trial+index)%2 == 1 {
				arms = []string{ArmCandidate, ArmBaseline}
			}
			for _, arm := range arms {
				attempt := Attempt{TaskID: task, Arm: arm, Trial: trial}
				session.Protocol.Order = append(session.Protocol.Order, attempt)
				session.Records = append(session.Records, liveRecord(attempt, b.outcomes[attempt]))
			}
		}
	}
	return session
}

func liveRecord(attempt Attempt, outcome string) Record {
	record := Record{SessionID: sampleSessionID, TaskID: attempt.TaskID, Arm: attempt.Arm, Trial: attempt.Trial, Oracle: &OracleObservation{}}
	switch outcome {
	case "", "pass":
		record.Outcome, record.Signal, record.Oracle.Ran, record.Oracle.ExpectedPassed = OutcomePass, "accepted", true, 1
	case "fail":
		record.Outcome, record.Signal, record.Oracle.Ran, record.Oracle.ExpectedFailed = OutcomeFail, "oracle_failed", true, 1
	case "build":
		record.Outcome, record.Signal, record.Oracle.BuildFailed = OutcomeFail, "oracle_failed", true
	case "agent", "launch":
		record.Outcome, record.Signal, record.Oracle.Ran, record.Oracle.ExpectedFailed = OutcomeFail, "agent_exit_nonzero", true, 1
		if outcome == "launch" {
			record.Signal = "agent_launch_failed"
		}
	case "error", "ghost":
		record.Outcome, record.Signal, record.Oracle.Ran = OutcomeError, "warmup_failed", outcome == "ghost"
	}
	return record
}

// armWant is one arm's expected totals; a nil rate is a JSON null pass_rate.
// rate (summary_test.go) makes the non-null ones.
type armWant struct {
	passes, valid int
	rate          *float64
}

func assertArm(t *testing.T, want armWant, got ArmTotals, arm string) {
	t.Helper()
	assert.Equal(t, want.passes, got.Passes, arm+" passes")
	assert.Equal(t, want.valid, got.Valid, arm+" valid")
	if want.rate == nil {
		assert.Nil(t, got.PassRate, arm+" pass_rate")
		return
	}
	require.NotNil(t, got.PassRate, arm+" pass_rate")
	assert.InDelta(t, *want.rate, *got.PassRate, 1e-9, arm+" pass_rate")
}

// s9Live is S9's first record set: T1 baseline [pass,pass] candidate
// [fail,fail], T2 baseline [pass,fail] candidate [pass,pass], T3 baseline
// [pass,pass] candidate [pass,error].
func s9Live() *liveBuilder {
	return newLive(2, taskA, taskB, taskC).
		set(taskA, ArmBaseline, "pass", "pass").set(taskA, ArmCandidate, "fail", "fail").
		set(taskB, ArmBaseline, "pass", "fail").set(taskB, ArmCandidate, "pass", "pass").
		set(taskC, ArmBaseline, "pass", "pass").set(taskC, ArmCandidate, "pass", "error")
}

func TestComputeVerdict_FormulaAndPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		live                *liveBuilder
		baseline, candidate armWant
		delta, completeness float64
		flips               []string
		verdict, reason     string
	}{
		{"S9 hard flip outranks the pass rate", s9Live(), armWant{5, 6, rate(0.8333333333)}, armWant{3, 5, rate(0.6)},
			-0.2333333333, 0.9166666667, []string{taskA}, VerdictRegression, ReasonHardFlip},
		{"S9 small drop stays within threshold", s9Live().set(taskA, ArmCandidate, "pass", "fail"),
			armWant{5, 6, rate(0.8333333333)}, armWant{4, 5, rate(0.8)}, -0.0333333333, 0.9166666667, nil, VerdictOK, ReasonWithinThreshold},
		{"S9 errors below the completeness floor", s9Live().set(taskA, ArmCandidate, "pass", "fail").set(taskB, ArmCandidate, "error", "error"),
			armWant{5, 6, rate(0.8333333333)}, armWant{2, 3, rate(0.6666666667)}, -0.1666666667, 0.75, nil, VerdictIncomplete, ReasonCompletenessBelowFloor},
		{"S9 pass-rate regression without a hard flip",
			newLive(2, taskA, taskB, taskC).set(taskA, ArmCandidate, "pass", "fail").set(taskB, ArmCandidate, "fail", "pass").set(taskC, ArmCandidate, "fail", "pass"),
			armWant{6, 6, rate(1)}, armWant{3, 6, rate(0.5)}, -0.5, 1, nil, VerdictRegression, ReasonPassRateRegression},
		{"S9 delta exactly at the threshold is no regression", newLive(2, taskA, taskB, taskC, taskD, taskE).set(taskA, ArmCandidate, "pass", "fail"),
			armWant{10, 10, rate(1)}, armWant{9, 10, rate(0.9)}, -0.1, 1, nil, VerdictOK, ReasonWithinThreshold},
		{"S9 only errors leave both rates null", newLive(2, taskA, taskB, taskC).
			set(taskA, ArmBaseline, "error", "error").set(taskB, ArmBaseline, "error", "error").set(taskC, ArmBaseline, "error", "error").
			set(taskA, ArmCandidate, "error", "error").set(taskB, ArmCandidate, "error", "error").set(taskC, ArmCandidate, "error", "error"),
			armWant{0, 0, nil}, armWant{0, 0, nil}, 0, 0, nil, VerdictVacuous, ReasonOracleNotRun},
		{"S10 every trial failing to build is vacuous", newLive(2, taskA, taskB).
			set(taskA, ArmBaseline, "build", "build").set(taskB, ArmBaseline, "build", "build").
			set(taskA, ArmCandidate, "build", "build").set(taskB, ArmCandidate, "build", "build"),
			armWant{0, 4, rate(0)}, armWant{0, 4, rate(0)}, 0, 1, nil, VerdictVacuous, ReasonOracleNotRun},
		{"one arm without a run oracle is vacuous over hard flips", newLive(2, taskA, taskB).
			set(taskA, ArmCandidate, "build", "build").set(taskB, ArmCandidate, "build", "build"),
			armWant{4, 4, rate(1)}, armWant{0, 4, rate(0)}, -1, 1, []string{taskA, taskB}, VerdictVacuous, ReasonOracleNotRun},
		{"an error trial keeps its task out of the hard flips",
			newLive(2, taskA).withFloor(0.7).set(taskA, ArmCandidate, "fail", "error"),
			armWant{2, 2, rate(1)}, armWant{0, 1, rate(0)}, -1, 0.75, nil, VerdictRegression, ReasonPassRateRegression},
		{"an arm without a valid trial is incomplete at any floor",
			newLive(2, taskA).withFloor(0.5).set(taskA, ArmCandidate, "ghost", "ghost"),
			armWant{2, 2, rate(1)}, armWant{0, 0, nil}, 0, 0.5, nil, VerdictIncomplete, ReasonNoValidTrial},
		{"no trial past the agent stage in either arm is vacuous, not ok", newLive(2, taskA, taskB).
			set(taskA, ArmBaseline, "agent", "launch").set(taskB, ArmBaseline, "agent", "agent").
			set(taskA, ArmCandidate, "launch", "agent").set(taskB, ArmCandidate, "agent", "agent"),
			armWant{0, 4, rate(0)}, armWant{0, 4, rate(0)}, 0, 1, nil, VerdictVacuous, "agent_all_failed"},
		{"agent failures everywhere are vacuous before incomplete", newLive(2, taskA, taskB).
			set(taskA, ArmBaseline, "agent", "error").set(taskB, ArmBaseline, "agent", "agent").
			set(taskA, ArmCandidate, "agent", "agent").set(taskB, ArmCandidate, "agent", "agent"),
			armWant{0, 3, rate(0)}, armWant{0, 4, rate(0)}, 0, 0.875, nil, VerdictVacuous, "agent_all_failed"},
		{"S11 a candidate surface the agent cannot start with is a regression", newLive(2, taskA, taskB).
			set(taskA, ArmCandidate, "launch", "launch").set(taskB, ArmCandidate, "launch", "launch"),
			armWant{4, 4, rate(1)}, armWant{0, 4, rate(0)}, -1, 1, []string{taskA, taskB}, VerdictRegression, ReasonHardFlip},
		{"one trial past the agent stage keeps the session judged", newLive(2, taskA, taskB).
			set(taskA, ArmBaseline, "pass", "agent").set(taskB, ArmBaseline, "agent", "agent").
			set(taskA, ArmCandidate, "agent", "agent").set(taskB, ArmCandidate, "agent", "agent"),
			armWant{1, 4, rate(0.25)}, armWant{0, 4, rate(0)}, -0.25, 1, nil, VerdictRegression, ReasonPassRateRegression},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			verdict, err := ComputeVerdict(tt.live.session())

			require.NoError(t, err)
			assertArm(t, tt.baseline, verdict.Arms.Baseline, ArmBaseline)
			assertArm(t, tt.candidate, verdict.Arms.Candidate, ArmCandidate)
			assert.InDelta(t, tt.delta, verdict.RegressionDelta, 1e-9, "regression_delta")
			assert.InDelta(t, tt.completeness, verdict.Completeness, 1e-9, "completeness")
			if tt.flips == nil {
				tt.flips = []string{}
			}
			assert.Equal(t, tt.flips, verdict.HardFlips)
			assert.Equal(t, CalibrationPassed, verdict.Calibration.Status)
			assert.Equal(t, tt.verdict, verdict.Verdict)
			assert.Equal(t, tt.reason, verdict.Reason)
		})
	}
}

// TestComputeVerdict_AgentStageSignals_AreTheOnlyIncompleteAgentSteps: in a
// session whose oracle ran in every trial, each fail signal of the closed
// REQ-HE-08 table fills both arms alone. Only the agent-stage failures leave a
// session without a completed agent step; a fail after a completed agent step
// is a measurement, so 0 passes in both arms stays within the threshold.
func TestComputeVerdict_AgentStageSignals_AreTheOnlyIncompleteAgentSteps(t *testing.T) {
	t.Parallel()
	agentStage := map[string]bool{"agent_launch_failed": true, "agent_exit_nonzero": true,
		"agent_timeout": true, "observation_failed": true}
	fails := 0
	for signal, outcome := range signalOutcomes {
		if outcome != OutcomeFail {
			continue
		}
		fails++
		session := newLive(2, taskA, taskB).session()
		for index := range session.Records {
			session.Records[index].Outcome, session.Records[index].Signal = OutcomeFail, signal
		}

		verdict, err := ComputeVerdict(session)

		require.NoError(t, err, signal)
		want := [2]string{VerdictOK, ReasonWithinThreshold}
		if agentStage[signal] {
			want = [2]string{VerdictVacuous, "agent_all_failed"}
		}
		assert.Equal(t, want, [2]string{verdict.Verdict, verdict.Reason}, signal)
	}
	assert.Equal(t, 9, fails, "the REQ-HE-08 table has nine fail signals")
}
