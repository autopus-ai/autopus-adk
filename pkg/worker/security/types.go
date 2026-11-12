// Package security provides security policy types and validation for worker task execution.
package security

import (
	"regexp"
	"strings"
)

// SecurityPolicy defines extended security constraints for task execution.
// This extends the basic a2a.SecurityPolicy with command validation and directory restrictions.
//
// What the legacy `auto worker` loop enforces: TimeoutSec bounds a task's
// execution, and a Codex task runs in Codex's own OS sandbox. A Claude task
// runs with permission prompts bypassed and no tool-call hook, so
// AllowedCommands, DeniedPatterns, and AllowedDirs are checked only when
// ValidateCommand is called, for example by `auto worker validate`; they do
// not confine the tools a provider runs. The A2A cache does not carry those
// fields either, so a cached policy holds no command allowlist.
type SecurityPolicy struct {
	AllowNetwork    bool     `json:"allow_network"`
	AllowFS         bool     `json:"allow_fs"`
	AllowedPaths    []string `json:"allowed_paths,omitempty"`
	AllowedCommands []string `json:"allowed_commands,omitempty"`
	DeniedPatterns  []string `json:"denied_patterns,omitempty"`
	AllowedDirs     []string `json:"allowed_dirs,omitempty"`
	TimeoutSec      int      `json:"timeout_sec,omitempty"`
}

// normalizeForValidation strips null bytes and trims whitespace from a command.
func normalizeForValidation(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\x00", "")
	return strings.TrimSpace(cmd)
}

// ValidateCommand checks whether a command is permitted under this policy.
// Returns (true, "") if allowed, or (false, reason) if denied.
// Fail-closed: empty AllowedCommands means deny all.
func (p *SecurityPolicy) ValidateCommand(command, workDir string) (bool, string) {
	// Fail-closed: no allowed commands means deny everything.
	if len(p.AllowedCommands) == 0 {
		return false, "no allowed commands configured (fail-closed)"
	}

	// Normalize command BEFORE any pattern matching to prevent bypass via null bytes or encoding tricks.
	command = normalizeForValidation(command)

	// Check against denied patterns first (deny takes precedence).
	for _, pattern := range p.DeniedPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, "invalid denied pattern: " + pattern
		}
		if re.MatchString(command) {
			return false, "command matches denied pattern: " + pattern
		}
	}

	// Check command against allowed commands (word-boundary match).
	// Supports both "go " (trailing space) and "go" (no trailing space) conventions.
	commandAllowed := false
	for _, allowed := range p.AllowedCommands {
		trimmed := strings.TrimRight(allowed, " ")
		if command == trimmed || strings.HasPrefix(command, trimmed+" ") {
			commandAllowed = true
			break
		}
	}
	if !commandAllowed {
		return false, "command not in allowed list"
	}

	// Check workDir against allowed directories (with trailing slash).
	if len(p.AllowedDirs) > 0 && workDir != "" {
		dirAllowed := false
		for _, dir := range p.AllowedDirs {
			if strings.HasPrefix(workDir, dir+"/") || workDir == dir {
				dirAllowed = true
				break
			}
		}
		if !dirAllowed {
			return false, "working directory not in allowed list"
		}
	}

	return true, ""
}
