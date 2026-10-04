package agentexec

import "strings"

// detectOrder is the preference when no target is named: the first CLI found
// on PATH wins. Claude leads because its edit mode stays inside acceptEdits
// without a sandbox flag; the rest follow the platform adapter order.
var detectOrder = []struct {
	target Target
	binary string
}{
	{TargetClaude, "claude"},
	{TargetCodex, "codex"},
	{TargetGemini, "agy"},
	{TargetOpenCode, "opencode"},
}

// DetectTarget picks the agent `auto qa go` uses when --agent is omitted.
// With AUTOPUS_QA_AGENT_ARGV set the override decides the binary, so the
// first target is returned without looking anything up.
func DetectTarget(lookPath func(string) (string, error), getenv func(string) string) (Target, error) {
	if getenv != nil && strings.TrimSpace(getenv(OverrideEnv)) != "" {
		return TargetClaude, nil
	}
	for _, candidate := range detectOrder {
		if _, err := lookPath(candidate.binary); err == nil {
			return candidate.target, nil
		}
	}
	return "", &SetupGapError{Code: CodeCLIMissing, Binary: "claude|codex|agy|opencode"}
}
