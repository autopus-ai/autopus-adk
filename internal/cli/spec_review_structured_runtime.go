package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

type indexedSpecReviewOutcome struct {
	index   int
	outcome specReviewStructuredOutcome
}

func structuredSpecReviewContext(ctx context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
}

func structuredReviewExecutionMode(backend orchestra.ExecutionBackend) string {
	return "parallel"
}

func runStructuredSpecReviewProviders(
	ctx context.Context,
	cfg orchestra.OrchestraConfig,
	backend orchestra.ExecutionBackend,
	parser *orchestra.OutputParser,
	schemaPath string,
	embeddedSchema string,
	mode string,
) []specReviewStructuredOutcome {
	if mode == "sequential" {
		return runStructuredSpecReviewProvidersSequential(ctx, cfg, backend, parser, schemaPath, embeddedSchema, mode)
	}
	return runStructuredSpecReviewProvidersParallel(ctx, cfg, backend, parser, schemaPath, embeddedSchema, mode)
}

func runStructuredSpecReviewProvidersSequential(
	ctx context.Context,
	cfg orchestra.OrchestraConfig,
	backend orchestra.ExecutionBackend,
	parser *orchestra.OutputParser,
	schemaPath string,
	embeddedSchema string,
	mode string,
) []specReviewStructuredOutcome {
	results := make([]specReviewStructuredOutcome, len(cfg.Providers))
	start := time.Now()
	for i, provider := range cfg.Providers {
		if err := ctx.Err(); err != nil {
			backendName := specReviewProviderBackendName(backend, provider)
			results[i] = pendingStructuredReviewTimeoutOutcome(provider, backendName, "queued", cfg.TimeoutSeconds, time.Since(start), err)
			logStructuredReviewOutcome(provider.Name, backendName, results[i], time.Since(start))
			continue
		}
		results[i] = executeStructuredSpecReviewProvider(ctx, cfg, backend, parser, schemaPath, embeddedSchema, provider, mode)
	}
	return results
}

func runStructuredSpecReviewProvidersParallel(
	ctx context.Context,
	cfg orchestra.OrchestraConfig,
	backend orchestra.ExecutionBackend,
	parser *orchestra.OutputParser,
	schemaPath string,
	embeddedSchema string,
	mode string,
) []specReviewStructuredOutcome {
	results := make([]specReviewStructuredOutcome, len(cfg.Providers))
	done := make([]bool, len(cfg.Providers))
	outcomes := make(chan indexedSpecReviewOutcome, len(cfg.Providers))
	start := time.Now()

	for i, provider := range cfg.Providers {
		go func(idx int, provider orchestra.ProviderConfig) {
			timeout := specReviewAttemptTimeoutBudget(provider, cfg.TimeoutSeconds)
			childCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			outcomes <- indexedSpecReviewOutcome{
				index:   idx,
				outcome: executeStructuredSpecReviewProvider(childCtx, cfg, backend, parser, schemaPath, embeddedSchema, provider, mode),
			}
		}(i, provider)
	}

	pending := len(cfg.Providers)
	for pending > 0 {
		select {
		case result := <-outcomes:
			if result.index < 0 || result.index >= len(results) || done[result.index] {
				continue
			}
			results[result.index] = result.outcome
			done[result.index] = true
			pending--
		case <-ctx.Done():
			for i, provider := range cfg.Providers {
				if done[i] {
					continue
				}
				backendName := specReviewProviderBackendName(backend, provider)
				results[i] = pendingStructuredReviewTimeoutOutcome(provider, backendName, "provider_execution", cfg.TimeoutSeconds, time.Since(start), ctx.Err())
				logStructuredReviewOutcome(provider.Name, backendName, results[i], time.Since(start))
			}
			// Drain the still-running provider goroutines before returning so they
			// cannot write shared process globals (os.Stderr via structured review
			// logging) after the caller has moved on and restored those globals.
			// Bounded by a grace window so a backend that ignores context
			// cancellation cannot reintroduce the hang the watchdog prevents.
			drainInFlightSpecReviewProviders(outcomes, pending)
			return results
		}
	}
	return results
}

