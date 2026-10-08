package orchestra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// providerResult holds the result of a single provider execution.
type providerResult struct {
	resp *ProviderResponse
	err  error
	idx  int
}

// runProvider executes a single provider and returns its response.
// @AX:ANCHOR: [AUTO] internal fan_in=6; signature is a stable contract
func runProvider(ctx context.Context, provider ProviderConfig, prompt string) (*ProviderResponse, error) {
	start := time.Now()

	args := append([]string{}, provider.Args...)
	if provider.PromptViaArgs {
		args = append(args, "-p", prompt)
	}
	args, lastMessagePath, cleanupLastMessage, err := attachCodexLastMessageCapture(ProviderRequest{Config: provider, Prompt: prompt}, args)
	if err != nil {
		return nil, err
	}
	defer cleanupLastMessage()

	cmd := newCommand(ctx, provider.Binary, args...)
	if err := applyProviderEnv(cmd, provider.KeepEnv, provider.UnsetEnv); err != nil {
		return nil, err
	}
	cmd.SetDir(provider.WorkDir)
	execution := newProviderExecution(provider, args, start)

	detector := &fastFailDetector{}
	fastFailRules := resolveFastFailRules(provider.FastFailPatterns)
	stdoutBuf := newFastFailBuffer(detector, fastFailRules, func(reason string) {
		_ = cmd.Terminate(provider.Name + " fast-fail: " + reason)
	})
	stderrBuf := newFastFailBuffer(detector, fastFailRules, func(reason string) {
		_ = cmd.Terminate(provider.Name + " fast-fail: " + reason)
	})
	stdoutBuf.limit, stderrBuf.limit = provider.MaxOutputBytes, provider.MaxOutputBytes
	cmd.SetStdout(stdoutBuf)
	cmd.SetStderr(stderrBuf)
	readyMonitor := newResultReadyMonitor(provider, stdoutBuf, stderrBuf)

	if !provider.PromptViaArgs {
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("%s stdin 파이프 생성 실패: %w", provider.Name, err)
		}

		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("%s 시작 실패: %w", provider.Name, err)
		}
		execution.PID = cmd.PID()
		waitCh := startCommandWait(cmd)

		if _, err := io.WriteString(stdinPipe, prompt); err != nil {
			_ = cmd.Terminate(provider.Name + " stdin write failure")
			drainCommandWait(waitCh)
			return nil, fmt.Errorf("%s stdin 쓰기 실패: %w", provider.Name, err)
		}
		_ = stdinPipe.Close()

		waitErr := waitForCommand(ctx, cmd, provider.Name, waitCh, readyMonitor)
		return finishProviderResponse(providerResponseArgs{start, provider, detector.Reason(), waitErr, ctx, cmd.ExitCode(), execution},
			stdoutBuf.String(), stderrBuf.String(), lastMessagePath)
	}

	cmd.SetStdin(nil)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s 시작 실패: %w", provider.Name, err)
	}
	execution.PID = cmd.PID()

	waitCh := startCommandWait(cmd)
	waitErr := waitForCommand(ctx, cmd, provider.Name, waitCh, readyMonitor)
	return finishProviderResponse(providerResponseArgs{start, provider, detector.Reason(), waitErr, ctx, cmd.ExitCode(), execution},
		stdoutBuf.String(), stderrBuf.String(), lastMessagePath)
}

func runProviderWithProgress(ctx context.Context, provider ProviderConfig, prompt string, tracker *ProgressTracker) (*ProviderResponse, error) {
	if tracker != nil {
		tracker.MarkRunning(provider.Name)
	}
	resp, err := runProvider(ctx, provider, prompt)
	if tracker == nil {
		return resp, err
	}
	if err != nil || resp == nil || resp.TimedOut || resp.EmptyOutput {
		tracker.MarkFailed(provider.Name)
	} else {
		tracker.MarkDone(provider.Name)
	}
	return resp, err
}

func startCommandWait(cmd command) <-chan error {
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	return waitCh
}

func waitForCommand(ctx context.Context, cmd command, providerName string, waitCh <-chan error, readyMonitor *resultReadyMonitor) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var readyGrace <-chan time.Time
	resultReady := false

	select {
	case err := <-waitCh:
		return err
	default:
	}

	for {
		select {
		case err := <-waitCh:
			if resultReady {
				return errResultReady
			}
			return err
		case <-ctx.Done():
			_ = cmd.Terminate(providerName + " context cancelled")
			select {
			case err := <-waitCh:
				return err
			case <-time.After(providerWaitGracePeriod):
				return ctx.Err()
			}
		case <-ticker.C:
			if resultReady || readyMonitor == nil || !readyMonitor.ShouldTerminate(time.Now()) {
				continue
			}
			resultReady = true
			_ = cmd.Terminate(providerName + " semantic result ready")
			readyGrace = time.After(providerWaitGracePeriod)
		case <-readyGrace:
			return errResultReady
		}
	}
}

func drainCommandWait(waitCh <-chan error) {
	select {
	case <-waitCh:
	case <-time.After(providerWaitGracePeriod):
	}
}

func buildProviderResponse(start time.Time, provider ProviderConfig, stdout, stderr, fastFailReason string, waitErr error, ctx context.Context, exitCode int) (*ProviderResponse, error) {
	duration := time.Since(start)
	resultReady := errors.Is(waitErr, errResultReady)
	if resultReady {
		waitErr = nil
		exitCode = 0
	}

	resp := &ProviderResponse{
		Provider:        provider.Name,
		Output:          stdout,
		Error:           stderr,
		Duration:        duration,
		ExitCode:        exitCode,
		EmptyOutput:     strings.TrimSpace(stdout) == "",
		ExecutedBackend: "subprocess",
		ModelFamily:     provider.ModelFamily,
	}

	if ctx.Err() != nil {
		resp.TimedOut = true
	}

	if fastFailReason != "" {
		return resp, fmt.Errorf("%s fast-fail: %s", provider.Name, fastFailReason)
	}

	if waitErr != nil && !resp.TimedOut && resp.ExitCode != 0 {
		return resp, fmt.Errorf("%s 실행 오류 (exit %d): %w", provider.Name, resp.ExitCode, waitErr)
	}

	return resp, nil
}

// detectProviderFastFail evaluates output against the built-in default fast-fail
// rules. Retained as a thin wrapper over matchFastFailRules so the default reason
// strings have a single declarative source (DefaultFastFailRules).
func detectProviderFastFail(output string) string {
	return matchFastFailRules(output, DefaultFastFailRules())
}

func buildAllProvidersFailedError(failed []FailedProvider, fallback error) error {
	if fallback == nil && allFailuresTimedOut(failed) {
		return nil
	}

	if len(failed) == 0 {
		if fallback != nil {
			return fallback
		}
		return fmt.Errorf("모든 프로바이더가 실패했습니다")
	}

	details := make([]string, 0, len(failed))
	for _, fp := range failed {
		details = append(details, fmt.Sprintf("%s: %s", fp.Name, fp.Error))
	}

	if fallback != nil {
		return fmt.Errorf("모든 프로바이더가 실패했습니다 (%s): %w", strings.Join(details, "; "), fallback)
	}
	return fmt.Errorf("모든 프로바이더가 실패했습니다: %s", strings.Join(details, "; "))
}

func allFailuresTimedOut(failed []FailedProvider) bool {
	if len(failed) == 0 {
		return false
	}
	for _, fp := range failed {
		if !strings.HasPrefix(fp.Error, "timeout:") {
			return false
		}
	}
	return true
}
