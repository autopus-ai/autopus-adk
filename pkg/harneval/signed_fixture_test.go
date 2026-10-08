package harneval

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// signedSessionID is the session id of every signed-lane fixture.
const signedSessionID = "5e55105e55105e55105e55105e55105e"

// signedStart is the protocol started_at of every signed-lane fixture.
var signedStart = time.Date(2026, 10, 7, 1, 2, 3, 0, time.UTC)

// The four attempts of the one-task K=2 fixture order.
var (
	signedB0 = Attempt{TaskID: "GT-AG-001", Arm: ArmBaseline, Trial: 0}
	signedC0 = Attempt{TaskID: "GT-AG-001", Arm: ArmCandidate, Trial: 0}
	signedC1 = Attempt{TaskID: "GT-AG-001", Arm: ArmCandidate, Trial: 1}
	signedB1 = Attempt{TaskID: "GT-AG-001", Arm: ArmBaseline, Trial: 1}
)

// signedRun is a signed-lane live result, by default the S4 session: one
// black-box task GT-AG-001 with K=2 whose baseline passes both trials and
// whose candidate passes trial 0 and fails trial 1. Every document is a Go
// value until attest renders the bytes and the attestations that name them.
type signedRun struct {
	t           *testing.T
	trusted     TrustedProtocol
	meta        RunMeta
	protocol    Protocol
	trials      []signedTrial
	calibration Calibration
	boundAt     time.Time
	resultAt    time.Time
}

// signedTrial is one record and the oracle result bytes its trial produced,
// nil when the trial wrote none.
type signedTrial struct {
	record Record
	result []byte
}

func newSignedRun(t *testing.T) *signedRun {
	t.Helper()
	trusted, err := RebuildTrustedProtocol(loadedSet(t, nil), trustedInputs("GT-AG-001"))
	require.NoError(t, err)
	passed := func() CalibrationPhase {
		return CalibrationPhase{Status: CalibrationPassed, Tasks: []CalibrationTask{{TaskID: "GT-AG-001", CleanAccepted: true}}}
	}
	after := passed()
	corpus := []json.RawMessage{}
	for _, row := range trusted.CorpusDigests {
		corpus = append(corpus, json.RawMessage(mustJSON(t, row)))
	}
	run := &signedRun{
		t: t, trusted: trusted,
		meta: RunMeta{RunID: 18234567890, RunAttempt: 1, RunCreatedAt: "2026-10-07T00:40:00Z", AttemptStartedAt: "2026-10-07T00:40:05Z"},
		protocol: Protocol{
			SchemaVersion: ProtocolSchemaV1, SessionID: signedSessionID, StartedAt: signedStart.Format(time.RFC3339),
			WorkspaceRevision: trusted.WorkspaceRevision, BaselineRef: trusted.BaselineRef,
			BaselineSurfaceDigest: trusted.BaselineSurfaceDigest, CandidateSurfaceDigest: trusted.CandidateSurfaceDigest,
			AgentSetDigest: trusted.AgentSetDigest, CorpusDigests: corpus,
			RunnerSHA256: strings.Repeat("5c", 32), GraderProfileSHA256: strings.Repeat("9a", 32),
			Calibration: passed(), Policy: trusted.Policy, Pins: trusted.Pins, CLIVersion: trusted.CLIVersion,
			Model: trusted.Model, Order: trusted.Order,
			PromptLayers: []json.RawMessage{json.RawMessage(`{"layer":"stable","identifiers":["agent_set_digest"]}`)},
			RunID:        18234567890, RunAttempt: 1, BindingDigest: trusted.BindingDigest,
			BaselineCommit: trusted.BaselineCommit, RunnerTreeDigest: trusted.RunnerTreeDigest,
		},
		calibration: Calibration{SchemaVersion: CalibrationSchemaV1, SessionID: signedSessionID, Before: passed(), After: &after},
		boundAt:     signedStart.Add(-10 * time.Minute),
		resultAt:    signedStart.Add(2 * time.Hour),
	}
	for _, attempt := range trusted.Order {
		run.trials = append(run.trials, run.accepted(attempt))
	}
	run.put(run.mismatched(signedC1))
	return run
}

// trial builds a black-box trial: the record of attempt a and, when result is
// not nil, the oracle result bytes its digest names.
func (r *signedRun) trial(a Attempt, stage string, term AgentTermination, outcome, signal string,
	oracle OracleObservation, result []byte) signedTrial {
	record := Record{
		SchemaVersion: RecordSchemaV1, SessionID: signedSessionID, TaskID: a.TaskID, Arm: a.Arm, Trial: a.Trial,
		Outcome: outcome, Signal: signal, Oracle: &oracle, DurationS: 41.5,
		StageReached: stage, AgentTermination: &term,
	}
	if result != nil {
		digest := sha256Hex(result)
		record.OracleResultSHA256 = &digest
	}
	return signedTrial{record: record, result: result}
}

