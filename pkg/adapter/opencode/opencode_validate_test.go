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

func TestAdapter_Validate_WarnsWhenContext7FallbackMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("demo"))
	require.NoError(t, err)

	rulePath := filepath.Join(dir, ".opencode", "rules", "autopus", "context7-docs.md")
	require.NoError(t, os.WriteFile(rulePath, []byte("# Context7\n"), 0644))

	errs, err := a.Validate(context.Background())
	require.NoError(t, err)

	found := false
	for _, e := range errs {
		if e.File == filepath.Join(".opencode", "rules", "autopus", "context7-docs.md") && e.Message == "OpenCode Context7 규칙에 web fallback 계약이 없음" {
			found = true
		}
	}
	assert.True(t, found, "context7 fallback warning should be reported")
}

func TestInjectOrchestraPlugin_InvalidExistingJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("1.18.7"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "opencode.json"), []byte("{broken"), 0o644))

	err := a.InjectOrchestraPlugin("/path/to/script.js")
	assert.Error(t, err)
}
