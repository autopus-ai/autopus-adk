package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// specReviewRoundTiming records how long one provider round of the review loop
// took, and how that time split across reviewers and the judge. It is the
// evidence for where SPEC review time goes; nothing gates on it.
type specReviewRoundTiming struct {
	Revision   int                        `json:"revision"`
	StartedAt  string                     `json:"started_at"`
	DurationMS int64                      `json:"duration_ms"`
	Providers  []specReviewProviderTiming `json:"providers,omitempty"`
}

type specReviewProviderTiming struct {
	Provider   string `json:"provider"`
	Role       string `json:"role,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
}

func newSpecReviewRoundTiming(revision int, started time.Time, elapsed time.Duration, result *orchestra.OrchestraResult) specReviewRoundTiming {
	round := specReviewRoundTiming{
		Revision:   revision,
		StartedAt:  started.UTC().Format(time.RFC3339Nano),
		DurationMS: elapsed.Milliseconds(),
	}
	if result == nil {
		return round
	}
	for _, response := range result.Responses {
		round.Providers = append(round.Providers, specReviewProviderTiming{
			Provider:   response.Provider,
			Role:       response.Role,
			DurationMS: response.Duration.Milliseconds(),
			TimedOut:   response.TimedOut,
		})
	}
	return round
}

// printSpecReviewRoundTiming writes one operator line per round, e.g.
// "SPEC 리뷰 라운드 0 소요: 6m12s (claude 4m1s, codex 5m40s, judge:claude 32s)".
func printSpecReviewRoundTiming(w io.Writer, round specReviewRoundTiming) {
	parts := make([]string, 0, len(round.Providers))
	for _, provider := range round.Providers {
		label := provider.Provider
		if provider.Role == "judge" {
			label = "judge:" + label
		}
		entry := fmt.Sprintf("%s %s", label, formatSpecReviewDuration(provider.DurationMS))
		if provider.TimedOut {
			entry += " timeout"
		}
		parts = append(parts, entry)
	}
	line := fmt.Sprintf("SPEC 리뷰 라운드 %d 소요: %s", round.Revision, formatSpecReviewDuration(round.DurationMS))
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}
	fmt.Fprintln(w, line)
}

func formatSpecReviewDuration(ms int64) string {
	return (time.Duration(ms) * time.Millisecond).Round(time.Second).String()
}

// applySpecReviewTiming projects the run's wall-clock evidence onto the receipt.
func applySpecReviewTiming(receipt *specReviewPromotionReceipt, evidence specReviewRuntimeEvidence, finishedAt time.Time) {
	if receipt == nil || evidence.StartedAt.IsZero() {
		return
	}
	receipt.StartedAt = evidence.StartedAt.UTC().Format(time.RFC3339Nano)
	receipt.DurationMS = finishedAt.Sub(evidence.StartedAt).Milliseconds()
	receipt.Rounds = append([]specReviewRoundTiming(nil), evidence.Rounds...)
}
