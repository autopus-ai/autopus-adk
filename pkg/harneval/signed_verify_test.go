package harneval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifySignedSession_S4_AttestedSession_IsTheVerdictInput: the S4 session
// passes every signer check, and the session it returns is the attested
// documents, so the 001 verdict judges exactly them: delta -0.5 with no hard
// flip is a pass-rate regression.
func TestVerifySignedSession_S4_AttestedSession_IsTheVerdictInput(t *testing.T) {
	t.Parallel()
	run := newSignedRun(t)
	in, _ := run.attest()

	session, err := run.verify()

	require.NoError(t, err)
	assert.Equal(t, "2026-10-07T01:02:03Z", session.Protocol.StartedAt, "produced_at comes from this string")
	calibration, err := DecodeCalibration(in.Calibration)
	require.NoError(t, err)
	assert.Equal(t, &calibration, session.Calibration)
	records, err := DecodeRecords(in.Records)
	require.NoError(t, err)
	assert.Equal(t, records, session.Records)
	verdict, err := ComputeVerdict(session)
	require.NoError(t, err)
	assert.InDelta(t, -0.5, verdict.RegressionDelta, 1e-9)
	assert.Empty(t, verdict.HardFlips)
	assert.Equal(t, [2]int{2, 2}, [2]int{verdict.Arms.Baseline.Passes, verdict.Arms.Baseline.Valid})
	assert.Equal(t, [2]int{1, 2}, [2]int{verdict.Arms.Candidate.Passes, verdict.Arms.Candidate.Valid})
	assert.Equal(t, [2]string{VerdictRegression, ReasonPassRateRegression}, [2]string{verdict.Verdict, verdict.Reason})
}

// TestVerifySignedSession_S4_ChangedAfterAttestation_IsDigestMismatchFirst:
// series A. Bytes changed or removed after the live-eval job attested them
// stop the signer before any semantic check, even where a semantic check
// would also have failed or passed.
func TestVerifySignedSession_S4_ChangedAfterAttestation_IsDigestMismatchFirst(t *testing.T) {
	t.Parallel()
	failedAfter := func(r *signedRun) {
		r.calibration.After = &CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{
			{TaskID: "GT-AG-001", CleanAccepted: true, MutatedAccepted: true}}}
	}
	tamperedResult := func(t *testing.T, in *SignerInput) string {
		for index, data := range in.OracleResults {
			if sha256Hex(data) == *newSignedRun(t).trials[2].record.OracleResultSHA256 {
				in.OracleResults[index] = replaceOnce(t, data, `"task_id":"GT-AG-001"`, `"task_id":"GT-AG-002"`)
				return "oracle result " + sha256Hex(in.OracleResults[index]) + " is not attested"
			}
		}
		t.Fatal("no failing oracle result")
		return ""
	}
	tests := []struct {
		name   string
		base   func(r *signedRun)
		tamper func(t *testing.T, in *SignerInput) string
	}{
		{"protocol byte", nil, func(t *testing.T, in *SignerInput) string {
			in.Protocol = replaceOnce(t, in.Protocol, `"threshold_bp":-1000`, `"threshold_bp":-1001`)
			return ProtocolFile + " differs from protocol_sha256"
		}},
		{"record byte", nil, func(t *testing.T, in *SignerInput) string {
			in.Records = replaceOnce(t, in.Records, `"expected_passed":2`, `"expected_passed":3`)
			return RecordsFile + " differs from records_sha256"
		}},
		{"oracle result byte", nil, tamperedResult},
		{"calibration after failed to passed", failedAfter, func(t *testing.T, in *SignerInput) string {
			in.Calibration = replaceOnce(t, in.Calibration, `"status":"failed"`, `"status":"passed"`)
			return CalibrationFile + " differs from calibration_sha256"
		}},
		{"calibration removed", nil, func(t *testing.T, in *SignerInput) string {
			in.Calibration = nil
			return CalibrationFile + " is missing"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			if tt.base != nil {
				tt.base(run)
			}
			_, err := run.verify()
			require.NoError(t, err, "the untampered base must verify")
			in, attested := run.attest()
			detail := tt.tamper(t, &in)

			session, err := VerifySignedSession(in, run.trusted, run.meta, attested)

			assert.Nil(t, session)
			requireTrustError(t, err, ReasonAttestationDigestMismatch, detail)
		})
	}
}

