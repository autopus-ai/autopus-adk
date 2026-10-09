package content

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/templates"
)

// supervisorWorktreeIsolationTemplate is the worktree-isolation body for the
// surfaces whose subagents have no isolated-workspace option: Gemini CLI and
// Antigravity, and OpenCode. The shared source tells a Claude Code Agent() call
// to pass isolation: "worktree" so the runtime creates the worktree; there the
// supervisor runs the worktree lifecycle itself with git. Codex and OMP replace
// the body with their own native rewrites in their adapters.
const supervisorWorktreeIsolationTemplate = "shared/supervisor-worktree-isolation.md.tmpl"

// nativeSkillBodyTemplates maps a canonical skill and a normalized platform to
// the hand-authored body that replaces the shared source on that platform.
var nativeSkillBodyTemplates = map[string]map[string]string{
	"worktree-isolation": {
		"gemini":   supervisorWorktreeIsolationTemplate,
		"opencode": supervisorWorktreeIsolationTemplate,
	},
}

// nativeSkillBody returns the platform-native body of a skill and whether one
// exists, trimmed like a parsed shared body. The body still goes through the
// platform reference rewrites.
func nativeSkillBody(name, platform string) (string, bool, error) {
	path, ok := nativeSkillBodyTemplates[name][normalizePlatform(platform)]
	if !ok {
		return "", false, nil
	}
	data, err := templates.FS.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read native %s body for %s: %w", name, platform, err)
	}
	return strings.TrimSpace(string(data)), true, nil
}
