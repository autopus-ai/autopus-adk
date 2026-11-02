package pipeline

import (
	"fmt"
	"strings"
)

// @AX:NOTE [AUTO] @AX:SPEC: SPEC-PIPELINE-RESUME-001: one admission contract for the canonical engine and both exported runners.
// admitPhaseResponse is the response validity contract every runner applies
// before it reads a verdict (REQ-PRR-RESP-01). A Go error, a nil response, a
// timeout, a non-zero exit, or empty output fails the dispatch, so a VERDICT
// line inside a failed response can never turn it into a pass, and a runner
// that refuses the dispatch never reaches the phases that depend on it.
func admitPhaseResponse(phaseID PhaseID, resp *PhaseResponse, execErr error) error {
	if execErr != nil {
		return fmt.Errorf("phase %s: %w", phaseID, execErr)
	}
	return validatePhaseResponse(phaseID, resp)
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
