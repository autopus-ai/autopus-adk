package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSpecDoc(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

// Cited sources come from research.md and plan.md only, deduplicated, with URLs
// and prose left out: a URL or a sentence fragment resolves to no file and would
// spend the review's line budget on nothing.
func TestExtractSpecContextTargetsForCLI_CollectsCitedSourcesOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSpecDoc(t, dir, "research.md", "관련 코드는 `pkg/spec/context_collect.go`와 ./internal/cli/spec.go 이며\n"+
		"참고 문서는 https://example.com/docs/guide.go 이다.\n")
	writeSpecDoc(t, dir, "plan.md", "- [ ] T1: pkg/spec/context_collect.go 수정\n- [ ] T2: cmd/auto/main.go 추가\n")
	writeSpecDoc(t, dir, "acceptance.md", "pkg/ignored/not_read.go\n")

	targets := ExtractSpecContextTargetsForCLI(dir)

	assert.Equal(t, []string{
		"pkg/spec/context_collect.go",
		"internal/cli/spec.go",
		"cmd/auto/main.go",
	}, targets)
	assert.NotContains(t, targets, "pkg/ignored/not_read.go")
	for _, target := range targets {
		assert.NotContains(t, target, "example.com")
	}
}

func TestExtractSpecContextTargetsForCLI_EmptyWhenNoCitingDocuments(t *testing.T) {
	t.Parallel()

	assert.Empty(t, ExtractSpecContextTargetsForCLI(t.TempDir()))
}

// A module root wins over the project root, so a submodule's own copy of a path
// is reviewed instead of the meta repo's file with the same relative name.
func TestResolveSpecTargetPathForCLI_PrefersModuleRoot(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	module := t.TempDir()
	for _, root := range []string{project, module} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "pkg"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "pkg", "a.go"), []byte("package a\n"), 0o644))
	}

	resolved := ResolveSpecTargetPathForCLI(project, module, "pkg/a.go")

	assert.Equal(t, filepath.Join(module, "pkg", "a.go"), resolved)
}

// The project root is the fallback when the module does not carry the file.
func TestResolveSpecTargetPathForCLI_FallsBackToProjectRoot(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "only-here.go"), []byte("package a\n"), 0o644))

	assert.Equal(t,
		filepath.Join(project, "only-here.go"),
		ResolveSpecTargetPathForCLI(project, t.TempDir(), "only-here.go"),
	)
}

// A directory is not a reviewable target, and an unresolvable path returns the
// empty string instead of a guess the caller would then fail to read.
func TestResolveSpecTargetPathForCLI_RejectsDirectoryAndMissingTarget(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, "pkg"), 0o755))

	assert.Empty(t, ResolveSpecTargetPathForCLI(project, "", "pkg"))
	assert.Empty(t, ResolveSpecTargetPathForCLI(project, "", "pkg/absent.go"))
	assert.Empty(t, ResolveSpecTargetPathForCLI("", "", "pkg/absent.go"))
}

// An absolute target is taken as given rather than joined onto a root, which
// would produce a nonexistent path like <root>/<root>/file.go.
func TestResolveSpecTargetPathForCLI_HonorsAbsoluteTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	absolute := filepath.Join(dir, "abs.go")
	require.NoError(t, os.WriteFile(absolute, []byte("package a\n"), 0o644))

	assert.Equal(t, absolute, ResolveSpecTargetPathForCLI(t.TempDir(), "", absolute))
}
