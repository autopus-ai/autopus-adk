package harneval

import (
	"regexp"
	"slices"
)

// OracleResultSchemaV1 identifies the document the trusted oracle harness
// writes for one trial (SPEC-HARNEVAL-003 Wire Contracts). Every result
// carries it, as the trusted runner requires.
const OracleResultSchemaV1 = "harness_oracle_result.v1"

// Output checks of an oracle result: how opening the pinned output paths went.
const (
	OutputCheckOK           = "ok"
	OutputCheckLinkRejected = "link_rejected"
	OutputCheckTooLarge     = "too_large"
	OutputCheckNotChecked   = "not_checked"
)

// OracleResult is the harness_oracle_result.v1 document. ArtifactExit and
// TimedOut are the runner's observation of the artifact, copied by the
// harness; a timed-out artifact is not_checked, and only an ok output check
// carries assertions. ArtifactExit is null or an exit status in 0..255.
type OracleResult struct {
	SchemaVersion string            `json:"schema_version"`
	TaskID        string            `json:"task_id"`
	OutputCheck   string            `json:"output_check"`
	Assertions    []OracleAssertion `json:"assertions"`
	ArtifactExit  *int              `json:"artifact_exit"`
	TimedOut      bool              `json:"timed_out"`
}

// OracleAssertion is one pinned expectation the harness compared.
type OracleAssertion struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
}

// DecodeOracleResult strictly decodes and validates one oracle result alone,
// as the trusted runner's golden_blackbox.decode_result does: every key
// present once, null only for artifact_exit. Whether its assertions are the
// task's is decided against main's task definition when the trial is
// re-derived.
func DecodeOracleResult(data []byte) (OracleResult, error) {
	var result OracleResult
	if err := strictDecode(data, &result); err != nil {
		return result, err
	}
	if err := checkOracleResultShape(data); err != nil {
		return result, err
	}
	switch {
	case result.SchemaVersion != OracleResultSchemaV1:
		return result, invalidf(DetailFieldInvalid, "oracle result schema_version %q is not %s", result.SchemaVersion, OracleResultSchemaV1)
	case result.ArtifactExit != nil && (*result.ArtifactExit < 0 || *result.ArtifactExit > 255):
		return result, invalidf(DetailFieldInvalid, "oracle result artifact_exit %d is outside 0..255", *result.ArtifactExit)
	case !taskIDPattern.MatchString(result.TaskID):
		return result, invalidf(DetailFieldInvalid, "oracle result task_id %q is not a GT id", result.TaskID)
	case !slices.Contains([]string{OutputCheckOK, OutputCheckLinkRejected, OutputCheckTooLarge, OutputCheckNotChecked}, result.OutputCheck):
		return result, invalidf(DetailFieldInvalid, "oracle result output_check %q is unknown", result.OutputCheck)
	case result.TimedOut != (result.OutputCheck == OutputCheckNotChecked):
		return result, invalidf(DetailFieldInvalid, "oracle result output_check is not_checked exactly when the artifact timed out")
	case result.OutputCheck != OutputCheckOK && len(result.Assertions) > 0:
		return result, invalidf(DetailFieldInvalid, "oracle result with output_check %s compared assertions", result.OutputCheck)
	}
	seen := make(map[string]bool, len(result.Assertions))
	for _, assertion := range result.Assertions {
		if blank(assertion.ID) || seen[assertion.ID] {
			return result, invalidf(DetailFieldInvalid, "oracle result assertion id %q is blank or repeated", assertion.ID)
		}
		seen[assertion.ID] = true
	}
	return result, nil
}

// stages lists the black-box stages a record may name.
var stages = []string{StageSetup, StageAgent, StageBuild, StageRun, StageOracle}

// osSignalPattern is a POSIX signal name such as SIGKILL.
var osSignalPattern = regexp.MustCompile(`^SIG[A-Z0-9]+$`)

// blackBoxOutcomes extends the REQ-HE-08 signal table with the signals only a
// black-box trial records (REQ-HR-08). Every one is a fail: each says the
// agent step completed and the artifact or its output did not hold up.
var blackBoxOutcomes = map[string]string{
	SignalArtifactBuildFailed: OutcomeFail,
	SignalOracleHarnessError:  OutcomeFail,
	SignalArtifactTimeout:     OutcomeFail,
	SignalOutputLinkRejected:  OutcomeFail,
	SignalOutputTooLarge:      OutcomeFail,
	SignalExpectationMismatch: OutcomeFail,
}

// signalOutcome is the outcome a known signal fixes.
func signalOutcome(signal string) (string, bool) {
	if outcome, known := signalOutcomes[signal]; known {
		return outcome, true
	}
	outcome, known := blackBoxOutcomes[signal]
	return outcome, known
}

// validateBlackBox checks the black-box observation of one record alone. A
// SPEC-HARNEVAL-001 record carries none of it and no black-box signal. A
// black-box record names a known stage and its agent termination, and an
// oracle result digest only at the oracle stage. Whether the observation
// supports the record's signal is the signer's re-derivation, not decoding.
func validateBlackBox(r Record) error {
	if r.StageReached == "" && r.AgentTermination == nil && r.OracleResultSHA256 == nil {
		if _, blackBox := blackBoxOutcomes[r.Signal]; blackBox {
			return invalidf(DetailFieldInvalid, "signal %s needs the black-box stage_reached and agent_termination", r.Signal)
		}
		return nil
	}
	if !slices.Contains(stages, r.StageReached) || r.AgentTermination == nil {
		return invalidf(DetailFieldInvalid, "black-box record needs a known stage_reached and an agent_termination, not %q", r.StageReached)
	}
	if digest := r.OracleResultSHA256; digest != nil && (r.StageReached != StageOracle || !sha256Pattern.MatchString(*digest)) {
		return invalidf(DetailFieldInvalid, "oracle_result_sha256 needs stage_reached %s and a 64-hex digest", StageOracle)
	}
	return validateTermination(*r.AgentTermination)
}

// validateTermination enforces the normalized agent termination: nothing but
// launched=false for an agent that never started, and exactly one of an exit
// code in 0..255 or a signal name for one that did. A negative Python return
// code is a signal ending the runner writes as os_signal.
func validateTermination(t AgentTermination) error {
	exited, signaled := t.ExitCode != nil, t.OSSignal != nil
	switch {
	case !t.Launched && (exited || signaled || t.TimedOut):
		return invalidf(DetailFieldInvalid, "agent_termination of an agent that never launched holds only nulls and false")
	case t.Launched && exited == signaled:
		return invalidf(DetailFieldInvalid, "agent_termination of a launched agent needs exactly one of exit_code and os_signal")
	case exited && (*t.ExitCode < 0 || *t.ExitCode > 255):
		return invalidf(DetailFieldInvalid, "agent_termination exit_code %d is outside 0..255; a signal ending is os_signal", *t.ExitCode)
	case signaled && !osSignalPattern.MatchString(*t.OSSignal):
		return invalidf(DetailFieldInvalid, "agent_termination os_signal %q is not a signal name", *t.OSSignal)
	}
	return nil
}
