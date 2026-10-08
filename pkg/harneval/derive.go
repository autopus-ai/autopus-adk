package harneval

import (
	"slices"
	"sort"
)

// VerifySignedSession runs every signer check of SPEC-HARNEVAL-003 REQ-HR-02
// and REQ-HR-09 over a received live result, in a fixed order: the trusted
// protocol's own limits and inputs; the attestation digests, before anything
// is decoded; the protocol against the trusted protocol and the attested run
// attempt; the record set against the trusted order; calibration.json; and
// the REQ-HR-08 table over every record and its oracle result. Nothing the
// runner reports is believed until it is re-derived from attested bytes.
//
// The returned session holds only the attested documents, so ComputeVerdict
// judges exactly them. A refusal is a *TrustError; a document that breaks its
// wire contract is an *InvalidError naming the file.
func VerifySignedSession(in SignerInput, trusted TrustedProtocol, meta RunMeta, attested AttestedSession) (*Session, error) {
	if err := trusted.validate(); err != nil {
		return nil, err
	}
	if !sha256Pattern.MatchString(trusted.BindingDigest) {
		return nil, invalidf(DetailFieldInvalid, "trusted binding_digest %q is not a 64-hex digest", trusted.BindingDigest)
	}
	if err := checkAttestation(in, trusted.BindingDigest, meta, attested); err != nil {
		return nil, err
	}
	protocol, err := DecodeProtocol(in.Protocol)
	if err != nil {
		return nil, withPath(err, ProtocolFile)
	}
	if path := protocolMismatch(protocol, trusted); path != "" {
		return nil, trustErr(ReasonProtocolMismatch, "%s", path)
	}
	if path := runMismatch(protocol, meta, attested); path != "" {
		return nil, trustErr(ReasonProtocolMismatch, "%s", path)
	}
	records, err := DecodeRecords(in.Records)
	if err != nil {
		return nil, err
	}
	if err := checkRecordSet(protocol.SessionID, trusted.Order, records); err != nil {
		return nil, err
	}
	calibration, err := checkSignedCalibration(in.Calibration, protocol, scheduledTasks(trusted.Order))
	if err != nil {
		return nil, err
	}
	results := make(map[string][]byte, len(in.OracleResults))
	for _, data := range in.OracleResults {
		results[sha256Hex(data)] = data
	}
	if err := deriveRecords(records, results, trusted.OracleAssertions); err != nil {
		return nil, err
	}
	return &Session{Protocol: protocol, Calibration: calibration, Records: records}, nil
}

// checkRecordSet requires the records to be exactly the trusted order's
// attempts, each once, all of the protocol's session.
func checkRecordSet(sessionID string, order []Attempt, records []Record) error {
	recorded := make(map[Attempt]bool, len(order))
	for _, attempt := range order {
		recorded[attempt] = false
	}
	for _, record := range records {
		attempt := Attempt{TaskID: record.TaskID, Arm: record.Arm, Trial: record.Trial}
		seen, scheduled := recorded[attempt]
		switch {
		case record.SessionID != sessionID:
			return trustErr(ReasonRecordsProtocolMismatch, "session_id %s", attempt)
		case !scheduled:
			return trustErr(ReasonRecordsProtocolMismatch, "unscheduled %s", attempt)
		case seen:
			return trustErr(ReasonRecordsProtocolMismatch, "duplicate %s", attempt)
		}
		recorded[attempt] = true
	}
	for _, attempt := range order {
		if !recorded[attempt] {
			return trustErr(ReasonRecordsProtocolMismatch, "missing %s", attempt)
		}
	}
	return nil
}

