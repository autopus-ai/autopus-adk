package harneval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifySignedSession_S8_TableGivesEachTrialExactlyOneJudgement: candidate
// trial 1 of the S4 session takes each S8 variant. A record that carries the
// row's (outcome, signal, oracle.ran, oracle.build_failed) verifies, so the
// signer re-derived exactly that; refusal rows carry a plausible record the
// table does not give.
func TestVerifySignedSession_S8_TableGivesEachTrialExactlyOneJudgement(t *testing.T) {
	t.Parallel()
	never := AgentTermination{}
	notRun := OracleObservation{}
	built := OracleObservation{BuildFailed: true}
	allPass := func(r *signedRun) []byte { return r.result(OutputCheckOK, true, true, true) }
	tests := []struct {
		name   string
		trial  func(r *signedRun) signedTrial
		refuse string
	}{
		{"row 1 workspace setup failed", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageSetup, never, OutcomeError, "workspace_setup_failed", notRun, nil)
		}, ""},
		{"row 2 agent timeout, then every assertion passed", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentKilled("SIGKILL", true), OutcomeFail, "agent_timeout", compared(3, 0), allPass(r))
		}, ""},
		{"row 2 a signal ending with a timeout is no launch failure", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentKilled("SIGKILL", true), OutcomeFail, "agent_launch_failed", compared(3, 0), allPass(r))
		}, "GT-AG-001/candidate/1 records fail/agent_launch_failed, the table gives fail/agent_timeout"},
		{"row 2 agent exited nonzero, then the build failed", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageBuild, agentExited(2), OutcomeFail, "agent_exit_nonzero", built, nil)
		}, ""},
		{"row 2 the agent failure outranks the build failure", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageBuild, agentExited(2), OutcomeFail, SignalArtifactBuildFailed, built, nil)
		}, "GT-AG-001/candidate/1 records fail/artifact_build_failed, the table gives fail/agent_exit_nonzero"},
		{"row 2 agent never launched, the unrepaired artifact graded", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, never, OutcomeFail, "agent_launch_failed", compared(2, 1), r.result(OutputCheckOK, true, false, true))
		}, ""},
		{"row 2 agent ended by a signal before its timeout", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentKilled("SIGSEGV", false), OutcomeFail, "agent_exit_nonzero", compared(3, 0), allPass(r))
		}, ""},
		{"row 3 leftover processes not confirmed gone", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageRun, agentExited(0), OutcomeFail, "observation_failed", notRun, nil)
		}, ""},
		{"row 4 scope violation skips build and oracle", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageAgent, agentExited(0), OutcomeFail, "scope_violation", notRun, nil)
		}, ""},
		{"row 4 a scope violation is only judged at the agent stage", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, "scope_violation", compared(3, 0), allPass(r))
		}, "GT-AG-001/candidate/1 records fail/scope_violation, the table gives pass/accepted"},
		{"row 5 artifact build failed", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageBuild, agentExited(0), OutcomeFail, SignalArtifactBuildFailed, built, nil)
		}, ""},
		{"row 5 a build failure sets build_failed", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageBuild, agentExited(0), OutcomeFail, SignalArtifactBuildFailed, notRun, nil)
		}, "GT-AG-001/candidate/1 records oracle {Ran:false BuildFailed:false ExpectedPassed:0 ExpectedFailed:0}, " +
			"the table gives {Ran:false BuildFailed:true ExpectedPassed:0 ExpectedFailed:0}"},
		{"row 6 harness wrote no result", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, notRun, nil)
		}, ""},
		{"row 6 result breaks the schema", func(r *signedRun) signedTrial {
			bad := []byte(strings.Replace(string(allPass(r)), `"timed_out":false`, `"timed_out":false,"verdict":"pass"`, 1))
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, notRun, bad)
		}, ""},
		{"row 6 result judges another task", func(r *signedRun) signedTrial {
			other := oracleResultDoc(r.t, "GT-AG-002", OutputCheckOK, signedAssertions, true, true, true)
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, notRun, other)
		}, ""},
		{"row 6 assertion ids other than main's", func(r *signedRun) signedTrial {
			partial := oracleResultDoc(r.t, "GT-AG-001", OutputCheckOK, []string{"exit_status", "stdout", "extra"}, true, true, true)
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, notRun, partial)
		}, ""},
		{"row 6 an unchecked output without a timeout", func(r *signedRun) signedTrial {
			unchecked := []byte(v1 + `"task_id":"GT-AG-001","output_check":"not_checked","assertions":[],"artifact_exit":0,"timed_out":false}`)
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, OracleObservation{}, unchecked)
		}, ""},
		{"row 6 an empty comparison is no pass", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomePass, "accepted", compared(0, 0), r.result(OutputCheckOK))
		}, "GT-AG-001/candidate/1 records pass/accepted, the table gives fail/oracle_harness_error"},
		{"row 7 artifact timed out", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalArtifactTimeout, notRun, r.result(OutputCheckNotChecked))
		}, ""},
		{"row 8 output link rejected", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOutputLinkRejected, notRun, r.result(OutputCheckLinkRejected))
		}, ""},
		{"row 8 output too large", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOutputTooLarge, notRun, r.result(OutputCheckTooLarge))
		}, ""},
		{"row 9 an assertion failed", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalExpectationMismatch, compared(1, 2), r.result(OutputCheckOK, false, false, true))
		}, ""},
		{"row 9 counts come from the result", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalExpectationMismatch, compared(2, 1), r.result(OutputCheckOK, false, false, true))
		}, "GT-AG-001/candidate/1 records oracle {Ran:true BuildFailed:false ExpectedPassed:2 ExpectedFailed:1}, " +
			"the table gives {Ran:true BuildFailed:false ExpectedPassed:1 ExpectedFailed:2}"},
		{"row 10 every assertion passed", func(r *signedRun) signedTrial { return r.accepted(signedC1) }, ""},
		{"a signal outside the table", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, "forbidden_construct", compared(2, 1), r.result(OutputCheckOK, true, false, true))
		}, "GT-AG-001/candidate/1 records fail/forbidden_construct, the table gives fail/expectation_mismatch"},
		{"no row for a pass that never reached the oracle", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageRun, agentExited(0), OutcomePass, "accepted", notRun, nil)
		}, "GT-AG-001/candidate/1 at stage run with signal accepted matches no row"},
		{"no row for a setup stage without a setup signal", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageSetup, agentExited(0), OutcomePass, "accepted", notRun, nil)
		}, "GT-AG-001/candidate/1 at stage setup with signal accepted matches no row"},
		{"row 3 observation_failed after the artifact ran", func(r *signedRun) signedTrial {
			return r.trial(signedC1, StageRun, agentKilled("SIGTERM", false), OutcomeFail, "observation_failed", notRun, nil)
		}, "GT-AG-001/candidate/1 records fail/observation_failed, the table gives fail/agent_exit_nonzero"},
		{"a white-box record in the signed lane", func(r *signedRun) signedTrial {
			trial := r.mismatched(signedC1)
			trial.record.StageReached, trial.record.AgentTermination, trial.record.OracleResultSHA256 = "", nil, nil
			trial.record.Signal, trial.result = "oracle_failed", nil
			return trial
		}, "GT-AG-001/candidate/1 carries no black-box observation"},
		{"a digest naming no attested result", func(r *signedRun) signedTrial {
			trial := r.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, notRun, nil)
			dangling := strings.Repeat("ab", 32)
			trial.record.OracleResultSHA256 = &dangling
			return trial
		}, "GT-AG-001/candidate/1 names oracle result " + strings.Repeat("ab", 32) + " that is not attested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			run.put(tt.trial(run))

			session, err := run.verify()

			if tt.refuse != "" {
				assert.Nil(t, session)
				requireTrustError(t, err, ReasonOutcomeDerivationMismatch, tt.refuse)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, run.trials[2].record, session.Records[2])
			_, err = ComputeVerdict(session)
			assert.NoError(t, err, "a verified session is always judged")
		})
	}
}