// specReviewDrainGrace bounds how long the parallel runner waits for in-flight
// provider goroutines to exit after the review context is cancelled. Backends
// that honor context cancellation return well within this window; the bound
// stops a context-ignoring backend from reintroducing the watchdog hang.
const specReviewDrainGrace = 2 * time.Second

// drainInFlightSpecReviewProviders waits for the remaining provider goroutines
// to send their outcomes so they do not outlive the parallel runner and race on
// shared process globals (notably os.Stderr through structured review logging).
// The drain is bounded by specReviewDrainGrace.
func drainInFlightSpecReviewProviders(outcomes <-chan indexedSpecReviewOutcome, pending int) {
	if pending <= 0 {
		return
	}
	grace := time.NewTimer(specReviewDrainGrace)
	defer grace.Stop()
	for pending > 0 {
		select {
		case <-outcomes:
			pending--
		case <-grace.C:
			return
		}
	}
}

func executeStructuredSpecReviewProvider(
	ctx context.Context,
	cfg orchestra.OrchestraConfig,
	backend orchestra.ExecutionBackend,
	parser *orchestra.OutputParser,
	schemaPath string,
	embeddedSchema string,
	provider orchestra.ProviderConfig,
	mode string,
) specReviewStructuredOutcome {
	timeout := specReviewTimeout(provider, cfg.TimeoutSeconds)
	start := time.Now()
	backendName := specReviewProviderBackendName(backend, provider)
	fmt.Fprintf(os.Stderr, "SPEC 리뷰 provider 시작: %s (backend=%s, timeout=%s, mode=%s)\n",
		provider.Name, backendName, timeout, mode)
	if backend == nil {
		outcome := malformedStructuredOutcome(provider.Name, fmt.Errorf("execution backend unavailable"), nil, backendName)
		decorateStructuredFailure(&outcome, timeout, time.Since(start))
		logStructuredReviewOutcome(provider.Name, backendName, outcome, time.Since(start))
		return outcome
	}

	prompt := specReviewStructuredPrompt(cfg.Prompt, embeddedSchema, shouldInlineStructuredReviewSchema(backend, provider))
	req := orchestra.ProviderRequest{
		Provider:   provider.Name,
		Prompt:     prompt,
		SchemaPath: schemaPath,
		Role:       "reviewer",
		Timeout:    timeout,
		Config:     provider,
	}

	resp, execErr := backend.Execute(ctx, req)
	elapsed := time.Since(start)
	if execErr != nil {
		if ctx.Err() != nil {
			outcome := structuredReviewTimeoutOutcome(provider.Name, backendName, "provider_execution", timeout, elapsed, ctx.Err(), resp)
			logStructuredReviewOutcome(provider.Name, backendName, outcome, elapsed)
			return outcome
		}
		outcome := malformedStructuredOutcome(provider.Name, fmt.Errorf("execution failed: %w", execErr), resp, backendName)
		decorateStructuredFailure(&outcome, timeout, elapsed)
		logStructuredReviewOutcome(provider.Name, backendName, outcome, elapsed)
		return outcome
	}
	if resp == nil {
		outcome := malformedStructuredOutcome(provider.Name, fmt.Errorf("provider returned nil response"), nil, backendName)
		decorateStructuredFailure(&outcome, timeout, elapsed)
		logStructuredReviewOutcome(provider.Name, backendName, outcome, elapsed)
		return outcome
	}
	if strings.TrimSpace(resp.ExecutedBackend) == "" {
		resp.ExecutedBackend = backendName
	}
	if resp.TimedOut {
		outcome := structuredReviewTimeoutOutcome(provider.Name, resp.ExecutedBackend, "provider_execution", timeout, elapsed, fmt.Errorf("provider timed out after %s", req.Timeout), resp)
		logStructuredReviewOutcome(provider.Name, resp.ExecutedBackend, outcome, elapsed)
		return outcome
	}
	if resp.EmptyOutput {
		outcome := malformedStructuredOutcome(provider.Name, fmt.Errorf("provider returned empty output"), resp, backendName)
		decorateStructuredFailure(&outcome, timeout, elapsed)
		logStructuredReviewOutcome(provider.Name, backendName, outcome, elapsed)
		return outcome
	}
	if _, parseErr := parser.ParseReviewer(resp.Output); parseErr != nil {
		if retryResp, retryErr := repromptStructuredReviewerJSON(ctx, backend, parser, req, resp, parseErr); retryErr == nil {
			outcome := specReviewStructuredOutcome{resp: *retryResp}
			logStructuredReviewOutcome(provider.Name, retryResp.ExecutedBackend, outcome, time.Since(start))
			return outcome
		} else {
			resp = mergeStructuredRepromptFailureResponse(resp, retryResp)
			parseErr = fmt.Errorf("invalid reviewer JSON after reprompt: initial: %w; reprompt: %v", parseErr, retryErr)
			elapsed = time.Since(start)
		}
		outcome := malformedStructuredOutcome(provider.Name, fmt.Errorf("invalid reviewer JSON: %w", parseErr), resp, backendName)
		decorateStructuredFailure(&outcome, timeout, elapsed)
		logStructuredReviewOutcome(provider.Name, backendName, outcome, elapsed)
		return outcome
	}

	outcome := specReviewStructuredOutcome{resp: *resp}
	logStructuredReviewOutcome(provider.Name, resp.ExecutedBackend, outcome, elapsed)
	return outcome
}