// checkSignedCalibration decodes the attested calibration.json and ties it to
// the session before the 001 verdict reads it: the protocol's session, the
// trusted task set in every recorded phase, and the before calibration the
// protocol froze (records_protocol_mismatch); then each phase's status to its
// task rows, passed exactly when every task proved both directions
// (outcome_derivation_mismatch); then the rest of the 001 contract.
func checkSignedCalibration(data []byte, p Protocol, tasks []string) (*Calibration, error) {
	var calibration Calibration
	if err := strictDecode(data, &calibration); err != nil {
		return nil, withPath(err, CalibrationFile)
	}
	if calibration.SessionID != p.SessionID {
		return nil, trustErr(ReasonRecordsProtocolMismatch, "%s session_id", CalibrationFile)
	}
	type namedPhase struct {
		name  string
		phase CalibrationPhase
	}
	phases := []namedPhase{{"before", calibration.Before}}
	if calibration.After != nil {
		phases = append(phases, namedPhase{"after", *calibration.After})
	}
	for _, phase := range phases {
		ids := []string{}
		for _, task := range phase.phase.Tasks {
			ids = append(ids, task.TaskID)
		}
		sort.Strings(ids)
		if !slices.Equal(ids, tasks) {
			return nil, trustErr(ReasonRecordsProtocolMismatch, "%s %s.tasks", CalibrationFile, phase.name)
		}
	}
	for _, phase := range phases {
		status := phase.phase.Status
		if derived := phaseStatus(phase.phase.Tasks); (status == CalibrationPassed || status == CalibrationFailed) && status != derived {
			return nil, trustErr(ReasonOutcomeDerivationMismatch, "%s %s.status %s, its tasks give %s", CalibrationFile, phase.name, status, derived)
		}
	}
	if err := validateCalibration(calibration); err != nil {
		return nil, withPath(err, CalibrationFile)
	}
	if calibration.Before.Status != p.Calibration.Status || !slices.Equal(calibration.Before.Tasks, p.Calibration.Tasks) {
		return nil, trustErr(ReasonRecordsProtocolMismatch, "%s before differs from the calibration in %s", CalibrationFile, ProtocolFile)
	}
	if calibration.Before.Status != CalibrationPassed {
		return nil, trustErr(ReasonRecordsProtocolMismatch, "%s before failed, yet the trials ran", CalibrationFile)
	}
	return &calibration, nil
}

// phaseStatus is the status the trusted runner gives a calibration phase:
// passed when it has tasks and every one accepted the clean artifact and
// did not accept the mutated one.
func phaseStatus(tasks []CalibrationTask) string {
	proven := len(tasks) > 0
	for _, task := range tasks {
		proven = proven && task.CleanAccepted && !task.MutatedAccepted
	}
	if proven {
		return CalibrationPassed
	}
	return CalibrationFailed
}

// derived is what the REQ-HR-08 table gives one trial.
type derived struct {
	outcome, signal string
	oracle          OracleObservation
}

// deriveRecords re-derives every record from its attested observation and
// oracle result bytes and refuses any record the table does not give exactly
// (outcome_derivation_mismatch). A record must reference only attested oracle
// results, and every attested result must belong to a record.
func deriveRecords(records []Record, results map[string][]byte, assertions map[string][]string) error {
	used := make(map[string]bool, len(results))
	for _, record := range records {
		attempt := Attempt{TaskID: record.TaskID, Arm: record.Arm, Trial: record.Trial}
		if record.StageReached == "" || record.AgentTermination == nil {
			return trustErr(ReasonOutcomeDerivationMismatch, "%s carries no black-box observation", attempt)
		}
		var data []byte
		if digest := record.OracleResultSHA256; digest != nil {
			var attested bool
			if data, attested = results[*digest]; !attested {
				return trustErr(ReasonOutcomeDerivationMismatch, "%s names oracle result %s that is not attested", attempt, *digest)
			}
			used[*digest] = true
		}
		got, matched := deriveTrial(record, data, assertions[record.TaskID])
		switch {
		case !matched:
			return trustErr(ReasonOutcomeDerivationMismatch, "%s at stage %s with signal %s matches no row", attempt, record.StageReached, record.Signal)
		case got.outcome != record.Outcome || got.signal != record.Signal:
			return trustErr(ReasonOutcomeDerivationMismatch, "%s records %s/%s, the table gives %s/%s",
				attempt, record.Outcome, record.Signal, got.outcome, got.signal)
		case got.oracle != *record.Oracle:
			return trustErr(ReasonOutcomeDerivationMismatch, "%s records oracle %+v, the table gives %+v", attempt, *record.Oracle, got.oracle)
		}
	}
	unused := []string{}
	for digest := range results {
		if !used[digest] {
			unused = append(unused, digest)
		}
	}
	if len(unused) > 0 {
		sort.Strings(unused)
		return trustErr(ReasonOutcomeDerivationMismatch, "oracle result %s belongs to no trial", unused[0])
	}
	return nil
}

