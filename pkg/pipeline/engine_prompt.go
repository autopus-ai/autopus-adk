package pipeline

import (
	"fmt"
	"strings"
)

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

func validatePhaseResponse(phaseID PhaseID, resp *PhaseResponse) error {
	if resp == nil {
		return fmt.Errorf("phase %s: backend returned nil response", phaseID)
	}
	if resp.TimedOut {
		return fmt.Errorf("phase %s: backend timed out", phaseID)
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("phase %s: backend exited with code %d", phaseID, resp.ExitCode)
	}
	if strings.TrimSpace(resp.Output) == "" {
		return fmt.Errorf("phase %s: backend returned empty output", phaseID)
	}
	return nil
}
