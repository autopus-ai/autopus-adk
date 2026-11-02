package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

const specReviewJSONRepromptAttempts = 1

func specReviewAttemptTimeoutBudget(provider orchestra.ProviderConfig, fallbackSeconds int) time.Duration {
	return specReviewTimeout(provider, fallbackSeconds) * time.Duration(1+specReviewJSONRepromptAttempts)
}

func repromptStructuredReviewerJSON(
	ctx context.Context,
	backend orchestra.ExecutionBackend,
	parser *orchestra.OutputParser,
	req orchestra.ProviderRequest,
	initialResp *orchestra.ProviderResponse,
	parseErr error,
) (*orchestra.ProviderResponse, error) {
	if specReviewJSONRepromptAttempts <= 0 {
		return nil, fmt.Errorf("reprompt disabled")
	}
	if backend == nil {
		return nil, fmt.Errorf("reprompt backend unavailable")
	}

	repromptReq := req
	repromptReq.Prompt = buildStructuredSpecReviewReprompt(req.Prompt, initialResp, parseErr)
	fmt.Fprintf(os.Stderr, "SPEC 리뷰 provider JSON 재요청: %s (backend=%s)\n", req.Provider, backend.Name())

	resp, execErr := backend.Execute(withSpecReviewCallReason(ctx, specReviewReasonJSONRepair), repromptReq)
	if execErr != nil {
		return resp, fmt.Errorf("reprompt execution failed: %w", execErr)
	}
	if resp == nil {
		return nil, fmt.Errorf("reprompt returned nil response")
	}
	if strings.TrimSpace(resp.ExecutedBackend) == "" {
		resp.ExecutedBackend = backend.Name()
	}
	if resp.TimedOut {
		return resp, fmt.Errorf("reprompt timed out after %s", req.Timeout)
	}
	if resp.EmptyOutput || strings.TrimSpace(resp.Output) == "" {
		return resp, fmt.Errorf("reprompt returned empty output")
	}
	if _, err := parser.ParseReviewer(resp.Output); err != nil {
		return resp, fmt.Errorf("reprompt invalid reviewer JSON: %w", err)
	}
	return resp, nil
}

func buildStructuredSpecReviewReprompt(originalPrompt string, initialResp *orchestra.ProviderResponse, parseErr error) string {
	var sb strings.Builder
	sb.WriteString("Your previous reviewer response was not valid JSON and could not be parsed.\n")
	sb.WriteString("Return exactly one JSON object for the same review task. Do not include markdown fences, headings, bullets, or prose outside the JSON object.\n")
	if parseErr != nil {
		sb.WriteString("Parser error: ")
		sb.WriteString(truncateStructuredReviewError(parseErr.Error(), 240))
		sb.WriteString("\n")
	}
	if initialResp != nil && strings.TrimSpace(initialResp.Output) != "" {
		sb.WriteString("Previous response preview: ")
		sb.WriteString(truncateStructuredReviewError(initialResp.Output, 240))
		sb.WriteString("\n")
	}
	sb.WriteString("\n### Original Review Request\n\n")
	sb.WriteString(originalPrompt)
	return sb.String()
}

func mergeStructuredRepromptFailureResponse(initialResp, retryResp *orchestra.ProviderResponse) *orchestra.ProviderResponse {
	if retryResp == nil {
		return initialResp
	}
	if initialResp == nil {
		return retryResp
	}
	merged := *retryResp
	initialOutput := strings.TrimSpace(initialResp.Output)
	retryOutput := strings.TrimSpace(retryResp.Output)
	if initialOutput != "" && retryOutput != "" {
		merged.Output = "initial response: " + truncateStructuredReviewError(initialOutput, 120) +
			"; reprompt response: " + truncateStructuredReviewError(retryOutput, 120)
	} else if retryOutput == "" {
		merged.Output = initialResp.Output
	}
	initialError := strings.TrimSpace(initialResp.Error)
	retryError := strings.TrimSpace(retryResp.Error)
	if initialError != "" && retryError != "" {
		merged.Error = "initial stderr: " + truncateStructuredReviewError(initialError, 120) +
			"; reprompt stderr: " + truncateStructuredReviewError(retryError, 120)
	} else if retryError == "" {
		merged.Error = initialResp.Error
	}
	return &merged
}
