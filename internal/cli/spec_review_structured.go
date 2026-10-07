package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

type specReviewStructuredOutcome struct {
	resp   orchestra.ProviderResponse
	failed *orchestra.FailedProvider
}

func runStructuredSpecReviewOrchestra(ctx context.Context, cfg orchestra.OrchestraConfig) (*orchestra.OrchestraResult, error) {
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("spec review: no providers configured")
	}

	// Reviewers run in parallel before the sequential judge, so the watchdog
	// includes the longest reviewer-attempt budget plus the judge budget.
	watchdogSeconds := specReviewWatchdogForConfig(cfg)
	ctx, cancel := structuredSpecReviewContext(ctx, watchdogSeconds)
	defer cancel()

	schema := &orchestra.SchemaBuilder{}
	schemaPath, cleanup, err := schema.WriteToFile("reviewer")
	if err != nil {
		return nil, fmt.Errorf("spec review: reviewer schema: %w", err)
	}
	defer cleanup()

	embeddedSchema, err := schema.EmbedInPrompt("reviewer")
	if err != nil {
		return nil, fmt.Errorf("spec review: embed reviewer schema: %w", err)
	}

	backend := specReviewBackendFactory(cfg)
	if backend == nil {
		return nil, fmt.Errorf("spec review: no execution backend configured")
	}

	parser := &orchestra.OutputParser{}
	start := time.Now()

	mode := structuredReviewExecutionMode(backend)
	fmt.Fprintf(os.Stderr, "SPEC 리뷰 백엔드: %s (providers=%d, watchdog=%s, mode=%s)\n",
		backend.Name(), len(cfg.Providers), formatStructuredReviewTimeout(watchdogSeconds), mode)

	results := runStructuredSpecReviewProviders(ctx, cfg, backend, parser, schemaPath, embeddedSchema, mode)

	judgeEnabled := strings.TrimSpace(cfg.JudgeProvider) != "" && !cfg.NoJudge
	responses := make([]orchestra.ProviderResponse, 0, len(results))
	var reviewerResponses []orchestra.ProviderResponse
	if judgeEnabled {
		reviewerResponses = make([]orchestra.ProviderResponse, 0, len(results))
	}
	failed := make([]orchestra.FailedProvider, 0)
	for _, result := range results {
		responses = append(responses, result.resp)
		if result.failed != nil {
			failed = append(failed, *result.failed)
		} else if judgeEnabled {
			reviewerResponses = append(reviewerResponses, result.resp)
		}
	}
	if judgeEnabled {
		judgeResponse, judgeFailure := runStructuredSpecReviewJudge(ctx, cfg, backend, reviewerResponses)
		if judgeResponse != nil {
			responses = append(responses, *judgeResponse)
		}
		if judgeFailure != nil {
			failed = append(failed, *judgeFailure)
		}
	}

	return orchestra.FinalizeOrchestrationResult(&orchestra.OrchestraResult{
		Strategy:        cfg.Strategy,
		Responses:       responses,
		Duration:        time.Since(start),
		Summary:         fmt.Sprintf("structured spec review: %d providers", len(results)),
		FailedProviders: failed,
	}, cfg), nil
}

// ownStructuredReviewHookSession created the pane hook session of a review.
// Spec review runs no pane, so nothing calls it (SPEC-PANERM-001); it is group
// C code that W2 (T9) deletes.
//
//nolint:unused // kept until the SPEC-PANERM-001 W2 deletion merge (T9)
func ownStructuredReviewHookSession(cfg orchestra.OrchestraConfig) func() {
	if !cfg.HookMode || strings.TrimSpace(cfg.SessionID) == "" {
		return func() {}
	}
	hookSession, err := orchestra.NewHookSession(cfg.SessionID)
	if err != nil {
		return func() {}
	}
	hookSession.ApplyProviderHooks(cfg.Providers)
	return hookSession.Cleanup
}

func buildStructuredSpecReviewPrompt(basePrompt, schemaJSON string, inlineSchema bool) string {
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n### Structured Response Contract\n\n")
	sb.WriteString("Return ONLY one valid JSON object. The first non-whitespace character must be `{` and the last must be `}`. Do NOT return progress notes, markdown fences, bullets, headings, or prose before/after the JSON.\n")
	sb.WriteString("If you are blocked or the scope is too large, still return valid JSON with `verdict: \"REVISE\"` and describe the blocker in `summary` plus at least one finding.\n")
	if isVerifyReviewPrompt(basePrompt) {
		sb.WriteString("In verify mode, scope the review to the prior findings and checklist statuses requested above.\n")
		sb.WriteString("Do not perform a fresh full-SPEC discovery pass. Add new findings only for critical/security regressions or behavior newly broken by the revision.\n")
	} else {
		sb.WriteString("Review the full SPEC in one pass and include all actionable findings together; do not drip-feed optional suggestions across revisions.\n")
	}
	sb.WriteString("Use `severity: \"suggestion\"` only for advisory feedback. Suggestion-only feedback must not be the reason for `verdict: \"REVISE\"`.\n")
	sb.WriteString("Use these fields:\n")
	sb.WriteString("- `verdict`: `PASS`, `REVISE`, or `REJECT`\n")
	sb.WriteString("- `summary`: concise explanation of the overall verdict\n")
	sb.WriteString("- `findings`: array of `{severity, category, scope_ref, location, description, suggestion}`\n")
	sb.WriteString("- `checklist`: array of `{id, status, reason}` where `status` is `PASS` or `FAIL`\n")
	sb.WriteString("- `finding_statuses`: array of `{id, status, reason}` where `status` is `open`, `resolved`, or `regressed`\n")
	if inlineSchema {
		sb.WriteString("\nRequired JSON schema:\n```json\n")
		sb.WriteString(schemaJSON)
		sb.WriteString("\n```\n")
	}
	return sb.String()
}

