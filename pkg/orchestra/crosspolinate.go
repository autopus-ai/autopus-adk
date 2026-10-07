package orchestra

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// ProviderResult holds a single provider's output from one round.
type ProviderResult struct {
	Provider        string // real provider name
	Output          string // raw output text
	Response        ProviderResponse
	Usage           []telemetry.UsageEnvelope
	UsageCapability UsageCapability
}

// CrossPollinateBuilder anonymizes provider outputs and builds
// cross-pollination prompts for the next debate round.
type CrossPollinateBuilder struct {
	identityMap map[string]string // alias -> real name
	reverseMap  map[string]string // real name -> alias
}

// NewCrossPollinateBuilder creates a builder with the given provider names.
// Aliases are assigned in order: "Debater A", "Debater B", "Debater C", etc.
func NewCrossPollinateBuilder(providerNames []string) *CrossPollinateBuilder {
	im := make(map[string]string, len(providerNames))
	rm := make(map[string]string, len(providerNames))
	for i, name := range providerNames {
		alias := fmt.Sprintf("Debater %c", 'A'+rune(i))
		im[alias] = name
		rm[name] = alias
	}
	return &CrossPollinateBuilder{identityMap: im, reverseMap: rm}
}

// Anonymize converts provider results to anonymized results.
// ICE scores are stripped and outputs are capped to the Round 2 prompt budget.
func (cpb *CrossPollinateBuilder) Anonymize(results []ProviderResult) []PreviousResult {
	cleanedOutputs := make([]string, len(results))
	for i, r := range results {
		cleanedOutputs[i] = stripICEScores(r.Output)
	}
	cappedOutputs := capPromptSections(cleanedOutputs, rebuttalPromptTotalTokens, rebuttalPromptPerParticipant)

	out := make([]PreviousResult, 0, len(results))
	for i, r := range results {
		alias, ok := cpb.reverseMap[r.Provider]
		if !ok {
			alias = r.Provider // fallback: use original name
		}
		out = append(out, PreviousResult{
			Alias:  alias,
			Output: cappedOutputs[i],
		})
	}
	return out
}

// AnonymizeForJudge converts multi-round results to judge-ready format while
// bounding per-participant context for the final judge prompt.
func (cpb *CrossPollinateBuilder) AnonymizeForJudge(round1, round2 []ProviderResult) []JudgeResult {
	r1Map := make(map[string]string, len(round1))
	for _, r := range round1 {
		r1Map[r.Provider] = stripICEScores(r.Output)
	}
	r2Map := make(map[string]string, len(round2))
	for _, r := range round2 {
		r2Map[r.Provider] = stripICEScores(r.Output)
	}

	out := make([]JudgeResult, 0, len(cpb.identityMap))
	for alias, realName := range cpb.identityMap {
		out = append(out, JudgeResult{
			Alias:  alias,
			Round1: r1Map[realName],
			Round2: r2Map[realName],
		})
	}
	return capJudgeResults(out)
}

// IdentityMap returns alias -> real provider name mapping for de-anonymization.
func (cpb *CrossPollinateBuilder) IdentityMap() map[string]string {
	cp := make(map[string]string, len(cpb.identityMap))
	for k, v := range cpb.identityMap {
		cp[k] = v
	}
	return cp
}

// iceTableHeaderRe matches ICE scoring table headers (various formats).
var iceTableHeaderRe = regexp.MustCompile(`(?i)(ICE\s*(Score|스코어)|통합\s*ICE|Top\s*\d+\s*(통합|아이디어)|Judge.*Merge|Judge.*Integration|Impact.*Confidence.*Ease)`)

// iceScoreLineRe matches standalone ICE score lines like "ICE: 5.12" or "Score: 432".
var iceScoreLineRe = regexp.MustCompile(`(?i)^\s*(ICE|Score)\s*[:=]\s*[\d.]+\s*$`)

// stripICEScores removes self-assigned ICE scoring sections from provider output.
// This prevents confidence cascade where later rounds blindly adopt earlier scores.
func stripICEScores(s string) string {
	lines := strings.Split(s, "\n")
	filtered := make([]string, 0, len(lines))
	inICETable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Detect ICE table headers and skip until next non-table line
		if iceTableHeaderRe.MatchString(trimmed) {
			inICETable = true
			continue
		}
		if inICETable {
			// Stay in ICE table while lines look like table rows
			if strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "+-") || strings.HasPrefix(trimmed, "┌") || strings.HasPrefix(trimmed, "├") || strings.HasPrefix(trimmed, "└") || strings.HasPrefix(trimmed, "│") || trimmed == "" {
				continue
			}
			inICETable = false
		}
		// Skip standalone ICE score lines
		if iceScoreLineRe.MatchString(trimmed) {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}
