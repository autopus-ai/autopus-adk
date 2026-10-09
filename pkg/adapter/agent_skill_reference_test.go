package adapter_test

import (
	"context"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

var (
	openCodeAgentSkillsRe = regexp.MustCompile("(?m)^Use the following Autopus skills when they fit the task: (.+)\\.$")
	codexAgentSkillsRe    = regexp.MustCompile(`(?m)^- Skills reference: (.+)$`)
	backtickedNameRe      = regexp.MustCompile("`([^`]+)`")
)

// generatedSkillNames is the set of skills a platform's install wrote, read
// from the SKILL.md paths themselves; Codex prefixes its native skills.
func generatedSkillNames(files []adapter.FileMapping) map[string]bool {
	names := map[string]bool{}
	for _, file := range files {
		target := strings.ReplaceAll(file.TargetPath, "\\", "/")
		if path.Base(target) == "SKILL.md" && path.Base(path.Dir(path.Dir(target))) == "skills" ||
			path.Base(target) == "SKILL.md" && path.Base(path.Dir(path.Dir(path.Dir(target)))) == "skills" {
			names[strings.TrimPrefix(path.Base(path.Dir(target)), "codex-")] = true
		}
	}
	return names
}

// agentSkillReferences returns the skill names one generated agent file
// declares: a frontmatter `skills:` list, the OpenCode skill line, or the Codex
// operational-defaults line.
func agentSkillReferences(body string) []string {
	var names []string
	if strings.HasPrefix(body, "---\n") {
		frontmatter, _, _ := strings.Cut(strings.TrimPrefix(body, "---\n"), "\n---")
		inSkills := false
		for _, line := range strings.Split(frontmatter, "\n") {
			switch {
			case strings.TrimSpace(line) == "skills:":
				inSkills = true
			case inSkills && strings.HasPrefix(strings.TrimSpace(line), "- "):
				names = append(names, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")))
			default:
				inSkills = false
			}
		}
	}
	for _, m := range openCodeAgentSkillsRe.FindAllStringSubmatch(body, -1) {
		for _, name := range backtickedNameRe.FindAllStringSubmatch(m[1], -1) {
			names = append(names, name[1])
		}
	}
	for _, m := range codexAgentSkillsRe.FindAllStringSubmatch(body, -1) {
		for _, name := range strings.Split(m[1], ",") {
			names = append(names, strings.TrimSpace(name))
		}
	}
	return names
}

func isGeneratedAgentFile(target string) bool {
	dir := path.Dir(target)
	return path.Base(dir) == "agents" || path.Base(path.Dir(dir)) == "agents" && path.Base(dir) == "autopus"
}

// assertAgentsReferenceGeneratedSkills checks every generated agent of every
// platform against the skills that platform's install actually wrote, and
// returns the references it saw per platform.
func assertAgentsReferenceGeneratedSkills(t *testing.T, cfg *config.HarnessConfig) map[string]map[string]bool {
	t.Helper()
	seen := map[string]map[string]bool{}
	for _, platform := range allPlatforms {
		pf, err := platformGenerators[platform](t.TempDir(), context.Background(), cfg)
		require.NoError(t, err)
		skills := generatedSkillNames(pf.Files)
		seen[platform] = map[string]bool{}
		for _, file := range pf.Files {
			target := strings.ReplaceAll(file.TargetPath, "\\", "/")
			if !isGeneratedAgentFile(target) {
				continue
			}
			for _, name := range agentSkillReferences(string(file.Content)) {
				seen[platform][name] = true
				assert.True(t, skills[name], "%s %s references skill %q, which the %s install does not generate",
					platform, target, name, platform)
			}
		}
	}
	return seen
}

// An agent that names a skill its surface never installed sends the model
// looking for guidance that does not exist there.
func TestGeneratedAgents_ReferenceOnlyGeneratedSkills(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("agent-skill-refs")
	cfg.Platforms = allPlatforms

	seen := assertAgentsReferenceGeneratedSkills(t, cfg)

	for _, platform := range []string{"claude-code", "codex", "antigravity-cli", "opencode"} {
		assert.True(t, seen[platform]["tdd"], "%s agents still reference installed skills", platform)
	}
}

// The filter follows the compiled surface, not a fixed list: a skill the
// project installs explicitly is referenced again on every platform.
func TestGeneratedAgents_ReferenceExplicitlyInstalledSkills(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultFullConfig("agent-skill-refs-explicit")
	cfg.Platforms = allPlatforms
	cfg.Skills.Compiler.ExplicitSkills = []string{"ddd"}

	seen := assertAgentsReferenceGeneratedSkills(t, cfg)

	for _, platform := range []string{"claude-code", "codex", "antigravity-cli", "opencode"} {
		assert.True(t, seen[platform]["ddd"], "%s agents reference ddd once it is installed", platform)
	}
}
