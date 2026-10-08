package harneval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Live-lane wire-contract schema identifiers (REQ-HE-07 ~ REQ-HE-10). The
// contracts do not require schema_version on these documents; one that
// carries it must carry exactly its own identifier.
const (
	ProtocolSchemaV1    = "harness_golden_live_protocol.v1"
	RecordSchemaV1      = "harness_golden_live_record.v1"
	CalibrationSchemaV1 = "harness_golden_calibration.v1"
)

// Files of a live session directory, all written by the trusted runner.
// Only the protocol is required: a session refused at calibration has no
// records file, and one without calibration.json is reported vacuous.
const (
	ProtocolFile    = "protocol.json"
	CalibrationFile = "calibration.json"
	RecordsFile     = "records.jsonl"
)

// Arms, trial outcomes, and calibration statuses. CalibrationMissing is not a
// calibration.json value: the advisory report uses it when the evidence is
// absent.
const (
	ArmBaseline  = "baseline"
	ArmCandidate = "candidate"

	OutcomePass  = "pass"
	OutcomeFail  = "fail"
	OutcomeError = "error"

	CalibrationPassed  = "passed"
	CalibrationFailed  = "failed"
	CalibrationMissing = "missing"
)

// Session is one live session directory as decoded documents. Calibration is
// nil when calibration.json is absent; Records is empty when no record exists.
type Session struct {
	Protocol    Protocol
	Calibration *Calibration
	Records     []Record
}

// Stages a black-box trial reaches in order (SPEC-HARNEVAL-003 REQ-HR-08):
// workspace setup, the agent, the artifact build, the artifact run, and the
// trusted oracle harness.
const (
	StageSetup  = "setup"
	StageAgent  = "agent"
	StageBuild  = "build"
	StageRun    = "run"
	StageOracle = "oracle"
)

// Signals only a black-box trial records (REQ-HR-08); every one is a fail.
const (
	SignalArtifactBuildFailed = "artifact_build_failed"
	SignalOracleHarnessError  = "oracle_harness_error"
	SignalArtifactTimeout     = "artifact_timeout"
	SignalOutputLinkRejected  = "output_link_rejected"
	SignalOutputTooLarge      = "output_too_large"
	SignalExpectationMismatch = "expectation_mismatch"
)

// Protocol is the harness_golden_live_protocol.v1 document frozen before the
// first trial. Policy is the manifest live policy; corpus_digests and
// prompt_layers are carried for the signed lane and never read here. The last
// five fields are written only by the signed lane, which needs all of them
// (SPEC-HARNEVAL-003 Wire Contracts); a maintainer-host session has none.
type Protocol struct {
	SchemaVersion          string            `json:"schema_version,omitempty"`
	SessionID              string            `json:"session_id"`
	StartedAt              string            `json:"started_at"`
	WorkspaceRevision      string            `json:"workspace_revision"`
	BaselineRef            string            `json:"baseline_ref"`
	BaselineSurfaceDigest  string            `json:"baseline_surface_digest"`
	CandidateSurfaceDigest string            `json:"candidate_surface_digest"`
	AgentSetDigest         string            `json:"agent_set_digest"`
	CorpusDigests          []json.RawMessage `json:"corpus_digests"`
	RunnerSHA256           string            `json:"runner_sha256"`
	GraderProfileSHA256    string            `json:"grader_profile_sha256"`
	Calibration            CalibrationPhase  `json:"calibration"`
	Policy                 LivePolicy        `json:"policy"`
	Pins                   Pins              `json:"pins"`
	CLIVersion             string            `json:"cli_version"`
	Model                  string            `json:"model"`
	Order                  []Attempt         `json:"order"`
	PromptLayers           []json.RawMessage `json:"prompt_layers"`
	RunID                  int64             `json:"run_id,omitempty"`
	RunAttempt             int               `json:"run_attempt,omitempty"`
	BindingDigest          string            `json:"binding_digest,omitempty"`
	BaselineCommit         string            `json:"baseline_commit,omitempty"`
	RunnerTreeDigest       string            `json:"runner_tree_digest,omitempty"`
}

// Attempt is one scheduled trial of the protocol order.
type Attempt struct {
	TaskID string `json:"task_id"`
	Arm    string `json:"arm"`
	Trial  int    `json:"trial"`
}

func (a Attempt) String() string { return fmt.Sprintf("%s/%s/%d", a.TaskID, a.Arm, a.Trial) }

// Calibration is the harness_golden_calibration.v1 document (calibration.json).
// After is recorded only once a trial ran, so it is nil for a refused session.
type Calibration struct {
	SchemaVersion string            `json:"schema_version,omitempty"`
	SessionID     string            `json:"session_id"`
	Before        CalibrationPhase  `json:"before"`
	After         *CalibrationPhase `json:"after,omitempty"`
}

// CalibrationPhase is one two-direction oracle calibration of every task.
type CalibrationPhase struct {
	Status string            `json:"status"`
	Tasks  []CalibrationTask `json:"tasks"`
}

