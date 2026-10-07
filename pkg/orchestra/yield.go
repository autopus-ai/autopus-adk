package orchestra

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// YieldOutput is the per-round JSON structure printed when a run skips the
// judge (--no-judge) and hands the round history to the caller.
type YieldOutput struct {
	Strategy        string                    `json:"strategy"`
	Rounds          int                       `json:"rounds"`
	RoundHistory    []YieldRound              `json:"round_history"`
	Panes           map[string]string         `json:"panes"` // provider -> pane ID
	SessionID       string                    `json:"session_id"`
	FailedProviders []YieldFailure            `json:"failed_providers,omitempty"` // Providers dropped from round history
	Usage           []telemetry.UsageEnvelope `json:"usage,omitempty"`
	UsageAggregate  telemetry.UsageAggregate  `json:"usage_aggregate"`
	UsageCapability UsageCapability           `json:"usage_capability"`
}

// YieldFailure reports a provider that failed during execution so main-session
// orchestrators can distinguish "missing" from "silently dropped".
type YieldFailure struct {
	Provider        string                    `json:"provider"`
	Error           string                    `json:"error"`
	Usage           []telemetry.UsageEnvelope `json:"usage,omitempty"`
	UsageCapability UsageCapability           `json:"usage_capability"`
}

// YieldRound holds per-round provider responses.
type YieldRound struct {
	Round     int             `json:"round"`
	Responses []YieldResponse `json:"responses"`
}

// YieldResponse holds a single provider's output for one round.
type YieldResponse struct {
	Provider        string                    `json:"provider"`
	Output          string                    `json:"output"`
	DurationMs      int64                     `json:"duration_ms"`
	TimedOut        bool                      `json:"timed_out"`
	Usage           []telemetry.UsageEnvelope `json:"usage,omitempty"`
	UsageCapability UsageCapability           `json:"usage_capability"`
}

// WriteYieldOutput serializes YieldOutput as JSON to the writer.
func WriteYieldOutput(w io.Writer, output YieldOutput) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal yield output: %w", err)
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// BuildYieldOutputFromResult creates a YieldOutput from an OrchestraResult.
// Used by --no-judge to output structured JSON.
func BuildYieldOutputFromResult(result *OrchestraResult, sessionID string) YieldOutput {
	aggregateOrchestraUsage(result)
	var yieldRounds []YieldRound
	for i, responses := range result.RoundHistory {
		yr := YieldRound{Round: i + 1}
		for _, r := range responses {
			yr.Responses = append(yr.Responses, YieldResponse{
				Provider:   r.Provider,
				Output:     r.Output,
				DurationMs: r.Duration.Milliseconds(),
				TimedOut:   r.TimedOut,
				Usage:      r.Usage, UsageCapability: r.UsageCapability,
			})
		}
		yieldRounds = append(yieldRounds, yr)
	}

	failures := make([]YieldFailure, 0, len(result.FailedProviders))
	for _, fp := range result.FailedProviders {
		failures = append(failures, YieldFailure{
			Provider: fp.Name, Error: fp.Error, Usage: fp.Usage, UsageCapability: fp.UsageCapability,
		})
	}

	return YieldOutput{
		Strategy:        string(result.Strategy),
		Rounds:          len(result.RoundHistory),
		RoundHistory:    yieldRounds,
		SessionID:       sessionID,
		FailedProviders: failures,
		Usage:           result.Usage, UsageAggregate: result.UsageAggregate, UsageCapability: result.UsageCapability,
	}
}
