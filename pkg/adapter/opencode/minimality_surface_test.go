package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeAdapter_Generate_MinimalityCommandsStayThinAndSharedSkillsCarryContracts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := NewWithRoot(dir, WithCLIVersion("1.18.7")).Generate(context.Background(), config.DefaultFullConfig("minimality-project"))
	require.NoError(t, err)

	for _, name := range []string{"auto-review", "auto-qa", "auto-sync"} {
		name := name
		t.Run(name+"-thin-command", func(t *testing.T) {
			t.Parallel()
			content := readGeneratedOpenCodeSurface(t, dir, filepath.Join(".opencode", "commands", name+".md"))
			subcommand := name[len("auto-"):]
			assert.Contains(t, content, "`$ARGUMENTS`")
			assert.Contains(t, content, "/auto "+subcommand+" ...")
			assert.Contains(t, content, "`skill` 도구로 `auto`를 로드")
			assert.Contains(t, content, "Preserve `--model <provider/model>` and `--variant <value>`")
			assert.NotContains(t, content, "`skill` 도구로 `"+name+"`를 로드")
			assert.NotContains(t, content, "Minimality Decision Matrix")
			assert.NotContains(t, content, "Correctness/Security Findings")
		})
	}

	reviewTokens := []string{
		"Correctness/Security Findings",
		"Complexity Findings",
		"delete",
		"stdlib",
		"native",
		"yagni",
		"shrink",
		"existing-helper",
		"existing-dependency",
	}
	cases := []struct {
		name   string
		tokens []string
	}{
		{"auto-review", append(reviewTokens, "receipt")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"-shared-skill", func(t *testing.T) {
			t.Parallel()
			content := readGeneratedOpenCodeSurface(t, dir, filepath.Join(".agents", "skills", tc.name, "SKILL.md"))
			for _, token := range tc.tokens {
				assert.Contains(t, content, token, "%s shared skill should contain %q", tc.name, token)
			}
		})
	}
}

func readGeneratedOpenCodeSurface(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err, rel)
	return string(body)
}
