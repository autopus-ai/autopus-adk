package adapter_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextEngineeringSurface struct {
	name, root, pipeline string
	details              map[string]string
}

func generateContextEngineeringSurfaces(t *testing.T) map[string]contextEngineeringSurface {
	t.Helper()
	base := t.TempDir()
	type generator struct {
		name     string
		generate func(string) (*adapter.PlatformFiles, error)
		pipeline string
		detail   func(string) string
	}
	generators := []generator{
		{"claude", func(root string) (*adapter.PlatformFiles, error) {
			return claude.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("context-engineering"))
		}, ".claude/skills/agent-pipeline/SKILL.md",
			func(command string) string { return ".claude/skills/auto-" + command + "/SKILL.md" }},
		{"codex", func(root string) (*adapter.PlatformFiles, error) {
			return codex.NewWithRoot(root).Generate(context.Background(), config.DefaultFullConfig("context-engineering"))
		}, ".codex/skills/codex-agent-pipeline/SKILL.md",
			func(command string) string { return ".codex/skills/codex-auto-" + command + "/SKILL.md" }},
		{"opencode", func(root string) (*adapter.PlatformFiles, error) {
			return opencode.NewWithRoot(root, opencode.WithCLIVersion("1.18.7")).Generate(context.Background(), config.DefaultFullConfig("context-engineering"))
		}, ".agents/skills/agent-pipeline/SKILL.md",
			func(command string) string { return ".agents/skills/auto-" + command + "/SKILL.md" }},
		{"omp", func(root string) (*adapter.PlatformFiles, error) {
			cfg := config.DefaultFullConfig("context-engineering")
			cfg.Platforms = []string{"omp"}
			return omp.NewWithRoot(root).Generate(context.Background(), cfg)
		}, ".omp/skills/agent-pipeline/SKILL.md",
			func(command string) string { return ".omp/skills/auto-" + command + "/SKILL.md" }},
		{"gemini", func(root string) (*adapter.PlatformFiles, error) {
			return antigravity.NewWithRoot(root).Generate(
				context.Background(), config.DefaultFullConfig("context-engineering"))
		}, ".gemini/skills/autopus/agent-pipeline/SKILL.md",
			func(command string) string { return ".gemini/skills/autopus/auto-" + command + "/SKILL.md" }},
	}
	out := make(map[string]contextEngineeringSurface, len(generators)+1)
	for _, item := range generators {
		root := filepath.Join(base, item.name)
		files, err := item.generate(root)
		require.NoError(t, err, item.name)
		for _, file := range files.Files {
			assert.False(t, filepath.IsAbs(file.TargetPath), "%s emitted absolute target %s", item.name, file.TargetPath)
			assert.False(t, strings.HasPrefix(filepath.Clean(file.TargetPath), ".."), "%s escaped scratch root", item.name)
		}
		details := make(map[string]string, 4)
		for _, command := range []string{"plan", "test", "canary", "go"} {
			details[command] = item.detail(command)
		}
		out[item.name] = contextEngineeringSurface{
			name: item.name, root: root, pipeline: item.pipeline, details: details,
		}
	}
	geminiSurface := out["gemini"]
	pluginDetails := make(map[string]string, 4)
	for _, command := range []string{"plan", "test", "canary", "go"} {
		pluginDetails[command] = ".agents/plugins/autopus/skills/auto-" + command + "/SKILL.md"
	}
	out["antigravity-mirror"] = contextEngineeringSurface{
		name: "antigravity-mirror", root: geminiSurface.root,
		pipeline: ".agents/plugins/autopus/skills/agent-pipeline/SKILL.md",
		details:  pluginDetails,
	}
	return out
}
