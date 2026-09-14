package cli

import (
	"fmt"
	"os"

	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/insajin/autopus-adk/pkg/spec"
)

var (
	specReviewBuildContextDelivery  = promptlayer.BuildContextDelivery
	specReviewVerifyContextDelivery = promptlayer.VerifyContextDeliveryForOptions
)

type specReviewContextDelivery struct {
	options promptlayer.ContextDeliveryOptions
}

func prepareSpecReviewContextDelivery(
	resolvedSpecDir string,
	providers []orchestra.ProviderConfig,
	opts specReviewOptions,
) (*specReviewContextDelivery, error) {
	if !requireCompleteGPTReviewDocuments(providers) {
		return nil, nil
	}
	scope, err := resolveSpecReviewContextScope(resolvedSpecDir)
	if err != nil {
		return nil, err
	}
	return &specReviewContextDelivery{options: promptlayer.ContextDeliveryOptions{
		Root: scope.projectRoot, Command: "review", SpecDir: scope.specDir,
		RequiredReferences:   append([]string(nil), opts.requiredDocuments...),
		ConditionalProfiles:  contextProfileNames(opts.conditionalProfiles),
		ExplicitArchitecture: true,
	}}, nil
}

func (delivery *specReviewContextDelivery) buildVerified() (promptlayer.ContextDeliveryResult, error) {
	if delivery == nil {
		return promptlayer.ContextDeliveryResult{}, fmt.Errorf("review context delivery is not configured")
	}
	receipt, err := specReviewBuildContextDelivery(delivery.options)
	if err != nil {
		return promptlayer.ContextDeliveryResult{}, fmt.Errorf("build review context delivery: %w", err)
	}
	if err := specReviewVerifyContextDelivery(delivery.options, receipt); err != nil {
		return promptlayer.ContextDeliveryResult{}, fmt.Errorf("verify review context delivery: %w", err)
	}
	return receipt, nil
}

func buildSpecReviewProviderPrompt(
	p specReviewLoopParams,
	doc *spec.SpecDocument,
	priorFindings []spec.ReviewFinding,
	revision int,
) (string, []spec.ReviewFinding, error) {
	staticFindings, staticErr := spec.RunSpecContractAnalysis(p.specDir)
	if staticErr != nil {
		fmt.Fprintf(os.Stderr, "경고: SPEC static contract analysis 실패: %v\n", staticErr)
	}
	opts := buildPromptOpts(priorFindings, revision, p.specDir, p.gate)
	if opts.Mode == spec.ReviewModeDiscover {
		opts.StaticFindings = staticFindings
	}
	if p.contextDelivery == nil {
		prompt, err := spec.BuildReviewPromptChecked(doc, p.codeContext, opts)
		return prompt, staticFindings, err
	}
	receipt, err := p.contextDelivery.buildVerified()
	if err != nil {
		return "", staticFindings, err
	}
	prompt, err := spec.BuildReviewPromptFromContextDeliveryChecked(doc, p.codeContext, opts, receipt)
	return prompt, staticFindings, err
}
