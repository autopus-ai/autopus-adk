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

// TestComputeVerdict_CalibrationFirst_UnprovenOracleIsVacuous: calibration is
// judged before any record, and a session whose oracle calibration did not
// pass at both ends is vacuous whatever its records say.
func TestComputeVerdict_CalibrationFirst_UnprovenOracleIsVacuous(t *testing.T) {
	t.Parallel()
	brokenAfter := CalibrationPhase{Status: CalibrationFailed, Tasks: []CalibrationTask{
		{TaskID: taskA, CleanAccepted: true}, {TaskID: taskB, CleanAccepted: true, MutatedAccepted: true}, {TaskID: taskC, CleanAccepted: true},
	}}
	passedTasks := s9Live().session().Calibration.Before.Tasks
	tests := []struct {
		name        string
		edit        func(s *Session)
		calibration CalibrationPhase
		noRecords   bool
	}{
		{"S10 refused before the first trial", refuse, refusedPhase, true},
		{"calibration.json absent", func(s *Session) { s.Calibration, s.Records = nil, []Record{} },
			CalibrationPhase{Status: CalibrationMissing, Tasks: []CalibrationTask{}}, true},
		{"S10 end-of-session calibration failed", func(s *Session) { s.Calibration.After = &brokenAfter }, brokenAfter, false},
		{"end-of-session calibration absent", func(s *Session) { s.Calibration.After = nil },
			CalibrationPhase{Status: CalibrationMissing, Tasks: passedTasks}, false},
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
			if tt.noRecords {
				assert.Equal(t, Arms{}, verdict.Arms, "no valid trial, both pass rates null")
				assert.Zero(t, verdict.RegressionDelta)
				assert.Zero(t, verdict.Completeness)
				assert.Equal(t, []string{}, verdict.HardFlips)
			} else {
				assert.Equal(t, []string{taskA}, verdict.HardFlips, "the records still count; the verdict does not")
			}
		})
	}
}

// TestComputeVerdict_RecordsMustReconcileWithTheOrder: without a passed
// before calibration no record may exist; with one, the records must hold
// every scheduled (task, arm, trial) of this session exactly once.
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
		{"no record after a passed calibration", "attempt GT-AGENT-A01/baseline/0 has no record", func(s *Session) {
			s.Calibration.After, s.Records = nil, []Record{}
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
