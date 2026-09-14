package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_MissingGeminiMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	errs, err := a.Validate(context.Background())
	require.NoError(t, err)
	found := false
	for _, e := range errs {
		if e.File == "GEMINI.md" && e.Message == "GEMINI.md를 읽을 수 없음" {
			found = true
		}
	}
	assert.True(t, found, "missing GEMINI.md must produce a read error")
}

func TestValidate_MissingMarkerAndNativeSurface(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	// GEMINI.md exists but has no AUTOPUS marker, and nothing is installed.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "GEMINI.md"), []byte("# plain\n"), 0o644))

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)

	var markerWarn, skillErr, manifestErr bool
	for _, e := range errs {
		if e.Message == "AUTOPUS 마커 섹션이 없음" {
			markerWarn = true
		}
		if e.Level == "error" && e.File == antigravityPluginDir+"/plugin.json" {
			manifestErr = true
		}
		if e.Level == "error" && strings.Contains(e.Message, "SKILL.md가 없음") {
			skillErr = true
		}
	}
	assert.True(t, markerWarn, "missing marker must warn")
	assert.True(t, skillErr, "missing skill dirs must error")
	assert.True(t, manifestErr, "a missing workspace plugin manifest must error")
}

func TestGenerateSettings_MergesInvalidExistingJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	settingsDir := filepath.Join(dir, ".gemini")
	require.NoError(t, os.MkdirAll(settingsDir, 0o755))
	// Invalid existing JSON: merge is skipped, generation still succeeds.
	require.NoError(t, os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte("{not json"), 0o644))

	mappings, err := a.generateSettings(config.DefaultFullConfig("demo"))
	require.NoError(t, err)
	require.NotEmpty(t, mappings)
	assert.Equal(t, filepath.Join(".gemini", "settings.json"), mappings[0].TargetPath)
}

func TestClean_RemovesGeneratedSurfaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	require.NoError(t, a.Clean(context.Background()))

	_, statErr := os.Stat(filepath.Join(dir, ".gemini", "skills"))
	assert.True(t, os.IsNotExist(statErr), ".gemini/skills must be removed")

	// GEMINI.md remains but the marker section is stripped.
	data, readErr := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
	require.NoError(t, readErr)
	assert.NotContains(t, string(data), markerBegin)
}

func TestClean_NoGeminiMD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir)
	// Clean is a no-op tolerant of a missing GEMINI.md.
	assert.NoError(t, a.Clean(context.Background()))
}
