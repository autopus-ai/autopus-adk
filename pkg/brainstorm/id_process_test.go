package brainstorm_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
)

const (
	childStartEnv = "BRAINSTORM_TEST_CHILD_START"
	childCacheEnv = "BRAINSTORM_TEST_CHILD_CACHE"
	childGoEnv    = "BRAINSTORM_TEST_CHILD_GO"
)

// TestMain doubles as a band process that writes one BS: separate
// processes are the honest proof that the per-user lock serializes
// allocations across processes.
func TestMain(m *testing.M) {
	if start := os.Getenv(childStartEnv); start != "" {
		os.Exit(runChild(start, os.Getenv(childCacheEnv), os.Getenv(childGoEnv)))
	}
	os.Exit(m.Run())
}

// runChild waits until the go file exists, so every child starts its
// allocation at the same time, then writes one BS and prints its path.
func runChild(start, cache, goFile string) int {
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		if _, err := os.Stat(goFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return 3
		}
	}
	opts := brainstorm.Options{CacheDir: func() (string, error) { return cache, nil }}
	result, err := brainstorm.Write(context.Background(), start, o2Request(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 4
	}
	fmt.Print(result.Path)
	return 0
}

// writeTogether runs one child process per start, releases them at once,
// and returns the created paths relative to base in start order.
func writeTogether(t *testing.T, base string, starts ...string) []string {
	t.Helper()
	cache, goFile := t.TempDir(), filepath.Join(t.TempDir(), "go")
	cmds := make([]*exec.Cmd, len(starts))
	outs := make([]*bytes.Buffer, len(starts))
	for i, start := range starts {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), childStartEnv+"="+filepath.Join(base, filepath.FromSlash(start)),
			childCacheEnv+"="+cache, childGoEnv+"="+goFile)
		outs[i] = &bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = outs[i], outs[i]
		require.NoError(t, cmd.Start())
		cmds[i] = cmd
	}
	require.NoError(t, os.WriteFile(goFile, nil, 0o600))
	paths := make([]string, len(starts))
	for i, cmd := range cmds {
		require.NoError(t, cmd.Wait(), "child %s: %s", starts[i], outs[i])
		paths[i] = rel(t, base, outs[i].String())[0]
	}
	return paths
}

func names(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = path.Base(p)
	}
	return out
}

// S9: on a fresh copy of N, starts in N/m/x and N/m at the same time take
// BS-BAND-011 and BS-BAND-012, each in its own directory, never one ID twice.
func TestWrite_S9NestedStartsAtTheSameTimeNeverShareAnID(t *testing.T) {
	t.Parallel()
	base := topologyN(t)
	requireProbeA3(t, base, "N")

	paths := writeTogether(t, base, "N/m/x", "N/m")

	assert.ElementsMatch(t, []string{"BS-BAND-011.md", "BS-BAND-012.md"}, names(paths))
	assert.Equal(t, "N/m/x/.autopus/brainstorms", path.Dir(paths[0]))
	assert.Equal(t, "N/m/.autopus/brainstorms", path.Dir(paths[1]))
}

// S9: on one copy of W, starts in W/module, W, and W/other at the same time
// take BS-BAND-013, 014, and 015, never one ID twice.
func TestWrite_S9MetaRootStartsAtTheSameTimeNeverShareAnID(t *testing.T) {
	t.Parallel()
	base := topologyW(t)
	requireProbeA3(t, base, "W")

	paths := writeTogether(t, base, "W/module", "W", "W/other")

	assert.ElementsMatch(t, []string{"BS-BAND-013.md", "BS-BAND-014.md", "BS-BAND-015.md"}, names(paths))
	assert.Equal(t, []string{"W/module/.autopus/brainstorms", "W/.autopus/brainstorms", "W/other/.autopus/brainstorms"},
		[]string{path.Dir(paths[0]), path.Dir(paths[1]), path.Dir(paths[2])})
}
