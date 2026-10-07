package orchestra

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/detect"
)

// RunOrchestra executes orchestration according to the given config.
// @AX:ANCHOR: [AUTO] public API — 4 callers; do not change signature
// @AX:REASON: CLI, spec-review loop, and tests rely on the result/error contract and degraded-provider propagation.
func RunOrchestra(ctx context.Context, cfg OrchestraConfig) (*OrchestraResult, error) {
	if err := validateOrchestraProviderConfig(cfg); err != nil {
		return nil, err
	}
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("providers 목록이 비어있습니다")
	}
	if !cfg.Strategy.IsValid() {
		return nil, fmt.Errorf("유효하지 않은 전략: %q", cfg.Strategy)
	}
	if !cfg.FallbackMode.IsValid() {
		return nil, fmt.Errorf("unknown fallback mode %q", cfg.FallbackMode)
	}
	if result, err := preflightJudgeFamilySeparation(cfg); err != nil {
		return result, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, orchestrationTimeout(cfg))
	defer cancel()

	for _, p := range cfg.Providers {
		if p.Backend == "" && !detect.IsInstalled(p.Binary) {
			return nil, fmt.Errorf("프로바이더 바이너리를 찾을 수 없습니다: %q", p.Binary)
		}
	}

	start := time.Now()
	var responses []ProviderResponse
	var roundHistory [][]ProviderResponse
	var failed []FailedProvider
	var err error

	switch cfg.Strategy {
	case StrategyPipeline:
		responses, failed, err = runPipeline(timeoutCtx, cfg)
	case StrategyFastest:
		responses, err = runFastest(timeoutCtx, cfg)
	case StrategyDebate:
		responses, roundHistory, failed, err = runDebate(timeoutCtx, cfg)
	case StrategyRelay:
		responses, err = runRelay(timeoutCtx, &cfg)
		failed = decorateRelayExecutionEvidence(responses, cfg)
	case StrategyRecheck:
		responses, roundHistory, failed, err = runRecheck(timeoutCtx, cfg)
	default:
		// consensus: prepend structured prompt prefix, then run parallel with graceful degradation
		consensusCfg := cfg
		consensusCfg.Prompt = buildStructuredPromptPrefix() + cfg.Prompt
		responses, failed, err = runParallel(timeoutCtx, consensusCfg)
	}
	if err != nil {
		return buildFailureResult(cfg, responses, failed, roundHistory, start, err), err
	}

	total := time.Since(start)

	merged, summary, mergeErr := mergeResponsesByStrategy(timeoutCtx, responses, cfg)
	if mergeErr != nil {
		return buildFailureResult(cfg, responses, failed, roundHistory, start, mergeErr), mergeErr
	}

	// Append failed provider info to summary if any
	if len(failed) > 0 {
		var names []string
		for _, f := range failed {
			names = append(names, f.Name)
		}
		summary = fmt.Sprintf("%s (실패: %s)", summary, strings.Join(names, ", "))
	}

	result := &OrchestraResult{
		Strategy:        cfg.Strategy,
		Responses:       responses,
		RoundHistory:    roundHistory,
		Merged:          merged,
		Duration:        total,
		Summary:         summary,
		FailedProviders: failed,
		RunID:           cfg.RunID,
		Degraded:        len(failed) > 0,
	}
	if cfg.Strategy == StrategyFastest {
		result.AttemptedProviders = providerConfigNames(cfg.Providers)
		result.DispatchCount = len(cfg.Providers)
	}
	if cfg.Strategy == StrategyDebate {
		result.FreshJudgeSession = freshJudgeSessionFromResponses(responses)
		return finalizeDebateOutcome(result, cfg)
	}
	return finalizeOrchestraResultForConfig(result, cfg), nil
}

