package antigravity

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

const antigravityPluginDir = ".agents/plugins/autopus"

// antigravityPluginManifest is the published plugin.json shape. The official
// schema declares `additionalProperties: false` over exactly these two fields,
// so the marshalled struct — not a map with extras such as `$schema` — is what
// survives strict validation.
type antigravityPluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func prepareAntigravityPluginJSON() ([]adapter.FileMapping, error) {
	body, err := json.MarshalIndent(antigravityPluginManifest{
		Name: "autopus",
		Description: "Autopus-ADK harness: auto-* workflow routes, reusable skills, " +
			"project rules, and subagent definitions.",
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(antigravityPluginDir, "plugin.json"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(string(body)),
		Content:         body,
	}}, nil
}

func mirrorAntigravityPluginMappings(files []adapter.FileMapping) []adapter.FileMapping {
	mirrored := make([]adapter.FileMapping, 0, len(files))
	for _, file := range files {
		target, ok := antigravityPluginTarget(file.TargetPath)
		if !ok {
			continue
		}
		content := rewriteAntigravityPluginContent(string(file.Content))
		mirrored = append(mirrored, adapter.FileMapping{
			TargetPath:      target,
			OverwritePolicy: file.OverwritePolicy,
			Checksum:        checksum(content),
			Content:         []byte(content),
		})
	}
	return mirrored
}

// antigravityPluginTarget maps a Gemini CLI surface path onto the matching
// component inside the workspace plugin. `agy plugin validate` reports
// skills, agents and commands as processed and accepts rules, so those four
// directories are the whole loadable component set; commands are converted
// into skills by the CLI itself.
func antigravityPluginTarget(path string) (string, bool) {
	path = filepath.ToSlash(path)
	switch {
	case strings.HasPrefix(path, ".gemini/skills/autopus/"):
		return strings.Replace(path, ".gemini/skills/autopus/", antigravityPluginDir+"/skills/", 1), true
	case path == ".gemini/skills/auto/SKILL.md":
		return filepath.ToSlash(filepath.Join(antigravityPluginDir, "skills", "auto", "SKILL.md")), true
	case strings.HasPrefix(path, ".gemini/rules/autopus/"):
		return strings.Replace(path, ".gemini/rules/autopus/", antigravityPluginDir+"/rules/", 1), true
	case strings.HasPrefix(path, ".gemini/agents/autopus/"):
		return strings.Replace(path, ".gemini/agents/autopus/", antigravityPluginDir+"/agents/", 1), true
	case path == ".gemini/commands/auto.toml":
		return filepath.ToSlash(filepath.Join(antigravityPluginDir, "commands", "auto.toml")), true
	case strings.HasPrefix(path, ".gemini/commands/auto/"):
		return strings.Replace(path, ".gemini/commands/auto/", antigravityPluginDir+"/commands/auto/", 1), true
	default:
		return "", false
	}
}

func rewriteAntigravityPluginContent(content string) string {
	replacer := strings.NewReplacer(
		".gemini/skills/autopus/", antigravityPluginDir+"/skills/",
		".gemini/skills/auto/", antigravityPluginDir+"/skills/auto/",
		".gemini/rules/autopus/", antigravityPluginDir+"/rules/",
		".gemini/agents/autopus/", antigravityPluginDir+"/agents/",
		".gemini/commands/auto/", antigravityPluginDir+"/commands/auto/",
	)
	return replacer.Replace(content)
}
