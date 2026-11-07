package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestAdapter_Generate_MarksOnlyMixedSharedSkillsAsOpenCodeOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		platforms []string
		wantMark  bool
	}{
		{name: "mixed Codex and OpenCode", platforms: []string{"codex", "opencode"}, wantMark: true},
		{name: "single OpenCode", platforms: []string{"opencode"}, wantMark: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			cfg := config.DefaultFullConfig("demo")
			cfg.Platforms = test.platforms

			_, err := NewWithRoot(dir, WithCLIVersion("1.18.7")).Generate(context.Background(), cfg)
			require.NoError(t, err)

			for _, name := range []string{"auto", "auto-review", "auto-qa", "review"} {
				path := filepath.Join(dir, ".agents", "skills", name, "SKILL.md")
				content, readErr := os.ReadFile(path)
				require.NoError(t, readErr, path)
				if test.wantMark {
					assert.Contains(t, string(content), `description: "[OpenCode-only] `, path)
				} else {
					assert.NotContains(t, string(content), "[OpenCode-only]", path)
				}
			}

			commandPath := filepath.Join(dir, ".opencode", "commands", "auto.md")
			command, readErr := os.ReadFile(commandPath)
			require.NoError(t, readErr)
			assert.NotContains(t, string(command), "[OpenCode-only]")
			assert.Contains(t, string(command), "/auto")
		})
	}
}
