package cli

// SPEC-MODELUX-002 Phase 4 fixes: Enter at the default preset select never
// writes when quality.default names no configured preset (security L1).

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// With an empty quality.default no option is the current default, so the
// select leads with a keep option holding it: Enter keeps the file
// byte-identical instead of writing the first preset.
func TestComposeForms_EmptyDefaultEnterAtThePresetSelectWritesNothing(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := writeComposeFixture(t, "claude-code", "codex")
	rewriteComposeLine(t, filepath.Join(dir, "autopus.yaml"), "    default: balanced", `    default: ""`)
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	require.Empty(t, cfg.Quality.Default)
	before := composeSessionHash(t, dir)
	session := newComposeSession(dir, cfg, composeSessionSources(t), "ko", composeSessionFile, nil)

	_, out, err := runComposeForms(t, session, composeFormOptions{Presets: true}, "3\n", "\n", "\n")

	require.NoError(t, err)
	assert.Contains(t, out, "기본 프리셋을 고르세요 \n1. 그대로 — 없음\n2. ultra — ")
	assert.NotContains(t, out, "quality.default = ultra\n")
	assert.Equal(t, before, composeSessionHash(t, dir))
}
