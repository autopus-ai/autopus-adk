package orchestra

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// runDebate executes the full debate flow:
// Phase 1 (parallel arguments) → optional Phase 2 (rebuttal) → optional judgment.
// Returns final responses, per-round history, and Round 1 failed providers so
// callers can surface failures (OrchestraResult.FailedProviders, yield JSON).
func runDebate(ctx context.Context, cfg OrchestraConfig) ([]ProviderResponse, [][]ProviderResponse, []FailedProvider, error) {
	// Phase 1: all debaters respond to original prompt in parallel
	round1Responses, round1Failed, err := runParallel(ctx, cfg)
	if err != nil {
		return nil, nil, round1Failed, err
	}

	roundHistory := [][]ProviderResponse{round1Responses}
	responses := round1Responses

	// Phase 2 (optional): rebuttal round when DebateRounds >= 2
	rounds := cfg.DebateRounds
	if rounds <= 0 {
		rounds = 1
	}
	if rounds >= 2 && len(responses) >= 2 {
		rebuttalResps, rebuttalFailed, rebuttalErr := runRebuttalRound(ctx, cfg, responses)
		// Always merge rebuttal failures even if rebuttal partially succeeded so
		// callers (OrchestraResult.FailedProviders, yield JSON) see the full picture.
		round1Failed = append(round1Failed, rebuttalFailed...)
		if rebuttalErr == nil && len(rebuttalResps) > 0 {
			roundHistory = append(roundHistory, rebuttalResps)
			responses = rebuttalResps
		}
	}

	// Phase 3 (optional): judge verdict. A required judge failure remains visible
	// in the same failed-provider stream as participant failures.
	// @AX:NOTE: [AUTO] failed-judge freshness evidence rides the last participant response so finalization can still project it
	if cfg.JudgeProvider != "" && !cfg.NoJudge {
		judgeResp, judgeFailure, freshJudgeSession := executeDebateJudge(ctx, cfg, responses)
		if judgeResp != nil {
			responses = append(responses, *judgeResp)
		} else if len(responses) > 0 {
			responses[len(responses)-1].freshJudgeSession = freshJudgeSession
		}
		if judgeFailure != nil {
			round1Failed = append(round1Failed, *judgeFailure)
		}
	}

	return responses, roundHistory, round1Failed, nil
}

// runRebuttalRound executes one rebuttal round for each debater.
// Each debater receives the original prompt plus all other debaters' responses.
// Returns successful responses and failed providers so callers can surface
// rebuttal-phase failures alongside Round 1 failures.
func runRebuttalRound(ctx context.Context, cfg OrchestraConfig, prevResponses []ProviderResponse) ([]ProviderResponse, []FailedProvider, error) {
	rebuttalResults := make([]providerResult, len(cfg.Providers))
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
		go func(idx int, provider ProviderConfig) {
			defer wg.Done()
			// Collect other debaters' responses (exclude current provider)
			var others []ProviderResponse
			for _, r := range prevResponses {
				if r.Provider != provider.Name {
					others = append(others, r)
				}
			}
			rebuttalPrompt := buildRebuttalPrompt(cfg.Prompt, others, 2)
			resp, err := runConfiguredProvider(
				ctx, cfg, provider, rebuttalPrompt, "debater_r2", 2, progress,
			)
			rebuttalResults[idx] = providerResult{resp: resp, err: err, idx: idx}
		}(i, p)
	}
	wg.Wait()

	var responses []ProviderResponse
	var failed []FailedProvider
	for i, r := range rebuttalResults {
		name := cfg.Providers[i].Name
		switch {
		case r.err != nil:
			fp := buildFailedProvider(cfg.Providers[i], r.resp, r.err, cfg.TimeoutSeconds)
			fp.Role = "debater_r2"
			fp.Attempt = 2
			fp.ExecutedBackend = "subprocess"
			fp.Name = name
			fp.Error = fmt.Sprintf("rebuttal: %s", fp.Error)
			failed = append(failed, fp)
		case r.resp != nil && r.resp.TimedOut:
			fp := buildFailedProvider(cfg.Providers[i], r.resp, nil, cfg.TimeoutSeconds)
			fp.Role = "debater_r2"
			fp.Attempt = 2
			fp.ExecutedBackend = r.resp.ExecutedBackend
			fp.Error = "rebuttal " + fp.Error
			failed = append(failed, fp)
		case r.resp != nil && r.resp.EmptyOutput:
			fp := buildFailedProvider(cfg.Providers[i], r.resp, nil, cfg.TimeoutSeconds)
			fp.Role = "debater_r2"
			fp.Attempt = 2
			fp.ExecutedBackend = r.resp.ExecutedBackend
			fp.Error = "rebuttal " + fp.Error
			failed = append(failed, fp)
		default:
			responses = append(responses, *r.resp)
		}
	}
	if len(responses) == 0 {
		if len(rebuttalResults) > 0 && rebuttalResults[0].err != nil {
			return nil, failed, rebuttalResults[0].err
		}
		return nil, failed, fmt.Errorf("rebuttal round: no providers produced output")
	}
	return responses, failed, nil
}