func pendingStructuredReviewTimeoutOutcome(provider orchestra.ProviderConfig, backendName, stage string, timeoutSeconds int, elapsed time.Duration, err error) specReviewStructuredOutcome {
	timeout := specReviewTimeout(provider, timeoutSeconds)
	return structuredReviewTimeoutOutcome(provider.Name, backendName, stage, timeout, elapsed, err, nil)
}

func structuredReviewTimeoutOutcome(provider, backendName, stage string, timeout, elapsed time.Duration, err error, sourceResp *orchestra.ProviderResponse) specReviewStructuredOutcome {
	if err == nil {
		err = context.DeadlineExceeded
	}
	description := fmt.Errorf("review watchdog timed out for provider %s backend=%s stage=%s timeout=%s elapsed=%s: %w",
		provider, backendName, stage, timeout, elapsed.Round(time.Millisecond), err)
	outcome := malformedStructuredOutcome(provider, description, sourceResp, backendName)
	decorateStructuredFailure(&outcome, timeout, elapsed)
	if outcome.failed != nil {
		outcome.failed.FailureClass = "timeout"
		outcome.failed.TimeoutSource = "spec_review_timeout"
	}
	outcome.resp.TimedOut = true
	if outcome.resp.Duration <= 0 {
		outcome.resp.Duration = elapsed
	}
	return outcome
}

func decorateStructuredFailure(outcome *specReviewStructuredOutcome, timeout, elapsed time.Duration) {
	if outcome == nil || outcome.failed == nil {
		return
	}
	if outcome.failed.ExecutedBackend == "" {
		outcome.failed.ExecutedBackend = outcome.resp.ExecutedBackend
	}
	outcome.failed.Role = "reviewer"
	outcome.failed.ConfiguredDuration = timeout
	outcome.failed.ElapsedDuration = elapsed
	if outcome.failed.FailureClass == "timeout" && outcome.failed.TimeoutSource == "" {
		outcome.failed.TimeoutSource = "provider_execution_timeout"
	}
	if outcome.resp.Duration <= 0 {
		outcome.resp.Duration = elapsed
	}
	if outcome.failed.FailureClass == "timeout" {
		outcome.resp.TimedOut = true
	}
}

func logStructuredReviewOutcome(provider, backendName string, outcome specReviewStructuredOutcome, elapsed time.Duration) {
	if outcome.failed != nil {
		fmt.Fprintf(os.Stderr, "SPEC 리뷰 provider 실패: %s (backend=%s, class=%s, elapsed=%s): %s\n",
			provider, backendName, outcome.failed.FailureClass, elapsed.Round(time.Millisecond), truncateStructuredReviewError(outcome.failed.Error, 180))
		return
	}
	fmt.Fprintf(os.Stderr, "SPEC 리뷰 provider 완료: %s (backend=%s, elapsed=%s)\n",
		provider, backendName, elapsed.Round(time.Millisecond))
}

func formatStructuredReviewTimeout(timeoutSeconds int) string {
	if timeoutSeconds <= 0 {
		return "default"
	}
	return (time.Duration(timeoutSeconds) * time.Second).String()
}