// TestVerifySignedSession_AttestationNotOfThisAttempt_IsDigestMismatch: the
// bound and session_result attestations must be of the run attempt the run
// meta names and of the trusted binding, with a verified log time, and the
// received files must be exactly the attested ones.
func TestVerifySignedSession_AttestationNotOfThisAttempt_IsDigestMismatch(t *testing.T) {
	t.Parallel()
	extra := []byte(`{"task_id":"GT-AG-001","output_check":"too_large","assertions":[],"artifact_exit":0,"timed_out":false}`)
	tests := []struct {
		name, detail string
		edit         func(in *SignerInput, a *AttestedSession)
	}{
		{"bound event", "bound.event", func(_ *SignerInput, a *AttestedSession) { a.Bound.Event = EventSessionResult }},
		{"bound run", "bound.run_id", func(_ *SignerInput, a *AttestedSession) { a.Bound.RunID++ }},
		{"bound attempt", "bound.run_attempt", func(_ *SignerInput, a *AttestedSession) { a.Bound.RunAttempt = 2 }},
		{"bound binding", "bound.binding_digest", func(_ *SignerInput, a *AttestedSession) { a.Bound.BindingDigest = sha256Hex([]byte("other")) }},
		{"bound log time missing", "bound.log_time", func(_ *SignerInput, a *AttestedSession) { a.BoundLogTime = time.Time{} }},
		{"result event", "session_result.event", func(_ *SignerInput, a *AttestedSession) { a.Result.Event = EventBound }},
		{"result run", "session_result.run_id", func(_ *SignerInput, a *AttestedSession) { a.Result.RunID-- }},
		{"result attempt", "session_result.run_attempt", func(_ *SignerInput, a *AttestedSession) { a.Result.RunAttempt = 3 }},
		{"result binding", "session_result.binding_digest", func(_ *SignerInput, a *AttestedSession) { a.Result.BindingDigest = "" }},
		{"result log time missing", "session_result.log_time", func(_ *SignerInput, a *AttestedSession) { a.ResultLogTime = time.Time{} }},
		{"protocol missing", ProtocolFile + " is missing", func(in *SignerInput, _ *AttestedSession) { in.Protocol = nil }},
		{"records missing", RecordsFile + " is missing", func(in *SignerInput, _ *AttestedSession) { in.Records = nil }},
		{"records emptied", RecordsFile + " differs from records_sha256", func(in *SignerInput, _ *AttestedSession) { in.Records = []byte{} }},
		{"oracle result missing", "oracle result " + sha256Hex(newSignedRun(t).trials[2].result) + " is missing",
			func(in *SignerInput, _ *AttestedSession) { in.OracleResults = in.OracleResults[:2] }},
		{"oracle result added", "oracle result " + sha256Hex(extra) + " is not attested",
			func(in *SignerInput, _ *AttestedSession) { in.OracleResults = append(in.OracleResults, extra) }},
		{"attested result list emptied", "oracle result " + sha256Hex(newSignedRun(t).trials[0].result) + " is not attested",
			func(_ *SignerInput, a *AttestedSession) { a.Result.OracleResultSHA256 = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			in, attested := run.attest()
			tt.edit(&in, &attested)

			session, err := VerifySignedSession(in, run.trusted, run.meta, attested)

			assert.Nil(t, session)
			requireTrustError(t, err, ReasonAttestationDigestMismatch, tt.detail)
		})
	}
}

// TestVerifySignedSession_S4_WrongContentAttested_IsRefusedByMeaning: series
// B. Content that was wrong when it was attested passes the digest check and
// is refused by the semantic checks, each with its reason and detail.
func TestVerifySignedSession_S4_WrongContentAttested_IsRefusedByMeaning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, reason, detail string
		edit                 func(r *signedRun)
	}{
		{"B1 threshold out of the trusted policy", ReasonProtocolMismatch, "policy.threshold_bp",
			func(r *signedRun) { r.protocol.Policy.ThresholdBP = -10000 }},
		{"B2 attempt other than the run meta", ReasonProtocolMismatch, "run_attempt",
			func(r *signedRun) { r.protocol.RunAttempt = 2 }},
		{"B3 started before the bound log time", ReasonProtocolMismatch, "started_at",
			func(r *signedRun) { r.boundAt = signedStart.Add(time.Second) }},
		{"B4 timed-out agent recorded as a pass", ReasonOutcomeDerivationMismatch,
			"GT-AG-001/candidate/0 records pass/accepted, the table gives fail/agent_timeout", func(r *signedRun) {
				r.put(r.trial(signedC0, StageOracle, agentKilled("SIGKILL", true), OutcomePass, "accepted", compared(3, 0),
					r.result(OutputCheckOK, true, true, true)))
			}},
		{"B5 duplicate record", ReasonRecordsProtocolMismatch, "duplicate GT-AG-001/candidate/0",
			func(r *signedRun) {
				r.trials = append(r.trials[:2], append([]signedTrial{r.trials[1]}, r.trials[2:]...)...)
			}},
		{"B6 passed after calibration with a mutated pass", ReasonOutcomeDerivationMismatch,
			"calibration.json after.status passed, its tasks give failed",
			func(r *signedRun) { r.calibration.After.Tasks[0].MutatedAccepted = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := newSignedRun(t)
			tt.edit(run)

			session, err := run.verify()

			assert.Nil(t, session)
			requireTrustError(t, err, tt.reason, tt.detail)
		})
	}
}

// TestVerifySignedSession_LogTimes_BoundTheStartOnly: started_at may sit at
// any instant from the bound log time's second to the session_result log
// time; the signer reads no clock, so signing long after the session works.
func TestVerifySignedSession_LogTimes_BoundTheStartOnly(t *testing.T) {
	t.Parallel()
	for _, edit := range []func(r *signedRun){
		func(r *signedRun) { r.boundAt = signedStart.Add(900 * time.Millisecond) },
		func(r *signedRun) { r.boundAt = signedStart },
		func(r *signedRun) { r.resultAt = signedStart },
		func(r *signedRun) { r.resultAt = signedStart.Add(30 * time.Hour) },
	} {
		run := newSignedRun(t)
		edit(run)

		_, err := run.verify()

		assert.NoError(t, err)
	}
	run := newSignedRun(t)
	run.resultAt = signedStart.Add(-time.Second)
	_, err := run.verify()
	requireTrustError(t, err, ReasonProtocolMismatch, "started_at")
}
