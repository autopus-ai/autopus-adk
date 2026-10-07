package orchestra

import (
	"context"
	"fmt"
)

// recheckRoundCount is fixed at two. The measured lever is exactly one
// re-derivation of the model's own answer; a third round was never measured to
// help and would silently multiply provider cost.
const recheckRoundCount = 2

// buildRecheckPrompt renders the round-2 prompt: the original task restated in
// full, the model's own round-1 answer, and an instruction to re-derive.
//
// No peer output is included, and that omission is the point. Measurement on a
// 45-question set with a frontier pair found peer content worth nothing to the
// stronger participant (peers-only vs own-only: 1 vs 1 discordant, p=1.000)
// while the re-derivation itself carried the whole gain (39 -> 44 of 45).
func buildRecheckPrompt(task, own string) string {
	return fmt.Sprintf(
		"%s\n\n# Round 2: Reconsider\nYour first answer is shown below. Re-derive the result carefully and state your final answer.\n\n## Your round 1 answer\n%s\n",
		task, own,
	)
}

// recheckRunner executes one recheck round. The signature matches
// ExecutionBackend.Execute so a backend's method value can be used directly.
type recheckRunner func(context.Context, ProviderRequest) (*ProviderResponse, error)

// recheckTransport picks how each round reaches the provider, and the backend
// name recorded in evidence. Every round spawns the provider directly; a
// provider with a configured backend (such as OMP) runs through it.
//
// SelectBackend is deliberately not used here. It returns the schema-enforcing
// subprocess backend, and recheck exchanges free text with the provider the
// same way consensus, pipeline, and relay do.
func recheckTransport(cfg OrchestraConfig) (recheckRunner, string) {
	return func(ctx context.Context, req ProviderRequest) (*ProviderResponse, error) {
		if req.Config.Backend != "" {
			return runConfiguredProvider(ctx, cfg, req.Config, req.Prompt, req.Role, req.Round, nil)
		}
		return runProvider(ctx, resolveProviderWorkDir(cfg, req.Config), req.Prompt)
	}, "subprocess"
}

// runRecheck executes a single provider twice: an independent first pass, then
// a re-derivation that sees only its own prior answer.
func runRecheck(ctx context.Context, cfg OrchestraConfig) ([]ProviderResponse, [][]ProviderResponse, []FailedProvider, error) {
	if len(cfg.Providers) != 1 {
		return nil, nil, nil, fmt.Errorf(
			"recheck 전략은 프로바이더를 정확히 1개만 사용합니다 (현재 %d개)", len(cfg.Providers))
	}
	provider := cfg.Providers[0]
	run, backendName := recheckTransport(cfg)
	responses := make([]ProviderResponse, 0, recheckRoundCount)
	history := make([][]ProviderResponse, 0, recheckRoundCount)
	prompt := cfg.Prompt

	for round := 1; round <= recheckRoundCount; round++ {
		role := "recheck_r1"
		if round > 1 {
			role = "recheck_r2"
		}
		// Bound each round by the per-provider timeout so a stalled first pass
		// cannot consume the budget the re-derivation still needs.
		perTimeout := providerExecutionTimeout(provider, cfg.TimeoutSeconds)
		req := ProviderRequest{
			Provider: provider.Name, Prompt: prompt, Role: role,
			Round: round, Timeout: perTimeout, Config: provider,
		}
		roundCtx, roundCancel := context.WithTimeout(ctx, perTimeout)
		response, err := run(roundCtx, req)
		roundCancel()
		applyProviderRequestEvidence(response, req, backendName)
		if err != nil {
			failure := buildFailedProviderWithContext(
				provider, response, err, cfg.TimeoutSeconds, role, len(responses) > 0,
			)
			failure.Attempt = round
			failure.ExecutedBackend = backendName
			if response != nil && response.ExecutedBackend != "" {
				failure.ExecutedBackend = response.ExecutedBackend
			}
			return responses, history, []FailedProvider{failure}, err
		}
		responses = append(responses, *response)
		history = append(history, []ProviderResponse{*response})
		prompt = buildRecheckPrompt(cfg.Prompt, response.Output)
	}
	return responses, history, nil, nil
}

// handleRecheck returns the re-derived answer. The round-1 answer stays in
// Responses and RoundHistory as evidence but is never the final answer.
func handleRecheck(_ context.Context, responses []ProviderResponse, _ OrchestraConfig) (string, string, error) {
	if len(responses) == 0 {
		return "", "", fmt.Errorf("응답이 없습니다")
	}
	final := responses[len(responses)-1]
	verdict := "유지"
	if len(responses) > 1 && responses[0].Output != final.Output {
		verdict = "수정"
	}
	summary := fmt.Sprintf("재검토: %s %d라운드, 답변 %s", final.Provider, len(responses), verdict)
	return final.Output, summary, nil
}
