package run

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/desktopobserve"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
)

// Guards the inverted "disabled" marker: the renderer only marks the negative
// case, so a node with no marker must report enabled=true while a disabled
// marker must report false. Reading it like the positive markers would invert
// every enabled-state verdict.
func TestDeclaredNodeStateEnabledIsInverted(t *testing.T) {
	t.Parallel()

	plain := declaredNodeState(desktopTreeNode{}, desktopobserve.StateEnabled)
	require.NotNil(t, plain.Enabled)
	assert.True(t, *plain.Enabled)
	assert.Nil(t, plain.Focused, "only the declared key may be reported")

	disabled := declaredNodeState(
		desktopTreeNode{StateMarkers: []string{"disabled"}}, desktopobserve.StateEnabled,
	)
	require.NotNil(t, disabled.Enabled)
	assert.False(t, *disabled.Enabled)
}

// Guards that positive markers are read directly and that only the requested
// key is populated, so the oracle never sees state nobody declared.
func TestDeclaredNodeStateReportsOnlyRequestedKey(t *testing.T) {
	t.Parallel()

	node := desktopTreeNode{StateMarkers: []string{"focused", "selected"}}

	focused := declaredNodeState(node, desktopobserve.StateFocused)
	require.NotNil(t, focused.Focused)
	assert.True(t, *focused.Focused)
	assert.Nil(t, focused.Selected)
	assert.Nil(t, focused.Enabled)

	expanded := declaredNodeState(node, desktopobserve.StateExpanded)
	require.NotNil(t, expanded.Expanded)
	assert.False(t, *expanded.Expanded, "absent marker is an observed false, not unknown")

	selected := declaredNodeState(node, desktopobserve.StateSelected)
	require.NotNil(t, selected.Selected)
	assert.True(t, *selected.Selected)

	unknown := declaredNodeState(node, desktopobserve.SemanticStateKey("checked"))
	assert.Equal(t, desktopobserve.SemanticState{}, unknown)
}

// Guards the declared-name suffix rule: localized role prefixes must still
// match, but an arbitrary substring or a prefix match must not, or unrelated
// nodes would satisfy declared landmarks.
func TestMatchesDeclaredNameSuffixBoundary(t *testing.T) {
	t.Parallel()

	node := desktopTreeNode{Name: "0 표준 윈도우 응용 프로그램"}

	assert.True(t, node.matchesDeclaredName("응용 프로그램"))
	assert.True(t, node.matchesDeclaredName("  응용 프로그램  "), "declared name is trimmed")
	assert.True(t, desktopTreeNode{Name: "Save"}.matchesDeclaredName("Save"))

	assert.True(t, node.matchesDeclaredName("프로그램"), "any space-delimited tail matches")
	assert.False(t, node.matchesDeclaredName("윈도우"), "interior match is not a suffix")
	assert.False(t, desktopTreeNode{Name: "Save As"}.matchesDeclaredName("Save"))
	assert.False(t, node.matchesDeclaredName(""))
	assert.False(t, node.matchesDeclaredName("   "))

	// A suffix that is not preceded by a space must not match, so "Unsave"
	// cannot satisfy a declared "save".
	assert.False(t, desktopTreeNode{Name: "Unsave"}.matchesDeclaredName("save"))
}

// Guards the mobile timeout precedence: a valid declared duration wins, while
// unparseable and non-positive declarations fall back to the default instead
// of producing an immediately expiring context.
func TestMobilePackTimeoutPrecedence(t *testing.T) {
	t.Parallel()

	declared := journey.Pack{}
	declared.Command.Timeout = "45s"
	assert.Equal(t, 45*time.Second, mobilePackTimeout(declared))

	for _, raw := range []string{"", "not-a-duration", "0s", "-5s"} {
		pack := journey.Pack{}
		pack.Command.Timeout = raw
		assert.Equal(t, mobileDefaultTimeout, mobilePackTimeout(pack), raw)
	}
}

// Guards the per-OS Playwright browser cache location; a wrong path silently
// re-downloads browsers on every run.
func TestDefaultPlaywrightBrowsersPathPerOS(t *testing.T) {
	t.Parallel()

	got := defaultPlaywrightBrowsersPath("/home/tester")
	switch runtime.GOOS {
	case "darwin":
		assert.Equal(t, filepath.Join("/home/tester", "Library", "Caches", "ms-playwright"), got)
	case "windows":
		assert.Equal(t, filepath.Join("/home/tester", "AppData", "Local", "ms-playwright"), got)
	default:
		assert.Equal(t, filepath.Join("/home/tester", ".cache", "ms-playwright"), got)
	}
}

// Guards the containment check that decides whether the symlink walk runs: a
// cache root outside the project is trusted, one inside must be validated.
func TestSandboxHomeContainedScope(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	outside := goCachePaths{ProjectDir: project, Root: t.TempDir()}
	assert.True(t, sandboxHomeContained(outside, filepath.Join(outside.Root, sandboxHomeDirName)))

	inside := goCachePaths{ProjectDir: project, Root: filepath.Join(project, ".qamesh-cache")}
	home := filepath.Join(inside.Root, sandboxHomeDirName)
	require.NoError(t, os.MkdirAll(home, 0o755))
	assert.True(t, sandboxHomeContained(inside, home))
}

// Guards the symlink refusal: a pre-planted link inside the project cache root
// must make containment fail so ensureSandboxHome falls back to a private temp
// HOME instead of letting tool state escape the cache root.
func TestEnsureSandboxHomeFallsBackOnSymlinkedRoot(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	escape := t.TempDir()
	root := filepath.Join(project, ".qamesh-cache")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.Symlink(escape, filepath.Join(root, sandboxHomeDirName)))

	paths := goCachePaths{ProjectDir: project, Root: root}
	assert.False(t, sandboxHomeContained(paths, filepath.Join(root, sandboxHomeDirName)))

	home := ensureSandboxHome(paths)
	assert.NotEqual(t, filepath.Join(root, sandboxHomeDirName), home)
	assert.DirExists(t, home)
}
