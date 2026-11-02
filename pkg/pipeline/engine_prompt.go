package pipeline

// buildPhasePrompt assembles a phase prompt. On the frozen SPEC path a fix
// dispatch's feedback is a manifest layer; without a SPEC directory it is
// appended. The gate contract stays the final instruction either way.
func (e *SubprocessEngine) buildPhasePrompt(phase Phase, previous map[PhaseID]string, feedback *PhaseFailure) (string, error) {
	if e.promptBuilder == nil {
		prompt := appendRemediationFeedback(buildPrompt(e.cfg.SpecID, phase.ID, previous), feedback)
		return appendGateContract(prompt, phase.Gate), nil
	}
	built, err := e.promptBuilder.BuildPrompt(phase.ID, PhaseContext{
		PreviousResults: previous, FrozenRequiredDocuments: true, Route: e.cfg.Route, Feedback: feedback,
	})
	if err != nil {
		return "", err
	}
	return appendGateContract(built, phase.Gate), nil
}

// buildOMPActivePhasePrompt is the phase body OMP's managed sessions receive;
// earlier outputs travel in the completed history instead.
func (e *SubprocessEngine) buildOMPActivePhasePrompt(phase Phase, feedback *PhaseFailure) (string, error) {
	return e.buildPhasePrompt(phase, nil, feedback)
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