// runParallel executes all providers in parallel with per-goroutine context (R1)
// and per-provider timeout (R2). Error is non-nil only when ALL providers fail.
func runParallel(ctx context.Context, cfg OrchestraConfig) ([]ProviderResponse, []FailedProvider, error) {
	results := make([]providerResult, len(cfg.Providers))
	providerNames := make([]string, len(cfg.Providers))
	for i, p := range cfg.Providers {
		providerNames[i] = p.Name
	}
	progress := NewProgressTracker(providerNames)
	stopProgress := progress.StartHeartbeat(ctx, progressHeartbeatInterval)
	defer stopProgress()

	var wg sync.WaitGroup

	for i, p := range cfg.Providers {
		wg.Add(1)
		perTimeout := providerExecutionTimeout(p, cfg.TimeoutSeconds)
		// R1: derive per-goroutine context for independent cancellation
		childCtx, childCancel := context.WithTimeout(ctx, perTimeout)
		go func(idx int, provider ProviderConfig, cancel context.CancelFunc) {
			defer wg.Done()
			defer cancel()
			role := "participant"
			if cfg.Strategy == StrategyDebate {
				role = "debater_r1"
			}
			resp, err := runConfiguredProvider(
				childCtx, cfg, provider, cfg.Prompt, role, 1, progress,
			)
			results[idx] = providerResult{resp: resp, err: err, idx: idx}
		}(i, p, childCancel)
	}
	wg.Wait()

	var responses []ProviderResponse
	var failedResults []providerResult

	for _, r := range results {
		if r.err != nil {
			failedResults = append(failedResults, r)
		} else if r.resp != nil && (r.resp.TimedOut || r.resp.EmptyOutput) {
			failedResults = append(failedResults, r)
		} else if r.resp == nil {
			failedResults = append(failedResults, r)
		} else {
			responses = append(responses, *r.resp)
		}
	}
	otherProvidersContinued := len(responses) > 0
	failed := make([]FailedProvider, 0, len(failedResults))
	for _, r := range failedResults {
		failure := buildFailedProviderWithContext(
			cfg.Providers[r.idx],
			r.resp,
			r.err,
			cfg.TimeoutSeconds,
			"",
			otherProvidersContinued,
		)
		failure.Attempt = 1
		if cfg.Strategy == StrategyDebate {
			failure.Role = "debater_r1"
		}
		failure.ExecutedBackend = "subprocess"
		if r.resp != nil && r.resp.ExecutedBackend != "" {
			failure.ExecutedBackend = r.resp.ExecutedBackend
		}
		failed = append(failed, failure)
	}

	if len(responses) == 0 {
		var fallback error
		if len(results) > 0 {
			fallback = results[0].err
		}
		return nil, failed, buildAllProvidersFailedError(failed, fallback)
	}
	return responses, failed, nil
}

func providerExecutionTimeout(provider ProviderConfig, fallbackSeconds int) time.Duration {
	if provider.ExecutionTimeout > 0 {
		return provider.ExecutionTimeout
	}
	timeout := time.Duration(fallbackSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return timeout
}

func orchestrationTimeout(cfg OrchestraConfig) time.Duration {
	baseSeconds := cfg.TimeoutSeconds
	if baseSeconds <= 0 {
		baseSeconds = 120
	}
	base := time.Duration(baseSeconds) * time.Second

	// Sequential strategies run providers one after another, so the global
	// deadline must be the SUM of per-provider budgets. Using the max (as the
	// parallel path does) would let later providers start with an already
	// expired context — the same failure class as the spec-review 0/N watchdog.
	if cfg.Strategy == StrategyPipeline || cfg.Strategy == StrategyRelay {
		total := time.Duration(0)
		for _, provider := range cfg.Providers {
			total += providerExecutionTimeout(provider, cfg.TimeoutSeconds)
		}
		if total < base {
			total = base
		}
		return total
	}

	longestProvider := base
	for _, provider := range cfg.Providers {
		if providerTimeout := providerExecutionTimeout(provider, cfg.TimeoutSeconds); providerTimeout > longestProvider {
			longestProvider = providerTimeout
		}
	}
	if cfg.JudgeProvider != "" {
		judgeTimeout := providerExecutionTimeout(findOrBuildJudgeConfig(cfg), cfg.TimeoutSeconds)
		if judgeTimeout > longestProvider {
			longestProvider = judgeTimeout
		}
	}

	phaseCount := 1
	if cfg.Strategy == StrategyDebate {
		rounds := cfg.DebateRounds
		if rounds <= 0 {
			rounds = 1
		}
		if rounds >= 2 {
			phaseCount++
		}
		if cfg.JudgeProvider != "" && !cfg.NoJudge {
			phaseCount++
		}
	}
	return longestProvider * time.Duration(phaseCount)
}

// runFastest는 모든 프로바이더를 병렬로 실행하고 첫 번째 성공 응답을 반환한다.
func runFastest(ctx context.Context, cfg OrchestraConfig) ([]ProviderResponse, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan ProviderResponse, len(cfg.Providers))
	var wg sync.WaitGroup

	for _, p := range cfg.Providers {
		wg.Add(1)
		go func(provider ProviderConfig) {
			defer wg.Done()
			resp, err := runConfiguredProvider(ctx, cfg, provider, cfg.Prompt, "fastest", 1, nil)
			// Reject failures the same way runParallel does: an exit-0 provider
			// with empty stdout is not a usable "fastest" winner (false green).
			if err != nil || resp == nil || resp.TimedOut || resp.EmptyOutput {
				return
			}
			select {
			case resultCh <- *resp:
				cancel() // 첫 번째 응답이 도착하면 나머지 취소
			default:
			}
		}(p)
	}

	// 고루틴 완료 후 채널 닫기
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	resp, ok := <-resultCh
	if !ok {
		return nil, fmt.Errorf("모든 프로바이더가 응답하지 않았습니다")
	}
	return []ProviderResponse{resp}, nil
}
