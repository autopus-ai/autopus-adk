package orchestra

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/terminal"
)

// recoveryHint is the concrete operator-facing recovery instruction surfaced
// when both backends are unavailable (REQ-013/S14).
const recoveryHint = "ensure a logged-in cmux/tmux CLI session and that the provider CLI is logged in"

// paneSplitGate serializes capacity-sensitive terminal split operations while
// still allowing queued callers to honor cancellation and deadlines.
var paneSplitGate = func() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}()

// paneInputGate serializes cmux input commits across surfaces. cmux accepts a
// paste and its trailing Enter through separate CLI calls; allowing another
// surface to paste between those calls can attach the Enter to the wrong input
// transaction and leave a provider pane blank or with a corrupted command.
// tmux and other terminals keep their existing parallel behavior.
var paneInputGate = func() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}()

func splitPaneSerialized(ctx context.Context, term terminal.Terminal, dir terminal.Direction) (terminal.PaneID, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-paneSplitGate:
	}
	defer func() { paneSplitGate <- struct{}{} }()
	return splitTrackedPane(ctx, term, dir)
}

// sendPaneInputAndEnterSerialized commits one pane input as an indivisible
// send/delay/Enter transaction on cmux. sendErr reports failure before the
// submit key; enterErr reports cancellation during the registration delay or
// failure to send Enter.
func sendPaneInputAndEnterSerialized(
	ctx context.Context,
	term terminal.Terminal,
	paneID terminal.PaneID,
	delay time.Duration,
	send func() error,
	enterRetryDelays ...time.Duration,
) (sendErr, enterErr error) {
	if err := ctx.Err(); err != nil {
		return err, nil
	}

	if strings.EqualFold(strings.TrimSpace(term.Name()), "cmux") {
		select {
		case <-ctx.Done():
			return ctx.Err(), nil
		case <-paneInputGate:
		}
		defer func() { paneInputGate <- struct{}{} }()
		// Cancellation and the gate token can become ready together. Re-check
		// after acquisition so a canceled queued transaction never reaches cmux.
		if err := ctx.Err(); err != nil {
			return err, nil
		}
	}

	if err := send(); err != nil {
		return err, nil
	}
	if err := waitPaneInputDelay(ctx, delay); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	enterErr = term.SendCommand(ctx, paneID, "\n")
	for _, retryDelay := range enterRetryDelays {
		if enterErr == nil {
			break
		}
		if err := waitPaneInputDelay(ctx, retryDelay); err != nil {
			return nil, err
		}
		enterErr = term.SendCommand(ctx, paneID, "\n")
	}
	return nil, enterErr
}

func waitPaneInputDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type paneSubmitEnterError struct {
	err error
}

func (e *paneSubmitEnterError) Error() string { return "submit Enter: " + e.err.Error() }
func (e *paneSubmitEnterError) Unwrap() error { return e.err }

// paneProvisioningFallback handles best-effort subprocess fallback only when
// no pane was committed (no terminal or SplitPane failed). Once SplitPane
// returns a non-empty pane ID, execution failures must stay on the pane path.
//
// On subprocess success it returns the subprocess response tagged
// ExecutedBackend="subprocess" (recoverable path, S6). When the subprocess
// backend is also unavailable it returns an actionable error naming BOTH
// failure causes plus a recovery instruction, and a response marked so neither
// backend is recorded as successful (S14).
func paneProvisioningFallback(ctx context.Context, cfg OrchestraConfig, req ProviderRequest, paneFailureReason string) (*ProviderResponse, error) {
	switch cfg.FallbackMode {
	case FallbackModeSkip:
		return &ProviderResponse{
			Provider: req.Provider, ExecutedBackend: noneBackendMarker,
			Role: req.Role, Attempt: req.Round, ModelFamily: req.Config.ModelFamily,
			EmptyOutput: true, TerminalState: TerminalSkipped,
			DegradedReasons: []string{"pane_provisioning_skipped"},
		}, nil
	case FallbackModeAbort:
		err := fmt.Errorf("pane provisioning aborted by fallback policy: %s", paneFailureReason)
		return &ProviderResponse{
			Provider: req.Provider, Error: err.Error(), ExecutedBackend: noneBackendMarker,
			Role: req.Role, Attempt: req.Round, ModelFamily: req.Config.ModelFamily,
			EmptyOutput: true, TerminalState: TerminalBlocked,
			DegradedReasons: []string{"pane_provisioning_aborted"},
		}, err
	case "", FallbackModeSubprocess:
		// Continue below.
	default:
		return nil, fmt.Errorf("unknown fallback mode %q", cfg.FallbackMode)
	}
	resp, err := NewSubprocessBackendImpl().Execute(ctx, req)
	if err == nil && resp != nil && !subprocessFailed(resp) {
		resp.ExecutedBackend = "subprocess"
		resp.Role = req.Role
		resp.Attempt = req.Round
		resp.ModelFamily = req.Config.ModelFamily
		resp.TerminalState = TerminalCompleted
		resp.DegradedReasons = append(resp.DegradedReasons, "pane_provisioning_fallback")
		return resp, nil
	}

	// Both backends failed: build an actionable error (REQ-013/S14) rather than
	// surfacing a raw provider/API error.
	subReason := "the -p subprocess fallback was unavailable"
	if err != nil {
		subReason = fmt.Sprintf("the -p subprocess fallback was unavailable (%v)", err)
	} else if resp != nil && subprocessFailed(resp) {
		subReason = "the -p subprocess fallback produced no usable output"
	}

	actionable := fmt.Errorf("%s AND %s; recovery: %s",
		paneFailureReason, subReason, recoveryHint)

	failed := &ProviderResponse{
		Provider:        req.Provider,
		Error:           actionable.Error(),
		ExecutedBackend: noneBackendMarker,
	}
	return failed, actionable
}

