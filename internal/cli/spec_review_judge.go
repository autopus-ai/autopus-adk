package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

const specReviewJudgeSuffix = " (judge)"

func runStructuredSpecReviewJudge(
	ctx context.Context,
	cfg orchestra.OrchestraConfig,
	backend orchestra.ExecutionBackend,
	reviewerResponses []orchestra.ProviderResponse,
) (*orchestra.ProviderResponse, *orchestra.FailedProvider) {
	judgeName := strings.TrimSpace(cfg.JudgeProvider)
	if judgeName == "" || cfg.NoJudge {
		return nil, nil
	}
	if len(reviewerResponses) == 0 {
		return nil, nil
	}

	judgeConfig := specReviewJudgeConfig(cfg)
	schema := &orchestra.SchemaBuilder{}
	schemaPath, cleanup, err := schema.WriteToFile("review_judge")
	if err != nil {
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, nil, "schema_error", err, 0)
	}
	defer cleanup()
	schemaJSON, err := schema.EmbedInPrompt("review_judge")
	if err != nil {
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, nil, "schema_error", err, 0)
	}
	promptBuilder, err := orchestra.NewPromptBuilder()
	if err != nil {
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, nil, "prompt_error", err, 0)
	}
	participants, aliasToProvider := orchestra.AnonymizeReviewParticipants(reviewerResponses)
	mode := orchestraReviewMode(cfg.Prompt)
	prompt, err := promptBuilder.BuildReviewJudge(orchestra.ReviewJudgeData{
		SpecID:        specIDFromReviewPrompt(cfg.Prompt),
		Mode:          mode,
		Participants:  participants,
		PriorFindings: priorFindingsFromReviewPrompt(cfg.Prompt),
		Checklist:     "",
		SchemaJSON:    schemaJSON,
	})
	if err != nil {
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, nil, "prompt_error", err, 0)
	}

	timeout := specReviewTimeout(judgeConfig, cfg.TimeoutSeconds)
	judgeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	request := orchestra.ProviderRequest{
		Provider: judgeName, Prompt: prompt, SchemaPath: schemaPath, Role: "judge",
		Timeout: timeout, Config: judgeConfig,
	}
	response, execErr := backend.Execute(judgeCtx, request)
	elapsed := time.Since(start)
	if failureClass, failureErr := classifyStructuredJudgeExecution(judgeCtx, response, execErr, timeout); failureErr != nil {
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, response, failureClass, failureErr, elapsed)
	}

	parsed, parseErr := (&orchestra.OutputParser{}).ParseReviewJudge(response.Output)
	if parseErr != nil {
		err := fmt.Errorf("invalid review judge JSON: %w", parseErr)
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, response, "invalid_output", err, elapsed)
	}
	if validationErr := validateStructuredReviewJudge(parsed, aliasToProvider); validationErr != nil {
		err := fmt.Errorf("invalid review judge JSON: %w", validationErr)
		return failStructuredSpecReviewJudge(cfg, judgeConfig, backend, response, "invalid_output", err, elapsed)
	}
	response.Provider = judgeName + specReviewJudgeSuffix
	response.Role = "judge"
	if response.Duration <= 0 {
		response.Duration = elapsed
	}
	if strings.TrimSpace(response.ExecutedBackend) == "" {
		response.ExecutedBackend = backend.Name()
	}
	if strings.TrimSpace(response.ModelFamily) == "" {
		response.ModelFamily = judgeConfig.ModelFamily
	}
	accepted, rejected, merged := reviewJudgeDecisionCounts(parsed)
	fmt.Fprintf(os.Stderr, "SPEC 리뷰 judge 완료: %s (verdict=%s, accepted=%d, rejected=%d, merged=%d)\n",
		judgeName, parsed.Verdict, accepted, rejected, merged)
	return response, nil
}

func specReviewJudgeConfig(cfg orchestra.OrchestraConfig) orchestra.ProviderConfig {
	if cfg.JudgeConfig != nil {
		return *cfg.JudgeConfig
	}
	judgeName := strings.TrimSpace(cfg.JudgeProvider)
	for _, provider := range cfg.Providers {
		if provider.Name == judgeName {
			return provider
		}
	}
	return orchestra.ProviderConfig{Name: judgeName, Binary: judgeName}
}

func classifyStructuredJudgeExecution(
	ctx context.Context,
	response *orchestra.ProviderResponse,
	execErr error,
	timeout time.Duration,
) (string, error) {
	if (response != nil && response.TimedOut) ||
		errors.Is(execErr, context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.DeadlineExceeded) {
		cause := execErr
		if cause == nil {
			cause = context.DeadlineExceeded
		}
		return "timeout", fmt.Errorf("judge timed out after %s: %w", timeout, cause)
	}
	if execErr != nil {
		return "execution_error", fmt.Errorf("judge execution failed: %w", execErr)
	}
	if response == nil {
		return "execution_error", fmt.Errorf("judge returned nil response")
	}
	if response.ExitCode != 0 {
		return "execution_error", fmt.Errorf("judge exited with code %d: %s", response.ExitCode, strings.TrimSpace(response.Error))
	}
	if response.EmptyOutput || strings.TrimSpace(response.Output) == "" {
		return "empty_output", fmt.Errorf("judge returned empty output")
	}
	return "", nil
}

