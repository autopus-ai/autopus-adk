package codex

import "github.com/insajin/autopus-adk/pkg/config"

// normalizeCodexExtendedSkill is the hook for a Codex native rewrite of a
// shared skill body. The verification harness ships no skill that needs one:
// the collaboration and PRD skills that did were retired with it. cfg stays in
// the signature so a future rewrite can read the project's settings.
func normalizeCodexExtendedSkill(_ string, body string, _ *config.HarnessConfig) string {
	return body
}
