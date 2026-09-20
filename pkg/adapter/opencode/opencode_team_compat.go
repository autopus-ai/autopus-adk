package opencode

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// sanitizeUnsupportedClaudeTeamMappings fail-closes OpenCode surfaces that
// accidentally inherit Claude-only lifecycle names from shared documentation.
func sanitizeUnsupportedClaudeTeamMappings(files []adapter.FileMapping) []adapter.FileMapping {
	sanitized := make([]adapter.FileMapping, len(files))
	for i, file := range files {
		body := string(file.Content)
		if file.TargetPath == "AGENTS.md" {
			body = markerRe.ReplaceAllStringFunc(body, sanitizeUnsupportedClaudeTeamSurface)
		} else {
			body = sanitizeUnsupportedClaudeTeamSurface(body)
		}
		file.Content = []byte(body)
		file.Checksum = adapter.Checksum(string(file.Content))
		sanitized[i] = file
	}
	return sanitized
}

func sanitizeUnsupportedClaudeTeamSurface(body string) string {
	return strings.NewReplacer(
		"TeamCreate", "unsupported_claude_team_create",
		"TeamDelete", "unsupported_claude_team_delete",
		"SendMessage", "unsupported_claude_team_message",
		"agent-teams/SKILL.md", "unsupported-team-mode.md",
		"skills/autopus/agent-teams.md", "unsupported-team-mode.md",
	).Replace(body)
}