// accepted is a trial whose agent exited 0 and whose output met every
// assertion (REQ-HR-08 row 10).
func (r *signedRun) accepted(a Attempt) signedTrial {
	return r.trial(a, StageOracle, agentExited(0), OutcomePass, "accepted", compared(3, 0), r.result(OutputCheckOK, true, true, true))
}

// mismatched is a trial whose output missed the second assertion (row 9).
func (r *signedRun) mismatched(a Attempt) signedTrial {
	return r.trial(a, StageOracle, agentExited(0), OutcomeFail, SignalExpectationMismatch, compared(2, 1),
		r.result(OutputCheckOK, true, false, true))
}

// put replaces the trial of the same attempt.
func (r *signedRun) put(trial signedTrial) {
	for index, current := range r.trials {
		if current.record.TaskID == trial.record.TaskID && current.record.Arm == trial.record.Arm &&
			current.record.Trial == trial.record.Trial {
			r.trials[index] = trial
			return
		}
	}
	r.t.Fatalf("no trial %s/%s/%d", trial.record.TaskID, trial.record.Arm, trial.record.Trial)
}

// result renders a harness_oracle_result.v1 document for GT-AG-001 whose
// assertions, in signedAssertions order, passed as given. A not_checked
// result is the oracle's view of an artifact that timed out.
func (r *signedRun) result(check string, passed ...bool) []byte {
	return oracleResultDoc(r.t, "GT-AG-001", check, signedAssertions[:len(passed)], passed...)
}

func oracleResultDoc(t *testing.T, task, check string, ids []string, passed ...bool) []byte {
	assertions := []map[string]any{}
	for index, ok := range passed {
		assertions = append(assertions, map[string]any{"id": ids[index], "passed": ok})
	}
	var exit any = 0
	if check == OutputCheckNotChecked {
		exit = nil
	}
	return []byte(mustJSON(t, map[string]any{
		"schema_version": OracleResultSchemaV1, "task_id": task, "output_check": check,
		"assertions": assertions, "artifact_exit": exit, "timed_out": check == OutputCheckNotChecked,
	}))
}

// attest renders the documents and the verified attestations that name
// their bytes, as the live-eval job would have attested them.
func (r *signedRun) attest() (SignerInput, AttestedSession) {
	r.t.Helper()
	in := SignerInput{Protocol: []byte(mustJSON(r.t, r.protocol)), Calibration: []byte(mustJSON(r.t, r.calibration))}
	var lines bytes.Buffer
	digests := []string{}
	for _, trial := range r.trials {
		lines.WriteString(mustJSON(r.t, trial.record) + "\n")
		if trial.result != nil {
			in.OracleResults = append(in.OracleResults, trial.result)
			digests = append(digests, sha256Hex(trial.result))
		}
	}
	in.Records = lines.Bytes()
	bound := BoundPredicate{RunID: r.meta.RunID, RunAttempt: r.meta.RunAttempt, BindingDigest: r.trusted.BindingDigest, Event: EventBound}
	result := SessionResultPredicate{
		BoundPredicate: bound, ProtocolSHA256: sha256Hex(in.Protocol), RecordsSHA256: sha256Hex(in.Records),
		OracleResultSHA256: digests, CalibrationSHA256: sha256Hex(in.Calibration),
	}
	result.Event = EventSessionResult
	return in, AttestedSession{Bound: bound, BoundLogTime: r.boundAt, Result: result, ResultLogTime: r.resultAt}
}

// verify runs the signer over the attested documents.
func (r *signedRun) verify() (*Session, error) {
	in, attested := r.attest()
	return VerifySignedSession(in, r.trusted, r.meta, attested)
}

func agentExited(code int) AgentTermination { return AgentTermination{Launched: true, ExitCode: &code} }

func agentKilled(signal string, timedOut bool) AgentTermination {
	return AgentTermination{Launched: true, OSSignal: &signal, TimedOut: timedOut}
}

func compared(passed, failed int) OracleObservation {
	return OracleObservation{Ran: true, ExpectedPassed: passed, ExpectedFailed: failed}
}

// replaceOnce swaps the single occurrence of old in data, failing the test
// when old does not occur exactly once.
func replaceOnce(t *testing.T, data []byte, old, replacement string) []byte {
	t.Helper()
	require.Equal(t, 1, bytes.Count(data, []byte(old)), "%q must occur once", old)
	return bytes.Replace(data, []byte(old), []byte(replacement), 1)
}
