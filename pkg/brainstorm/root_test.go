package brainstorm_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/setup"
)

// BS Root Resolution items 1 and 2: every start inside one component tree
// computes the same root and the same scope, root first and then each level
// of components in DetectMultiRepo order.
func TestResolveScope_S9EveryStartInATreeSharesItsRootAndScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		build  func(*testing.T) string
		starts []string
		root   string
		dirs   []string
	}{
		{name: "single repo", build: topologyP, starts: []string{"P"}, root: "P", dirs: []string{"P"}},
		{name: "meta root whose .git holds only hooks", build: topologyW, starts: []string{"W", "W/module", "W/other"},
			root: "W", dirs: []string{"W", "W/module", "W/other"}},
		{name: "project that no repository lists", build: topologyH, starts: []string{"H/work/proj"},
			root: "H/work/proj", dirs: []string{"H/work/proj"}},
		{name: "repository with a child repository", build: topologyH, starts: []string{"H", "H/dots"},
			root: "H", dirs: []string{"H", "H/dots"}},
		{name: "nested repository", build: topologyN, starts: []string{"N", "N/m", "N/m/x", "N/n"},
			root: "N", dirs: []string{"N", "N/m", "N/n", "N/m/x"}},
		{name: "outer repository listing a meta root", build: topologyO, starts: []string{"O", "O/W2", "O/W2/module", "O/d"},
			root: "O", dirs: []string{"O", "O/W2", "O/d", "O/W2/module", "O/W2/other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := tc.build(t)
			requireProbeA3(t, base, strings.Split(tc.root, "/")[0])
			for _, start := range tc.starts {
				scope, err := brainstorm.ResolveScope(filepath.Join(base, filepath.FromSlash(start)))

				require.NoError(t, err, start)
				assert.Equal(t, tc.root, rel(t, base, scope.Root)[0], "root from %s", start)
				assert.Equal(t, tc.dirs, rel(t, base, scope.Dirs...), "scope from %s", start)
			}
		})
	}
}

// Component edges start only at repositories, the same rule as the chain: a
// plain directory holding two repositories lists them without the root
// component, so it neither adopts them nor becomes their root.
func TestResolveScope_PlainDirectoryHasNoComponentEdges(t *testing.T) {
	t.Parallel()
	base := layout(t, "X/a/.git", "X/b/.git")
	info := setup.DetectMultiRepo(filepath.Join(base, "X"))
	require.NotNil(t, info, "precondition: X lists its two repositories")
	require.Len(t, info.Components, 2)

	plain, err := brainstorm.ResolveScope(filepath.Join(base, "X"))
	require.NoError(t, err)
	child, err := brainstorm.ResolveScope(filepath.Join(base, "X", "a"))
	require.NoError(t, err)

	assert.Equal(t, []string{"X"}, rel(t, base, plain.Dirs...))
	assert.Equal(t, []string{"X/a"}, rel(t, base, child.Dirs...))
}

// A relative start resolves like its absolute spelling.
func TestResolveScope_RelativeStartIsMadeAbsolute(t *testing.T) {
	base := topologyN(t)
	t.Chdir(filepath.Join(base, "N", "m"))

	scope, err := brainstorm.ResolveScope("x")

	require.NoError(t, err)
	assert.Equal(t, []string{"N", "N/m", "N/n", "N/m/x"}, rel(t, base, scope.Dirs...))
}

// chain builds r0/r1/…/r<depth>, each listed by its parent. A .git that
// holds only hooks/ is a repository to DetectMultiRepo (probe A3), and it
// keeps this ten-level fixture fast.
func chain(t *testing.T, depth int) (base string, dirs []string) {
	t.Helper()
	var entries []string
	path := "r0"
	for i := 0; i <= depth; i++ {
		if i > 0 {
			path += fmt.Sprintf("/r%d", i)
		}
		entries = append(entries, path+"/.git/hooks/")
		dirs = append(dirs, path)
	}
	return layout(t, entries...), dirs
}

// BS Root Resolution item 2 scans at most 8 levels. A start 8 component
// edges below its root is the deepest start the scan from the root reaches;
// a deeper start is refused before anything is written, because an ID
// allocated outside the shared scope could collide.
func TestResolveScope_RefusesAStartBeyondTheEightLevelScan(t *testing.T) {
	t.Parallel()
	require.Equal(t, 8, brainstorm.MaxScanDepth)
	base, dirs := chain(t, 9)

	scope, err := brainstorm.ResolveScope(filepath.Join(base, filepath.FromSlash(dirs[8])))
	require.NoError(t, err)
	assert.Equal(t, "r0", rel(t, base, scope.Root)[0])
	assert.Equal(t, dirs[:9], rel(t, base, scope.Dirs...), "the scan from the root reaches depth 8 and stops")

	_, err = brainstorm.ResolveScope(filepath.Join(base, filepath.FromSlash(dirs[9])))
	require.ErrorIs(t, err, brainstorm.ErrScopeTooDeep)
	assert.Equal(t, brainstorm.ReasonScopeTooDeep, brainstorm.Reason(err))
}