// topicIsolationInstruction prevents providers from reading project files during debate.
// @AX:NOTE [AUTO] REQ-2 hardcoded prompt prefix — injected by executeRound caller, not by buildRebuttalPrompt
const topicIsolationInstruction = "IMPORTANT: Discuss ONLY the topic below. Do NOT read, reference, or analyze any existing files in the project directory. Focus exclusively on the given discussion topic.\n\n"

// buildRebuttalPrompt creates a cross-pollination prompt with anonymized participant outputs.
// Uses the Acknowledge/Integrate/Risk 3-step structure for structured revision.
// ICE scores from Round 1 are stripped to prevent confidence cascade.
// Peer outputs are capped with an approximate token budget so Round 2 does not
// re-inject full brainstorm transcripts back into every provider context.
func buildRebuttalPrompt(original string, otherResponses []ProviderResponse, round int) string {
	var sb strings.Builder
	sb.WriteString(original)
	fmt.Fprintf(&sb, "\n\n---\n\n# Round %d: Cross-Pollination\n\n", round)
	sb.WriteString("Other participants' ideas are shown below (anonymized, ICE scores removed).\n\n")

	cleanedOutputs := make([]string, len(otherResponses))
	for i, r := range otherResponses {
		cleanedOutputs[i] = stripICEScores(r.Output)
	}
	cappedOutputs := capPromptSections(cleanedOutputs, rebuttalPromptTotalTokens, rebuttalPromptPerParticipant)
	sentinel := newDebateSentinel(cappedOutputs...)

	writeDebateFenceNotice(&sb, rebuttalFenceSecurityNote)
	for i := range otherResponses {
		alias := fmt.Sprintf("Participant %c", 'A'+rune(i))
		writeDebateFenceBlock(&sb, "##", alias, cappedOutputs[i], sentinel)
	}

	sb.WriteString(`Respond in exactly 3 steps:

### Step 1: Acknowledge
Identify the 2-3 strongest points from other participants.
For each, explain **why** it is strong with specific evidence.
Do NOT blindly praise — only acknowledge points with real merit.

### Step 2: Integrate
Your Round 1 core ideas MUST be preserved. Do not abandon them.
Enhance your ideas by incorporating the strongest elements from others.
Describe the integrated proposal with concrete details.

### Step 3: Risk Assessment
Identify 2-3 remaining weaknesses, risks, or implementation barriers
in the integrated proposal. Be specific about assumptions and dependencies.
`)
	return sb.String()
}

