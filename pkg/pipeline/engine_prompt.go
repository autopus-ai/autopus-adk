package pipeline

// buildPhasePrompt assembles a phase prompt. A pending fix dispatch carries
// its failure feedback before the gate contract, so the verdict instruction
// stays the final line.
func (e *SubprocessEngine) buildPhasePrompt(
	phase Phase, previous map[PhaseID]string, legacyPrevious string, feedback *PhaseFailure,
) (string, error) {
	prompt := buildPrompt(e.cfg.SpecID, phase.ID, legacyPrevious)
	if e.promptBuilder != nil {
		built, err := e.promptBuilder.BuildPrompt(phase.ID, PhaseContext{
			PreviousResults: previous, FrozenRequiredDocuments: true, Route: e.cfg.Route,
		})
		if err != nil {
			return "", err
		}
		prompt = built
	}
	return appendGateContract(appendRemediationFeedback(prompt, feedback), phase.Gate), nil
}

func (e *SubprocessEngine) buildOMPActivePhasePrompt(phase Phase, feedback *PhaseFailure) (string, error) {
	prompt := buildPrompt(e.cfg.SpecID, phase.ID, "")
	if e.promptBuilder != nil {
		built, err := e.promptBuilder.BuildPrompt(phase.ID, PhaseContext{
			FrozenRequiredDocuments: true, Route: e.cfg.Route,
		})
		if err != nil {
			return "", err
		}
		prompt = built
	}
	return appendGateContract(appendRemediationFeedback(prompt, feedback), phase.Gate), nil
}

// Gate report requests (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-09): the lines
// a failed gate reports are the evidence its fix dispatch receives.
const (
	validationReportContract = "If a check fails, report each failure before the verdict line as three lines: " +
		"FAILED_COMMAND: <exact command>, EXPECTED: <expected result>, ACTUAL: <actual result>."
	reviewReportContract = "Report each blocking finding before the verdict line as one line: " +
		"FINDING <ID>: <evidence with file:line> | FIX: <condition that resolves it>."
)

func appendGateContract(prompt string, gate GateType) string {
	switch gate {
	case GateValidation:
		return prompt + "\n\n" + validationReportContract +
			"\nReturn exactly one final line: VERDICT: PASS or VERDICT: FAIL"
	case GateReview:
		return prompt + "\n\n" + reviewReportContract +
			"\nReturn exactly one final line: VERDICT: APPROVE or VERDICT: REQUEST_CHANGES"
	default:
		return prompt
	}
}
