package harneval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusedPhase is a failed calibration: GT-AGENT-A02's oracle failed on the
// clean workspace, so the runner refused the session before any trial.
var refusedPhase = CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{
	{TaskID: taskA, CleanAccepted: true}, {TaskID: taskB}, {TaskID: taskC, CleanAccepted: true},
}}

// refuse turns a session into S10's refused session: protocol and
// calibration.json record the failed calibration and no record exists.
func refuse(s *Session) {
	s.Protocol.Calibration = refusedPhase
	s.Calibration.Before, s.Calibration.After, s.Records = refusedPhase, nil, []Record{}
}

// brokenAfter is a failed end-of-session calibration: GT-AGENT-A02's oracle
// accepted the mutated workspace once every trial had ended.
var brokenAfter = CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{
	{TaskID: taskA, CleanAccepted: true}, {TaskID: taskB, CleanAccepted: true, MutatedAccepted: true}, {TaskID: taskC, CleanAccepted: true},
}}

// stop turns a session into one that stopped after its first n trials: the
// runner appended n records in order and never recalibrated.
func stop(n int) func(s *Session) {
	return func(s *Session) { s.Calibration.After, s.Records = nil, s.Records[:n] }
}

// TestComputeVerdict_CalibrationFirst_UnprovenOracleIsVacuous: calibration is
// judged before any record, and a session whose oracle calibration did not
// pass at both ends is vacuous whatever its records say. A session stopped
// before its last trial never recalibrated, so its calibration is missing.
func TestComputeVerdict_CalibrationFirst_UnprovenOracleIsVacuous(t *testing.T) {
	t.Parallel()
	missing := CalibrationPhase{Status: CalibrationMissing, Tasks: s9Live().session().Calibration.Before.Tasks}
	tests := []struct {
		name                string
		edit                func(s *Session)
		calibration         CalibrationPhase
		baseline, candidate armWant
		completeness        float64
		flips               []string
	}{
		{"S10 refused before the first trial", refuse, refusedPhase, armWant{}, armWant{}, 0, []string{}},
		{"calibration.json absent", func(s *Session) { s.Calibration, s.Records = nil, []Record{} },
			CalibrationPhase{Status: CalibrationMissing, Tasks: []CalibrationTask{}}, armWant{}, armWant{}, 0, []string{}},
		{"S10 end-of-session calibration failed", func(s *Session) { s.Calibration.After = &brokenAfter }, brokenAfter,
			armWant{5, 6, rate(0.8333333333)}, armWant{3, 5, rate(0.6)}, 0.9166666667, []string{taskA}},
		{"end-of-session calibration absent", func(s *Session) { s.Calibration.After = nil }, missing,
			armWant{5, 6, rate(0.8333333333)}, armWant{3, 5, rate(0.6)}, 0.9166666667, []string{taskA}},
		{"S10 stopped before the first trial", stop(0), missing, armWant{}, armWant{}, 0, []string{}},
		{"S10 stopped before the last trial", stop(7), missing,
			armWant{3, 3, rate(1)}, armWant{2, 4, rate(0.5)}, 0.5833333333, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := s9Live().session()
			tt.edit(session)

			verdict, err := ComputeVerdict(session)

			require.NoError(t, err)
			assert.Equal(t, VerdictVacuous, verdict.Verdict)
			assert.Equal(t, ReasonOracleCalibrationFailed, verdict.Reason)
			assert.Equal(t, tt.calibration, verdict.Calibration)
			assertArm(t, tt.baseline, verdict.Arms.Baseline, ArmBaseline)
			assertArm(t, tt.candidate, verdict.Arms.Candidate, ArmCandidate)
			assert.InDelta(t, tt.completeness, verdict.Completeness, 1e-9, "the records still count; the verdict does not")
			assert.Equal(t, tt.flips, verdict.HardFlips)
		})
	}
}

// TestComputeVerdict_RecordsMustReconcileWithTheOrder: without a passed
// before calibration no record may exist. With one, every record must be a
// scheduled (task, arm, trial) of this session at most once, and once the
// after calibration ran, every scheduled attempt must have its record.
func TestComputeVerdict_RecordsMustReconcileWithTheOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, fragment string
		edit           func(s *Session)
	}{
		{"records after a refused calibration", "calibration did not pass before the first trial, yet 12 records exist", func(s *Session) {
			records := s.Records
			refuse(s)
			s.Records = records
		}},
		{"records without calibration.json", "yet 12 records exist", func(s *Session) { s.Calibration = nil }},
		{"a record of another session", "record 4 belongs to session " + strings.Repeat("0", 32), func(s *Session) {
			s.Records[3].SessionID = strings.Repeat("0", 32)
		}},
		{"a record outside the order", "record 5 (GT-AGENT-A09/baseline/0) is not scheduled", func(s *Session) { s.Records[4].TaskID = "GT-AGENT-A09" }},
		{"a repeated record", "record 12 (GT-AGENT-A01/baseline/0) repeats", func(s *Session) { s.Records[11] = s.Records[0] }},
		{"a missing record", "attempt GT-AGENT-A03/baseline/1 has no record", func(s *Session) { s.Records = s.Records[:11] }},
		{"a missing record after a failed end-of-session calibration", "attempt GT-AGENT-A03/baseline/1 has no record", func(s *Session) {
			s.Calibration.After, s.Records = &brokenAfter, s.Records[:11]
		}},
		{"a stopped session repeating a record", "record 3 (GT-AGENT-A01/baseline/0) repeats", func(s *Session) {
			stop(2)(s)
			s.Records = append(s.Records, s.Records[0])
		}},
		{"a stopped session with a record outside the order", "record 2 (GT-AGENT-A09/candidate/0) is not scheduled", func(s *Session) {
			stop(2)(s)
			s.Records[1].TaskID = "GT-AGENT-A09"
		}},
		{"a stopped session with a record of another session", "record 1 belongs to session " + strings.Repeat("0", 32), func(s *Session) {
			stop(2)(s)
			s.Records[0].SessionID = strings.Repeat("0", 32)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := s9Live().session()
			tt.edit(session)

			_, err := ComputeVerdict(session)

			require.ErrorIs(t, err, ErrRecordsProtocolMismatch)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
}

// TestComputeVerdict_CalibrationMustMatchTheProtocol: calibration.json must
// belong to the session, repeat the calibration the protocol froze, and cover
// every scheduled task when its after phase passed.
func TestComputeVerdict_CalibrationMustMatchTheProtocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, fragment string
		edit           func(s *Session)
	}{
		{"calibration of another session", "calibration session " + strings.Repeat("0", 32), func(s *Session) {
			s.Calibration.SessionID = strings.Repeat("0", 32)
		}},
		{"before differs from the frozen calibration", "before differs from", func(s *Session) {
			s.Calibration.Before.Tasks = s.Calibration.Before.Tasks[:2]
		}},
		{"after passed without every scheduled task", "after passed without", func(s *Session) {
			s.Calibration.After.Tasks = s.Calibration.After.Tasks[:2]
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := s9Live().session()
			tt.edit(session)

			_, err := ComputeVerdict(session)

			var invalid *InvalidError
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, DetailFieldInvalid, invalid.Detail)
			assert.Equal(t, CalibrationFile, invalid.Path)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
}