// buildJudgmentPrompt creates the judge's synthesis prompt with anonymized debate results.
// Includes structured ICE scoring instructions for consensus-based evaluation
// while capping per-participant context to keep judge prompts bounded.
func buildJudgmentPrompt(topic string, arguments []ProviderResponse) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Role: Final Judge\n\nYou are the final judge for a multi-analyst debate on the topic below.\nParticipant identities are anonymized. Judge purely on content quality.\n\n## Topic\n\n%s\n\n## Debate Results (Anonymized)\n\n", topic)

	cleanedOutputs := make([]string, len(arguments))
	for i, r := range arguments {
		cleanedOutputs[i] = stripICEScores(r.Output)
	}
	cappedOutputs := capPromptSections(cleanedOutputs, judgePromptTotalTokens, judgePromptPerParticipant)
	sentinel := newDebateSentinel(cappedOutputs...)

	writeDebateFenceNotice(&sb, judgeFenceSecurityNote)
	for i := range arguments {
		alias := fmt.Sprintf("Participant %c", 'A'+rune(i))
		writeDebateFenceBlock(&sb, "###", alias, cappedOutputs[i], sentinel)
	}

	sb.WriteString(`## Judging Instructions

### 1. Consensus Areas
Extract ideas that 2+ participants converged on. Convergence = high confidence signal.
For each: what is the shared idea, which participants agreed, why it matters.

### 2. Unique Insights
Identify ideas proposed by only 1 participant that others did NOT integrate.
These are potentially innovative but also potentially flawed.

### 3. Cross-Risks
Compile risks that 2+ participants independently flagged.
Shared risk identification = likely a real threat.
For each: describe the risk, severity (high/medium/low).

### 4. Top Ideas Ranking (ICE Score)
Select the top 5 ideas and score each:
- **Impact** (1-10): real-world value to the project
- **Confidence** (1-10): consensus level — more participants agreeing = higher
- **Ease** (1-10): implementation feasibility
- **Score** = Impact × Confidence × Ease / 100

### 5. Recommendation
Write a 2-3 sentence actionable recommendation.
`)
	return sb.String()
}

// buildDebateMerged formats the debate result and builds the summary.
// If the last response is from the judge, it is noted in the summary.
func buildDebateMerged(responses []ProviderResponse, cfg OrchestraConfig) (string, string) {
	if len(responses) == 0 {
		return "", "토론 결과 없음"
	}

	judgeVerdict := ""
	judgePresent := false

	// Check if the last response is from the judge
	last := responses[len(responses)-1]
	if cfg.JudgeProvider != "" && strings.HasPrefix(last.Provider, cfg.JudgeProvider) {
		judgeVerdict = last.Output
		judgePresent = true
	}

	merged := FormatDebate(responses)

	judgeLabel := cfg.JudgeProvider
	if judgeLabel == "" {
		judgeLabel = "없음"
	}

	var summary string
	if judgePresent {
		preview := judgeVerdict
		if len(preview) > 50 {
			preview = preview[:50]
		}
		summary = fmt.Sprintf("토론 완료, 판정: %s (verdict: %s)", judgeLabel, preview)
	} else if cfg.JudgeProvider != "" && cfg.NoJudge {
		summary = fmt.Sprintf("토론 완료, 판정 생략: %s", judgeLabel)
	} else if cfg.JudgeProvider != "" {
		summary = fmt.Sprintf("토론 완료, 필수 판정 실패: %s", judgeLabel)
	} else {
		summary = fmt.Sprintf("토론 완료, 판정: %s", judgeLabel)
	}

	return merged, summary
}

// findOrBuildJudgeConfig finds the judge's ProviderConfig from cfg.Providers,
// or creates a default one with Name and Binary both set to JudgeProvider.
func findOrBuildJudgeConfig(cfg OrchestraConfig) ProviderConfig {
	if cfg.JudgeConfig != nil && cfg.JudgeConfig.Name != "" {
		return *cfg.JudgeConfig
	}
	for _, p := range cfg.Providers {
		if p.Name == cfg.JudgeProvider {
			return p
		}
	}
	return ProviderConfig{
		Name:   cfg.JudgeProvider,
		Binary: cfg.JudgeProvider,
	}
}
