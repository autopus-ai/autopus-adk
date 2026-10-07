package harneval

import (
	"errors"
	"fmt"
	"slices"
)

// ErrRecordsProtocolMismatch is the records_protocol_mismatch refusal: the
// records do not reconcile with the protocol, so no report is written.
var ErrRecordsProtocolMismatch = errors.New("records_protocol_mismatch")

// reconcile checks the session documents against each other, calibration
// first (REQ-HE-10). Without a passed before calibration no trial may have
// run, so no record may exist; with one, the records must hold every
// scheduled (task, arm, trial) of this session exactly once. A failed or
// absent after calibration does not change what the records must hold: it
// runs once every trial has ended.
func reconcile(s *Session) error {
	if err := checkCalibration(s); err != nil {
		return err
	}
	if s.Calibration == nil || s.Calibration.Before.Status != CalibrationPassed {
		if len(s.Records) > 0 {
			return fmt.Errorf("%w: calibration did not pass before the first trial, yet %d records exist",
				ErrRecordsProtocolMismatch, len(s.Records))
		}
		return nil
	}
	return reconcileRecords(s.Protocol, s.Records)
}

// checkCalibration ties calibration.json to the protocol: the same session,
// the before calibration the protocol froze, and, when the after calibration
// passed, exactly the scheduled tasks.
func checkCalibration(s *Session) error {
	calibration := s.Calibration
	if calibration == nil {
		return nil
	}
	invalid := func(format string, args ...any) error {
		return withPath(invalidf(DetailFieldInvalid, format, args...), CalibrationFile)
	}
	frozen := s.Protocol.Calibration
	switch {
	case calibration.SessionID != s.Protocol.SessionID:
		return invalid("calibration session %s is not protocol session %s", calibration.SessionID, s.Protocol.SessionID)
	case calibration.Before.Status != frozen.Status || !slices.Equal(calibration.Before.Tasks, frozen.Tasks):
		return invalid("before differs from the calibration frozen in %s", ProtocolFile)
	case calibration.After != nil && calibration.After.Status == CalibrationPassed && !phaseCovers(*calibration.After, s.Protocol.Order):
		return invalid("after passed without exactly the scheduled tasks")
	}
	return nil
}

// reconcileRecords requires the records to hold every scheduled attempt of
// the session exactly once. Record numbers are 1-based, as are the lines of
// records.jsonl.
func reconcileRecords(protocol Protocol, records []Record) error {
	recorded := make(map[Attempt]bool, len(protocol.Order))
	for _, attempt := range protocol.Order {
		recorded[attempt] = false
	}
	for index, record := range records {
		attempt := Attempt{TaskID: record.TaskID, Arm: record.Arm, Trial: record.Trial}
		seen, scheduled := recorded[attempt]
		switch {
		case record.SessionID != protocol.SessionID:
			return fmt.Errorf("%w: record %d belongs to session %s, not %s",
				ErrRecordsProtocolMismatch, index+1, record.SessionID, protocol.SessionID)
		case !scheduled:
			return fmt.Errorf("%w: record %d (%s) is not scheduled", ErrRecordsProtocolMismatch, index+1, attempt)
		case seen:
			return fmt.Errorf("%w: record %d (%s) repeats an earlier record", ErrRecordsProtocolMismatch, index+1, attempt)
		}
		recorded[attempt] = true
	}
	for _, attempt := range protocol.Order {
		if !recorded[attempt] {
			return fmt.Errorf("%w: attempt %s has no record", ErrRecordsProtocolMismatch, attempt)
		}
	}
	return nil
}
