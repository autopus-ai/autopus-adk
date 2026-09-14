package claude

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"gopkg.in/yaml.v3"
)

var legacyClaudeSkillReference = regexp.MustCompile(`\.claude/skills/autopus/([A-Za-z0-9_-]+)\.md`)

// normalizeClaudeContent projects embedded content onto Claude's native
// frontmatter and path contracts.
func normalizeClaudeContent(cfg *config.HarnessConfig, subDir, filename string, data []byte) []byte {
	content := rewriteClaudeSkillReferences(string(data))
	switch subDir {
	case "skills":
		return []byte(normalizeClaudeSkillFrontmatter(content))
	case "agents":
		normalized := pkgcontent.NormalizeAgentReferences(content, "claude-code")
		normalized = pkgcontent.FilterDeclaredSkills(normalized, "claude", cfg)
		return []byte(applyClaudeAgentProfile(cfg, filename, rewriteClaudeSkillReferences(normalized)))
	default:
		return []byte(content)
	}
}

func rewriteClaudeSkillReferences(content string) string {
	return legacyClaudeSkillReference.ReplaceAllString(content, `.claude/skills/$1/SKILL.md`)
}

func normalizeClaudeSkillFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return content
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return content
	}
	var metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &metadata); err != nil ||
		strings.TrimSpace(metadata.Name) == "" {
		return content
	}
	body := strings.Join(lines[end+1:], "\n")
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s",
		strconv.Quote(metadata.Name), strconv.Quote(strings.TrimSpace(metadata.Description)), body)
}

// applyClaudeAgentProfile rewrites model and effort as one quality projection.
// @AX:WARN [AUTO]: agent profile projection contains more than eight conditional branches.
// @AX:REASON [AUTO]: frontmatter admission, model replacement, and tier-dependent effort insertion or removal must preserve one coherent profile.
func applyClaudeAgentProfile(cfg *config.HarnessConfig, filename, content string) string {
	if cfg == nil {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return content
	}
	end, modelIndex, effortIndex := -1, -1, -1
	currentModel := ""
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
		if value, ok := strings.CutPrefix(lines[i], "model:"); ok {
			modelIndex = i
			currentModel = strings.TrimSpace(value)
		}
		if _, ok := strings.CutPrefix(lines[i], "effort:"); ok {
			effortIndex = i
		}
	}
	if end < 0 || modelIndex < 0 {
		return content
	}
	agent := config.NormalizeAgentName(strings.TrimSuffix(filename, ".md"))
	model, effort := claudeAgentProfile(cfg, agent, currentModel)
	lines[modelIndex] = "model: " + model
	if effortIndex >= 0 {
		if effort == "" {
			lines = append(lines[:effortIndex], lines[effortIndex+1:]...)
		} else {
			lines[effortIndex] = "effort: " + effort
		}
	} else if effort != "" {
		lines = append(lines[:modelIndex+1], append([]string{"effort: " + effort}, lines[modelIndex+1:]...)...)
	}
	return strings.Join(lines, "\n")
}

// claudeAgentProfile resolves one agent's frontmatter model and reasoning
// effort. The standard balanced placement addresses exact model ids so Claude
// Code routes the top rung to the frontier model instead of whichever release
// the tier alias currently points at; every other preset — Ultra, a custom
// preset, or a per-agent tier the user moved off the standard rung — keeps
// projecting the relative tier alias.
func claudeAgentProfile(cfg *config.HarnessConfig, agent, fallbackTier string) (model, effort string) {
	if candidate, ok := cfg.Quality.NativeBalancedAgentCandidate(config.QualityProviderClaude, agent); ok {
		return nativeClaudeModelID(candidate.Selector), candidate.Thinking
	}
	tier := cfg.Quality.AgentTier(config.QualityProviderClaude, agent, fallbackTier)
	return tier, claudeAgentEffort(cfg.Quality.EffectiveMode(config.QualityProviderClaude), tier)
}

// nativeClaudeModelID drops the routing provider prefix from a role model
// selector, leaving the model id the Claude CLI itself accepts.
func nativeClaudeModelID(selector string) string {
	if _, model, found := strings.Cut(selector, "/"); found {
		return model
	}
	return selector
}

func claudeAgentEffort(mode, tier string) string {
	switch tier {
	case "haiku":
		return ""
	case "fable":
		return "max"
	case "opus":
		if mode == "ultra" {
			return "max"
		}
		return "high"
	default:
		if mode == "ultra" {
			return "high"
		}
		return "medium"
	}
}
