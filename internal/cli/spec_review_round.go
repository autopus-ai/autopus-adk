package cli

import (
	"context"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// runSpecReviewRound runs one round's model calls: the parallel reviewers and
// the judge. Under always the judge runs inside the structured runner as
// before; under a conditional mode the reviewers run first and the judge only
// if the round needs it (SPEC-REVIEWREUSE-001 REQ-RR-JDG-02 to REQ-RR-JDG-04).
// It returns the round result, the summary of a skipped judge, and the round
// timing it printed.
func runSpecReviewRound(
	p specReviewLoopParams, orchCfg orchestra.OrchestraConfig, doc *spec.SpecDocument, revision int, priorFindings []spec.ReviewFinding,
) (*orchestra.OrchestraResult, *spec.JudgeSummary, specReviewRoundTiming, error) {
	judgeCfg := orchCfg
	conditional := specReviewJudgeConditional(p.judgeMode) && !orchCfg.NoJudge
	if conditional {
		orchCfg.NoJudge = true
	}
	// Bound the parallel reviewer fan-out and the sequential judge together.
	reviewCtx, cancel := context.WithCancel(p.ctx)
	if watchdog := specReviewWatchdogForConfig(judgeCfg); watchdog > 0 {
		reviewCtx, cancel = context.WithTimeout(p.ctx, time.Duration(watchdog)*time.Second)
	}
	// The round carries its number for the call ledger and stops at the
	// session's wall-clock deadline (REQ-RP-BUD-05).
	reviewCtx, endRound := specReviewRoundContext(reviewCtx, revision)
	roundStarted := time.Now()
	result, err := specReviewRunOrchestra(reviewCtx, orchCfg)
	var skipped *spec.JudgeSummary
	if err == nil && conditional {
		skipped = runConditionalSpecReviewJudge(reviewCtx, judgeCfg, result, p, doc, revision, priorFindings, os.Stderr)
	}
	endRound()
	cancel()
	round := newSpecReviewRoundTiming(revision, roundStarted, time.Since(roundStarted), result)
	printSpecReviewRoundTiming(os.Stderr, round)
	return result, skipped, round, err
}
