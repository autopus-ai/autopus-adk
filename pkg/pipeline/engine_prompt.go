package pipeline

func (e *SubprocessEngine) buildPhasePrompt(phase Phase, previous map[PhaseID]string, legacyPrevious string) (string, error) {
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
	return appendGateContract(prompt, phase.Gate), nil
}

func (e *SubprocessEngine) buildOMPActivePhasePrompt(phase Phase) (string, error) {
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
	return appendGateContract(prompt, phase.Gate), nil
}

func appendGateContract(prompt string, gate GateType) string {
	switch gate {
	case GateValidation:
		return prompt + "\n\nReturn exactly one final line: VERDICT: PASS or VERDICT: FAIL"
	case GateReview:
		return prompt + "\n\nReturn exactly one final line: VERDICT: APPROVE or VERDICT: REQUEST_CHANGES"
	default:
		return prompt
	}
}