func failStructuredSpecReviewJudge(
	cfg orchestra.OrchestraConfig,
	judgeConfig orchestra.ProviderConfig,
	backend orchestra.ExecutionBackend,
	response *orchestra.ProviderResponse,
	failureClass string,
	err error,
	elapsed time.Duration,
) (*orchestra.ProviderResponse, *orchestra.FailedProvider) {
	backendName := ""
	if backend != nil {
		backendName = backend.Name()
	}
	failure := &orchestra.FailedProvider{
		Name: strings.TrimSpace(cfg.JudgeProvider) + specReviewJudgeSuffix, Role: "judge",
		ModelFamily: judgeConfig.ModelFamily, ExecutedBackend: backendName,
		FailureClass: failureClass, Error: err.Error(), OtherProvidersContinued: true,
		ConfiguredDuration: specReviewTimeout(judgeConfig, cfg.TimeoutSeconds), ElapsedDuration: elapsed,
		CollectionMode: backendName,
	}
	if response != nil {
		if response.ModelFamily != "" {
			failure.ModelFamily = response.ModelFamily
		}
		if response.ExecutedBackend != "" {
			failure.ExecutedBackend = response.ExecutedBackend
			failure.CollectionMode = response.ExecutedBackend
		}
		failure.ExitCode = response.ExitCode
		failure.TimedOut = response.TimedOut
		failure.StderrPreview = truncateStructuredReviewError(response.Error, 240)
		failure.OutputPreview = truncateStructuredReviewError(response.Output, 240)
		// The receipt judges the sandbox from this recorded launch; a judge
		// that ended before Execute (schema or prompt error) records none.
		failure.Execution = response.Execution
	}
	if failureClass == "timeout" {
		failure.TimedOut = true
		failure.TimeoutSource = "spec_review_judge_timeout"
		failure.NextRemediation = "increase the SPEC review timeout or simplify the judge input"
	} else {
		failure.NextRemediation = "inspect the judge output and provider configuration"
	}
	fmt.Fprintf(os.Stderr, "SPEC 리뷰 judge 실패: %s (%s): %s\n",
		cfg.JudgeProvider, failureClass, truncateStructuredReviewError(failure.Error, 180))
	return nil, failure
}

func validateStructuredReviewJudge(
	out *orchestra.ReviewJudgeOutput,
	aliasToProvider map[string]string,
) error {
	accepted, _, _ := reviewJudgeDecisionCounts(out)
	if out != nil && (out.Verdict == "REVISE" || out.Verdict == "REJECT") && accepted == 0 {
		return fmt.Errorf("%s without accepted findings", out.Verdict)
	}
	if out == nil {
		return nil
	}
	for _, finding := range out.Findings {
		for _, source := range finding.Sources {
			if _, known := aliasToProvider[strings.TrimSpace(source)]; !known {
				return fmt.Errorf("judge output references unknown reviewer alias %q", source)
			}
		}
	}
	return nil
}

func reviewJudgeDecisionCounts(out *orchestra.ReviewJudgeOutput) (accepted, rejected, merged int) {
	if out == nil {
		return 0, 0, 0
	}
	for _, finding := range out.Findings {
		switch finding.Decision {
		case "accept":
			accepted++
		case "reject":
			rejected++
		case "merge":
			merged++
		}
	}
	return accepted, rejected, merged
}

// specReviewWatchdogForConfig preserves the legacy reviewer budget when judge is
// disabled and adds the sequential judge timeout only when it can run.
func specReviewWatchdogForConfig(cfg orchestra.OrchestraConfig) int {
	seconds := specReviewWatchdogSeconds(cfg.Providers, cfg.TimeoutSeconds)
	if strings.TrimSpace(cfg.JudgeProvider) == "" || cfg.NoJudge {
		return seconds
	}
	judgeTimeout := specReviewTimeout(specReviewJudgeConfig(cfg), cfg.TimeoutSeconds)
	return seconds + int((judgeTimeout+time.Second-1)/time.Second)
}

func orchestraReviewMode(prompt string) string {
	if isVerifyReviewPrompt(prompt) {
		return "verify"
	}
	return "discover"
}

func specIDFromReviewPrompt(prompt string) string {
	const marker = "## SPEC:"
	start := strings.Index(prompt, marker)
	if start < 0 {
		return ""
	}
	line := prompt[start+len(marker):]
	if end := strings.IndexByte(line, '\n'); end >= 0 {
		line = line[:end]
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func priorFindingsFromReviewPrompt(prompt string) string {
	const marker = "#### Prior Findings Checklist"
	start := strings.LastIndex(prompt, marker)
	if start < 0 {
		return ""
	}
	body := prompt[start+len(marker):]
	end := strings.Index(body, "\nRespond with:")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(body[:end])
}

func isSpecReviewJudgeResponse(response orchestra.ProviderResponse) bool {
	return response.Role == "judge" || strings.HasSuffix(strings.TrimSpace(response.Provider), specReviewJudgeSuffix)
}
