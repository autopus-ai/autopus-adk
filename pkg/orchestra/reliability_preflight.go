package orchestra

import (
	"fmt"
	"os"
	"time"
)

func ensureRunID(cfg *OrchestraConfig) string {
	if cfg.RunID != "" {
		return cfg.RunID
	}
	cfg.RunID = fmt.Sprintf("run-%d-%s", time.Now().UnixMilli(), randomHex())
	return cfg.RunID
}

func resolveWorkingDir(cfg OrchestraConfig) string {
	if cfg.WorkingDir != "" {
		return cfg.WorkingDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func roundCorrelation(runID, provider string, round, attempt int) CorrelationIDs {
	correlation := CorrelationIDs{RunID: runID, ProviderID: provider}
	if round > 0 {
		correlation.RoundID = fmt.Sprintf("round-%d", round)
	}
	if attempt > 0 {
		correlation.AttemptID = fmt.Sprintf("attempt-%d", attempt)
	}
	return correlation
}

// providerCapability describes the only launch path: a subprocess that reads
// the prompt from stdin and whose stdout is the collected result.
func providerCapability(cfg OrchestraConfig) ProviderCapabilityReceipt {
	return ProviderCapabilityReceipt{
		LaunchMode:                "subprocess",
		PromptTransportMode:       "stdin_pipe",
		CollectionModes:           []string{"subprocess_stdout"},
		SupportsPromptReceipt:     true,
		SupportsCollectionReceipt: true,
		SupportsCWDCheck:          resolveWorkingDir(cfg) != "",
	}
}

func preflightReceipt(runID string, cfg OrchestraConfig, provider ProviderConfig) ProviderPreflightReceipt {
	capability := providerCapability(cfg)
	return ProviderPreflightReceipt{
		SchemaVersion: reliabilitySchemaVersion,
		Timestamp:     time.Now().UTC(),
		Correlation:   roundCorrelation(runID, provider.Name, 0, 1),
		Provider:      provider.Name,
		Status:        "pass",
		LaunchMode:    capability.LaunchMode,
		TransportMode: capability.PromptTransportMode,
		RequestedCWD:  resolveWorkingDir(cfg),
		Capability:    capability,
	}
}

func promptReceipt(runID, provider, transportMode string, prompt string, round int, status, failureCodeCandidate string) PromptTransportReceipt {
	return PromptTransportReceipt{
		SchemaVersion: reliabilitySchemaVersion,
		Timestamp:     time.Now().UTC(),
		Correlation:   roundCorrelation(runID, provider, round, 1),
		Provider:      provider,
		TransportMode: transportMode,
		Status:        status,
		FailureCode:   normalizePromptFailureCode(status, transportMode, failureCodeCandidate),
		Prompt:        sanitizePromptArtifact(prompt),
	}
}

func collectionReceipt(runID, provider, mode, provenance, status, errMsg, output string, round int, partial bool) CollectionReceipt {
	return CollectionReceipt{
		SchemaVersion:  reliabilitySchemaVersion,
		Timestamp:      time.Now().UTC(),
		Correlation:    roundCorrelation(runID, provider, round, 1),
		Provider:       provider,
		CollectionMode: mode,
		Provenance:     provenance,
		Status:         status,
		Partial:        partial,
		Error:          errMsg,
		Output:         sanitizeArtifact(output),
	}
}

func timeoutEvent(runID, provider string, round int, nextStep string) ReliabilityEvent {
	return ReliabilityEvent{
		SchemaVersion: reliabilitySchemaVersion,
		Timestamp:     time.Now().UTC(),
		Correlation:   roundCorrelation(runID, provider, round, 1),
		Kind:          "hook_timeout",
		Severity:      "error",
		Message:       "hook-mode completion timed out before a done signal was collected",
		NextStep:      nextStep,
	}
}