// deriveTrial applies the REQ-HR-08 table to one black-box record and its
// oracle result bytes (nil when it has none). The first matching row fixes
// the outcome and signal; matched is false when no row does. The oracle
// observation is filled apart from the rows: build_failed when the trial
// stopped at the build, and ran with the assertion counts only when the
// harness compared exactly the assertions of main's task definition.
func deriveTrial(r Record, data []byte, want []string) (derived, bool) {
	term := *r.AgentTermination
	var result OracleResult
	valid := false
	if r.StageReached == StageOracle && data != nil {
		decoded, err := DecodeOracleResult(data)
		result, valid = decoded, err == nil && decoded.TaskID == r.TaskID
	}
	got := derived{oracle: OracleObservation{BuildFailed: r.StageReached == StageBuild}}
	compared := valid && !result.TimedOut && result.OutputCheck == OutputCheckOK && sameAssertionIDs(result.Assertions, want)
	if compared {
		got.oracle.Ran = true
		for _, assertion := range result.Assertions {
			if assertion.Passed {
				got.oracle.ExpectedPassed++
			} else {
				got.oracle.ExpectedFailed++
			}
		}
	}
	// Rows 1-5 judge the record's own observation, rows 6-10 the oracle result.
	// A trial that never left setup matches row 1 or no row, as in the trusted
	// runner's table: rows 2-4 judge the agent step it never reached.
	switch {
	case r.StageReached == StageSetup && slices.Contains([]string{"workspace_setup_failed", "mutation_failed", "warmup_failed"}, r.Signal):
		got.signal = r.Signal
	case r.StageReached == StageSetup:
		return derived{}, false
	case !term.Launched:
		got.signal = "agent_launch_failed"
	case term.TimedOut:
		got.signal = "agent_timeout"
	case (term.ExitCode != nil && *term.ExitCode != 0) || term.OSSignal != nil:
		got.signal = "agent_exit_nonzero"
	case r.Signal == "observation_failed":
		got.signal = r.Signal
	case r.Signal == "scope_violation" && r.StageReached == StageAgent:
		got.signal = r.Signal
	case r.StageReached == StageBuild:
		got.signal = SignalArtifactBuildFailed
	case r.StageReached != StageOracle:
		// Rows 6-10 need the oracle stage: a trial stopped elsewhere for no
		// reason above matches no row.
		return derived{}, false
	case !valid || (!result.TimedOut && result.OutputCheck == OutputCheckOK && !compared):
		got.signal = SignalOracleHarnessError
	case result.TimedOut:
		got.signal = SignalArtifactTimeout
	case result.OutputCheck == OutputCheckLinkRejected:
		got.signal = SignalOutputLinkRejected
	case result.OutputCheck == OutputCheckTooLarge:
		got.signal = SignalOutputTooLarge
	case !compared:
		// Unreachable while DecodeOracleResult ties not_checked to a timeout;
		// kept so an uncompared result can never reach accepted.
		return derived{}, false
	case got.oracle.ExpectedFailed > 0:
		got.signal = SignalExpectationMismatch
	default:
		got.signal = "accepted"
	}
	got.outcome, _ = signalOutcome(got.signal)
	return got, true
}

// sameAssertionIDs reports whether the compared assertions are exactly the
// non-empty id set main's task definition pins.
func sameAssertionIDs(assertions []OracleAssertion, want []string) bool {
	if len(want) == 0 || len(assertions) != len(want) {
		return false
	}
	for _, assertion := range assertions {
		if !slices.Contains(want, assertion.ID) {
			return false
		}
	}
	return true
}