// paneExecutionFailure reports a failure after pane provisioning committed.
// It deliberately does not invoke the subprocess backend.
func paneExecutionFailure(req ProviderRequest, reason string) (*ProviderResponse, error) {
	err := errors.New(reason)
	resp := markUnavailableUsage(&ProviderResponse{
		Provider:        req.Provider,
		Error:           reason,
		ExecutedBackend: paneBackendName,
	}, usageSourcePane, usageReasonPane)
	return resp, err
}

// paneProvisioningError identifies failures originating from SplitPane. Callers
// may fall back to subprocess only for this error type. partial records that one
// or more panes were already committed and cleaned before fallback started.
type paneProvisioningError struct {
	cause   error
	partial bool
}

func (e *paneProvisioningError) Error() string { return e.cause.Error() }
func (e *paneProvisioningError) Unwrap() error { return e.cause }

func newPaneProvisioningError(cause error, partial ...bool) error {
	wasPartial := len(partial) > 0 && partial[0]
	return &paneProvisioningError{cause: cause, partial: wasPartial}
}

func isPaneProvisioningError(err error) bool {
	var target *paneProvisioningError
	return errors.As(err, &target)
}

func paneProvisioningWasPartial(err error) bool {
	var target *paneProvisioningError
	return errors.As(err, &target) && target.partial
}

// subprocessFailed reports whether a subprocess response should be treated as a
// failure (timed out or empty), so we do not pass off an empty fallback as success.
func subprocessFailed(resp *ProviderResponse) bool {
	return resp.TimedOut || resp.EmptyOutput
}

// runFallback applies the configured pre-commit pane provisioning policy while
// preserving the original pane failure in human-readable output and recording
// stable degraded evidence in the orchestration receipt.
func runFallback(ctx context.Context, cfg OrchestraConfig, paneFailure error) (*OrchestraResult, error) {
	switch cfg.FallbackMode {
	case "", FallbackModeSubprocess:
		fallbackCfg := cfg
		fallbackCfg.Terminal = nil
		fallbackCfg.SubprocessMode = true
		result, err := RunOrchestra(ctx, fallbackCfg)
		if result != nil {
			annotatePaneFallback(result, paneFailure, "pane_provisioning_fallback")
			result = finalizeOrchestraResultForConfig(result, cfg)
		}
		if err != nil && paneFailure != nil {
			return result, fmt.Errorf("pane provisioning failed (%s); subprocess fallback failed: %w",
				redactFailureText(paneFailure.Error()), err)
		}
		return result, err
	case FallbackModeSkip:
		result := &OrchestraResult{
			Strategy:        cfg.Strategy,
			Summary:         "pane provisioning skipped by fallback policy",
			Degraded:        true,
			DegradedReasons: []string{"pane_provisioning_skipped"},
			TerminalState:   TerminalSkipped,
			GateStatus:      "skipped",
			AnalysisVerdict: "skipped",
		}
		annotatePaneFallback(result, paneFailure, "pane_provisioning_skipped")
		return finalizeOrchestraResultForConfig(result, cfg), nil
	case FallbackModeAbort:
		err := fmt.Errorf("pane provisioning aborted by fallback policy")
		if paneFailure != nil {
			err = fmt.Errorf("pane provisioning aborted by fallback policy: %s", redactFailureText(paneFailure.Error()))
		}
		result := buildFailureResult(cfg, nil, nil, nil, time.Now(), err)
		annotatePaneFallback(result, paneFailure, "pane_provisioning_aborted")
		result = finalizeOrchestraResultForConfig(result, cfg)
		return result, err
	default:
		return nil, fmt.Errorf("unknown fallback mode %q", cfg.FallbackMode)
	}
}

func annotatePaneFallback(result *OrchestraResult, paneFailure error, reason string) {
	if result == nil {
		return
	}
	result.Degraded = true
	appendDegradedReason(result, reason)
	if paneProvisioningWasPartial(paneFailure) {
		appendDegradedReason(result, "pane_partial_split_cleanup")
	}
	if paneFailure == nil {
		if result.Summary == "" {
			result.Summary = "pane provisioning degraded by fallback policy"
		}
		return
	}
	failureDetail := redactFailureText(paneFailure.Error())
	detail := "pane provisioning failed: " + failureDetail
	if result.Summary == "" {
		result.Summary = detail
		return
	}
	if strings.Contains(result.Summary, failureDetail) {
		return
	}
	result.Summary = result.Summary + "; " + detail
}
