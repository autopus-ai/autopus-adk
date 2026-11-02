package cli

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// specReviewStructuredPrompt is the structured response wrapper every
// reviewer request goes through. The contract digest wraps its probe prompts
// with the same function, so a wrapper change is a contract change; tests
// replace it to simulate one (SPEC-REVIEWREUSE-001 REQ-RR-REUSE-01).
var specReviewStructuredPrompt = buildStructuredSpecReviewPrompt

// specReviewContractDigest digests the review contract. It is computed for
// every fingerprint rather than once per process, so a contract that changes
// between two invocations of the same process is still seen.
func specReviewContractDigest() string {
	return spec.SHA256Hex([]byte(specReviewContractText()))
}

// specReviewContractText is every request text a review sends, built for
// fixed probe data: each base prompt variant (discover and verify with inline
// and with complete documents, and the delta input) through the production
// structured wrapper with and without the inline reviewer schema, the judge
// prompt in both modes, and the JSON repair request.
func specReviewContractText() string {
	schema := &orchestra.SchemaBuilder{}
	reviewer, _ := schema.EmbedInPrompt("reviewer")
	judgeSchema, _ := schema.EmbedInPrompt("review_judge")
	var parts []string
	for _, variant := range spec.ReviewPromptContractVariants() {
		parts = append(parts, specReviewStructuredPrompt(variant, reviewer, true), specReviewStructuredPrompt(variant, reviewer, false))
	}
	if builder, err := orchestra.NewPromptBuilder(); err == nil {
		participants, _ := orchestra.AnonymizeReviewParticipants([]orchestra.ProviderResponse{{Provider: "probe", Output: "{}"}})
		for _, mode := range []string{"discover", "verify"} {
			judge, _ := builder.BuildReviewJudge(orchestra.ReviewJudgeData{SpecID: "SPEC-PROBE-001", Mode: mode,
				Participants: participants, PriorFindings: "- F-001 [major] REQ-001: probe", SchemaJSON: judgeSchema})
			parts = append(parts, judge)
		}
	}
	parts = append(parts, buildStructuredSpecReviewReprompt("probe prompt",
		&orchestra.ProviderResponse{Output: "probe output"}, errors.New("probe parse error")))
	return strings.Join(parts, "\x00")
}

// specReviewConfigDigest digests the effective configuration: the file that
// was actually loaded, wherever it lives (an external --config included),
// after migration, inheritance, and flag overrides. A configuration outside
// the repository never shows in the git component, so it is a component of
// its own (REQ-RR-REUSE-01).
func specReviewConfigDigest(cfg *config.HarnessConfig) string {
	data, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return spec.SHA256Hex(data)
}
