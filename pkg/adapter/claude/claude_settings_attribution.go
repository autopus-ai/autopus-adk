package claude

import "github.com/insajin/autopus-adk/pkg/config"

// coAuthoredByTrailer is the trailer Claude Code appends to every commit
// unless its attribution setting says otherwise.
const coAuthoredByTrailer = "Co-Authored-By"

// commitAttributionForbidden reports whether the project's Lore policy rejects
// the trailer Claude Code would otherwise add to each commit. Without this the
// commit-msg hook would refuse commits the agent produces by default, so the
// agent and the gate have to agree on the same list.
func commitAttributionForbidden(cfg *config.HarnessConfig) bool {
	if cfg == nil || !cfg.Lore.Enabled {
		return false
	}
	return cfg.Lore.ForbidsTrailer(coAuthoredByTrailer)
}

// projectSuppressedCommitAttribution clears Claude Code's commit attribution
// while keeping every other attribution field the user set, such as pr or
// sessionUrl. A value that already hides everything (false) is left as is, and
// a value of an unexpected shape is replaced because Claude Code would reject
// it anyway.
func projectSuppressedCommitAttribution(current any) any {
	switch value := current.(type) {
	case bool:
		if !value {
			return value
		}
	case map[string]any:
		projected := make(map[string]any, len(value)+1)
		for key, field := range value {
			projected[key] = field
		}
		projected["commit"] = ""
		return projected
	}
	return map[string]any{"commit": ""}
}