// TestVerifySignedSession_UnreferencedOracleResult_IsRefused: every attested
// oracle result must be the result of some trial.
func TestVerifySignedSession_UnreferencedOracleResult_IsRefused(t *testing.T) {
	t.Parallel()
	run := newSignedRun(t)
	orphan := run.result(OutputCheckLinkRejected)
	trial := run.trial(signedC1, StageOracle, agentExited(0), OutcomeFail, SignalOracleHarnessError, OracleObservation{}, orphan)
	trial.record.OracleResultSHA256 = nil
	run.put(trial)

	_, err := run.verify()

	requireTrustError(t, err, ReasonOutcomeDerivationMismatch, "oracle result "+sha256Hex(orphan)+" belongs to no trial")
}

// TestVerifySignedSession_S8_EnvironmentFailureEverywhere_IsVacuous: when
// every trial of both arms failed the same way before any comparison, the
// sessions verify and the 001 vacuity rule keeps their verdict from ok.
func TestVerifySignedSession_S8_EnvironmentFailureEverywhere_IsVacuous(t *testing.T) {
	t.Parallel()
	failures := map[string]func(r *signedRun, a Attempt) signedTrial{
		SignalArtifactBuildFailed: func(r *signedRun, a Attempt) signedTrial {
			return r.trial(a, StageBuild, agentExited(0), OutcomeFail, SignalArtifactBuildFailed, OracleObservation{BuildFailed: true}, nil)
		},
		SignalArtifactTimeout: func(r *signedRun, a Attempt) signedTrial {
			return r.trial(a, StageOracle, agentExited(0), OutcomeFail, SignalArtifactTimeout, OracleObservation{}, r.result(OutputCheckNotChecked))
		},
		SignalOutputLinkRejected: func(r *signedRun, a Attempt) signedTrial {
			return r.trial(a, StageOracle, agentExited(0), OutcomeFail, SignalOutputLinkRejected, OracleObservation{}, r.result(OutputCheckLinkRejected))
		},
	}
	for signal, failure := range failures {
		t.Run(signal, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			for _, attempt := range run.trusted.Order {
				run.put(failure(run, attempt))
			}

			session, err := run.verify()

			require.NoError(t, err)
			verdict, err := ComputeVerdict(session)
			require.NoError(t, err)
			assert.Equal(t, [2]string{VerdictVacuous, ReasonOracleNotRun}, [2]string{verdict.Verdict, verdict.Reason})
		})
	}
}
