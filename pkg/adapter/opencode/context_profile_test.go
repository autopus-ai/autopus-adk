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

func TestOpenCodeAdapter_ContextProfilesReplaceUnconditionalProjectDocumentLoad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := NewWithRoot(root, WithCLIVersion("1.18.7")).Generate(context.Background(), config.DefaultFullConfig("context-profile"))
	require.NoError(t, err)

	router := readOpenCodeContextSurface(t, root, ".agents/skills/auto/SKILL.md")
	assert.Contains(t, router, "test = core + test")
	assert.NotContains(t, router, "plan = core")
	assert.NotContains(t, router, "canary = core")
	assert.NotContains(t, router, "서브커맨드가 명확해 보여도 이 로드 단계를 생략하지 않습니다")
}

func readOpenCodeContextSurface(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err)
	return string(body)
}