func isVerifyReviewPrompt(prompt string) bool {
	return strings.Contains(prompt, "Instructions (Verify Mode)")
}

func malformedStructuredOutcome(provider string, err error, sourceResp *orchestra.ProviderResponse, backendName string) specReviewStructuredOutcome {
	description := structuredFailureDescription(err, sourceResp)
	response := orchestra.ProviderResponse{
		Provider:        provider,
		Output:          synthesizeMalformedReviewJSON(provider, description),
		Error:           description,
		ExecutedBackend: backendName,
	}
	failed := &orchestra.FailedProvider{
		Name:            provider,
		Error:           description,
		FailureClass:    structuredFailureClass(err),
		NextRemediation: structuredFailureRemediation(err, provider),
		CollectionMode:  structuredFailureCollectionMode(backendName, sourceResp),
	}
	if sourceResp != nil {
		if strings.TrimSpace(sourceResp.ExecutedBackend) != "" {
			response.ExecutedBackend = sourceResp.ExecutedBackend
			failed.CollectionMode = structuredFailureCollectionMode(sourceResp.ExecutedBackend, sourceResp)
		}
		failed.StderrPreview = truncateStructuredReviewError(sourceResp.Error, 240)
		failed.OutputPreview = truncateStructuredReviewError(sourceResp.Output, 240)
		// The receipt judges the sandbox from the recorded launch; a provider
		// that never started has none and claims no sandbox.
		response.Execution = sourceResp.Execution
		failed.Execution = sourceResp.Execution
	}
	// The receipt projects FailedProvider over the response for a failed
	// reviewer, so the observed backend must travel on both.
	failed.ExecutedBackend = response.ExecutedBackend
	return specReviewStructuredOutcome{resp: response, failed: failed}
}

func structuredFailureCollectionMode(backendName string, sourceResp *orchestra.ProviderResponse) string {
	if sourceResp != nil && strings.TrimSpace(sourceResp.Receipt) != "" {
		if strings.EqualFold(strings.TrimSpace(backendName), "pane") {
			return "pane_receipt"
		}
	}
	switch strings.TrimSpace(backendName) {
	case "pane":
		return "pane"
	case "subprocess":
		return "subprocess_stdout"
	case "":
		return "unknown"
	default:
		return backendName
	}
}

func synthesizeMalformedReviewJSON(provider, description string) string {
	payload := orchestra.ReviewerOutput{
		Verdict: "REVISE",
		Summary: fmt.Sprintf("Malformed or incomplete review output from %s", provider),
		Findings: []orchestra.Finding{{
			Severity:    "major",
			Category:    "completeness",
			ScopeRef:    "provider:" + provider,
			Location:    "provider:" + provider,
			Description: truncateStructuredReviewError(description, 240),
			Suggestion:  structuredFailureRemediationText(description, provider),
		}},
	}
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return `{"verdict":"REVISE","summary":"Malformed review output","findings":[{"severity":"major","category":"completeness","scope_ref":"provider:unknown","location":"provider:unknown","description":"failed to serialize malformed review result","suggestion":"Retry the provider."}]}`
	}
	return string(data)
}

func structuredFailureDescription(err error, resp *orchestra.ProviderResponse) string {
	if err == nil {
		return "unknown provider failure"
	}
	parts := []string{err.Error()}
	if resp == nil {
		return strings.Join(parts, "; ")
	}
	if resp.Duration > 0 {
		parts = append(parts, "duration "+resp.Duration.String())
	}
	if strings.TrimSpace(resp.Error) != "" {
		parts = append(parts, "stderr: "+truncateStructuredReviewError(resp.Error, 160))
	}
	if strings.TrimSpace(resp.Output) != "" {
		parts = append(parts, "stdout preview: "+truncateStructuredReviewError(resp.Output, 160))
	}
	return strings.Join(parts, "; ")
}

func specReviewTimeout(provider orchestra.ProviderConfig, fallbackSeconds int) time.Duration {
	if provider.ExecutionTimeout > 0 {
		return provider.ExecutionTimeout
	}
	if fallbackSeconds > 0 {
		return time.Duration(fallbackSeconds) * time.Second
	}
	return 120 * time.Second
}

// specReviewWatchdogSeconds derives the overall review deadline (in seconds)
// from the longest provider attempt budget. Structured review providers run in
// parallel, and each provider gets its own child context, so a hung provider
// should be bounded by its own timeout instead of forcing a sum-of-providers
// deadline. Slack covers schema build, prompt assembly, pane split/paste/read,
// cleanup, and goroutine fan-in overhead.
func specReviewWatchdogSeconds(providers []orchestra.ProviderConfig, fallbackSeconds int) int {
	if len(providers) == 0 {
		return fallbackSeconds
	}
	var longest time.Duration
	for _, provider := range providers {
		if budget := specReviewAttemptTimeoutBudget(provider, fallbackSeconds); budget > longest {
			longest = budget
		}
	}
	longest += 30*time.Second + time.Duration(len(providers))*10*time.Second
	return int(longest / time.Second)
}