// CalibrationTask is one task's calibration: its oracle must accept the
// clean workspace and must not accept the mutated one.
type CalibrationTask struct {
	TaskID          string `json:"task_id"`
	CleanAccepted   bool   `json:"clean_accepted"`
	MutatedAccepted bool   `json:"mutated_accepted"`
}

// Record is one harness_golden_live_record.v1 line of records.jsonl. Oracle
// is never nil in a decoded record. The last three fields are the black-box
// observation the trusted runner records for a signed-lane trial
// (SPEC-HARNEVAL-003 Wire Contracts); a SPEC-HARNEVAL-001 record has none,
// and a black-box record has StageReached and AgentTermination. The oracle
// result digest is set only for a trial that reached the oracle and got a
// result file from it.
type Record struct {
	SchemaVersion      string             `json:"schema_version,omitempty"`
	SessionID          string             `json:"session_id"`
	TaskID             string             `json:"task_id"`
	Arm                string             `json:"arm"`
	Trial              int                `json:"trial"`
	Outcome            string             `json:"outcome"`
	Signal             string             `json:"signal"`
	Oracle             *OracleObservation `json:"oracle"`
	DurationS          float64            `json:"duration_s"`
	OracleResultSHA256 *string            `json:"oracle_result_sha256,omitempty"`
	StageReached       string             `json:"stage_reached,omitempty"`
	AgentTermination   *AgentTermination  `json:"agent_termination,omitempty"`
}

// AgentTermination is how the agent process of a black-box trial ended, as
// the trusted runner normalizes it: an agent that never started has only
// nulls and false; one that ended by a signal has a null exit code and the
// signal name. TimedOut says the runner ended it at the trial timeout,
// whatever the ending looks like.
type AgentTermination struct {
	Launched bool    `json:"launched"`
	ExitCode *int    `json:"exit_code"`
	OSSignal *string `json:"os_signal"`
	TimedOut bool    `json:"timed_out"`
}

// OracleObservation is what the trusted parser saw in the grader output. Ran
// is true only when one of the expected tests emitted a test-level event, so
// a build failure alone leaves it false.
type OracleObservation struct {
	Ran            bool `json:"ran"`
	BuildFailed    bool `json:"build_failed"`
	ExpectedPassed int  `json:"expected_passed"`
	ExpectedFailed int  `json:"expected_failed"`
}

// LoadSession strictly decodes the documents of one live session directory.
// Any defect yields an *InvalidError naming the file; an absent calibration
// or records file is not a defect.
func LoadSession(dir string) (*Session, error) {
	data, err := readCapped(filepath.Join(dir, ProtocolFile))
	if err != nil {
		return nil, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, ProtocolFile)
	}
	session := &Session{Records: []Record{}}
	if session.Protocol, err = DecodeProtocol(data); err != nil {
		return nil, withPath(err, ProtocolFile)
	}
	data, present, err := readOptional(dir, CalibrationFile)
	if err != nil {
		return nil, err
	}
	if present {
		calibration, err := DecodeCalibration(data)
		if err != nil {
			return nil, withPath(err, CalibrationFile)
		}
		session.Calibration = &calibration
	}
	if data, _, err = readOptional(dir, RecordsFile); err != nil {
		return nil, err
	}
	if session.Records, err = DecodeRecords(data); err != nil {
		return nil, err
	}
	return session, nil
}

// readOptional reads a session file that may be absent. present is false only
// when the file does not exist, so an empty file is still decoded.
func readOptional(dir, name string) (data []byte, present bool, err error) {
	data, err = readCapped(filepath.Join(dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, name)
	}
	return data, true, nil
}

// DecodeProtocol strictly decodes and validates a
// harness_golden_live_protocol.v1 document. The returned policy carries the
// protocol's input identifiers even when the document left them out of it.
func DecodeProtocol(data []byte) (Protocol, error) {
	var protocol Protocol
	if err := strictDecode(data, &protocol); err != nil {
		return protocol, err
	}
	if err := validateProtocol(&protocol); err != nil {
		return protocol, err
	}
	return protocol, validateSignedLane(protocol)
}

// DecodeCalibration strictly decodes and validates a calibration.json document.
func DecodeCalibration(data []byte) (Calibration, error) {
	var calibration Calibration
	if err := strictDecode(data, &calibration); err != nil {
		return calibration, err
	}
	return calibration, validateCalibration(calibration)
}

// DecodeRecords strictly decodes records.jsonl: one record per line, every
// line terminated by a newline except possibly the last. Empty data holds no
// record; an empty line is malformed.
func DecodeRecords(data []byte) ([]Record, error) {
	records := []Record{}
	if len(data) == 0 {
		return records, nil
	}
	for index, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		var record Record
		err := strictDecode([]byte(line), &record)
		if err == nil {
			err = validateRecord(record)
		}
		if err == nil {
			err = validateBlackBox(record)
		}
		if err != nil {
			return nil, withPath(err, fmt.Sprintf("%s:%d", RecordsFile, index+1))
		}
		records = append(records, record)
	}
	return records, nil
}
