package cli

import (
	"context"
	"errors"
	"os"
	"time"
)

// ompReviewMaxAttempts bounds fresh-session retries for transient provider
// errors (overload, rate limit, gateway). OMP's own auto-retry stays off so a
// retry can never fall back to a different model than the pinned one.
const ompReviewMaxAttempts = 3

// ompReviewSession is one review request; every run starts a private OMP
// process with its own runtime directory and hardening overlay.
type ompReviewSession struct {
	projectDir string
	timeout    time.Duration
	model      string
	thinking   string
	prompt     string
	tools      []string
	toolsCSV   string
	maxTime    string
	unsetEnv   []string // inherited variables the OMP process starts without
}

// run executes one attempt. cleanupErr is reported separately because a
// successful reply with a failed runtime cleanup must still fail closed.
func (s ompReviewSession) run(ctx context.Context) (output string, executionErr, cleanupErr error) {
	processConfig, runtimeBase, err := prepareOMPReviewProcessConfig(s.projectDir, s.timeout, s.unsetEnv)
	if err != nil {
		return "", err, nil
	}
	overlayPath, err := writeOMPReviewHardeningOverlay(runtimeBase)
	if err != nil {
		return "", err, os.Remove(runtimeBase)
	}
	cleanupRuntime := func() error {
		return errors.Join(os.Remove(overlayPath), os.Remove(runtimeBase))
	}
	options := pipelineOMPProcessOptions{ExtraArgs: []string{
		"--no-skills", "--no-lsp", "--config", overlayPath,
		"--tools", s.toolsCSV, "--approval-mode", "yolo", "--max-time", s.maxTime,
	}}
	process, err := startPipelineOMPProcessWithOptions(ctx, processConfig, options)
	if err != nil {
		return "", err, cleanupRuntime()
	}
	output, executionErr = executeOMPReviewRPC(ctx, process, s.model, s.thinking, s.prompt, s.tools)
	return output, executionErr, errors.Join(process.Close(), cleanupRuntime())
}

// ompReviewShouldRetry allows another attempt only for a transient provider
// error on a clean session, while the request budget is still alive.
func ompReviewShouldRetry(ctx context.Context, executionErr, cleanupErr error, attempt int) bool {
	if attempt >= ompReviewMaxAttempts || cleanupErr != nil || ctx.Err() != nil {
		return false
	}
	var turnErr *pipelineOMPTurnError
	return errors.As(executionErr, &turnErr) && turnErr.Transient()
}

// ompReviewBackoff waits attempt*ompReviewBackoffUnit unless the budget ends first.
func ompReviewBackoff(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * ompReviewBackoffUnit)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var ompReviewBackoffUnit = 5 * time.Second
