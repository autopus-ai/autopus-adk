package brainstorm_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/setup"
)

// layout builds a fixture under a fresh temp base. An entry ending in
// "/.git" is a real repository (git init), "/.git/hooks/" is a meta-root .git
// that holds only hooks/, another entry ending in "/" is a directory, and the
// rest are files holding the line "existing".
func layout(t *testing.T, entries ...string) string {
	t.Helper()
	base := t.TempDir()
	for _, entry := range entries {
		path := filepath.Join(base, filepath.FromSlash(entry))
		switch {
		case strings.HasSuffix(entry, "/.git"):
			gitInit(t, filepath.Dir(path))
		case strings.HasSuffix(entry, "/"):
			require.NoError(t, os.MkdirAll(path, 0o755))
		default:
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("existing\n"), 0o644))
		}
	}
	return base
}

// gitInit makes dir a real repository without the user's git config; the
// empty template skips the sample hooks, which keeps fixtures fast.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	require.NoError(t, os.MkdirAll(dir, 0o755))
	cmd := exec.Command("git", "init", "-q", "--template=", dir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init %s: %s", dir, out)
}

// The S9 topologies (acceptance.md), each in its own temp base.

func topologyP(t *testing.T) string {
	return layout(t, "P/.git", "P/.autopus/brainstorms/BS-BAND-004.md")
}

func topologyW(t *testing.T) string {
	return layout(t, "W/.git/hooks/", "W/module/.git", "W/other/.git",
		"W/.autopus/brainstorms/BS-BAND-010.md",
		"W/module/.autopus/brainstorms/BS-BAND-001.md", "W/module/.autopus/brainstorms/BS-BAND-007.md",
		"W/module/.autopus/brainstorms/BS-042.md", "W/module/.autopus/brainstorms/BS-BAND-0x9.md",
		"W/other/.autopus/brainstorms/BS-BAND-012.md")
}

func topologyH(t *testing.T) string {
	return layout(t, "H/.git", "H/dots/.git", "H/work/proj/",
		"H/.autopus/brainstorms/BS-BAND-050.md", "H/work/proj/.autopus/brainstorms/BS-BAND-002.md")
}

func topologyN(t *testing.T) string {
	return layout(t, "N/.git", "N/m/.git", "N/n/.git", "N/m/x/.git", "N/m/.autopus/brainstorms/BS-BAND-010.md")
}

func topologyO(t *testing.T) string {
	return layout(t, "O/.git", "O/d/.git", "O/W2/.git/hooks/", "O/W2/module/.git", "O/W2/other/.git",
		"O/W2/module/.autopus/brainstorms/BS-BAND-020.md")
}

// probeA3 is the DetectMultiRepo output of probe A3 (plan.md) per fixture
// directory; nil means DetectMultiRepo returns nil.
var probeA3 = map[string][]string{
	"P": nil,
	"W": {".", "module", "other"}, "W/module": nil, "W/other": nil,
	"H": {".", "dots"}, "H/work": nil, "H/work/proj": nil,
	"N": {".", "m", "n"}, "N/m": {".", "x"}, "N/m/x": nil, "N/n": nil,
	"O": {".", "W2", "d"}, "O/W2": {".", "module", "other"}, "O/W2/module": nil, "O/d": nil,
}

// requireProbeA3 asserts the DetectMultiRepo preconditions of every fixture
// directory under base whose top-level name is in tops.
func requireProbeA3(t *testing.T, base string, tops ...string) {
	t.Helper()
	for dir, want := range probeA3 {
		top, _, _ := strings.Cut(dir, "/")
		if !contains(tops, top) {
			continue
		}
		var got []string
		if info := setup.DetectMultiRepo(filepath.Join(base, filepath.FromSlash(dir))); info != nil {
			for _, component := range info.Components {
				got = append(got, component.Path)
			}
		}
		require.Equal(t, want, got, "precondition DetectMultiRepo(%s)", dir)
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// rel returns paths relative to base with forward slashes.
func rel(t *testing.T, base string, paths ...string) []string {
	t.Helper()
	out := make([]string, len(paths))
	for i, path := range paths {
		r, err := filepath.Rel(base, path)
		require.NoError(t, err)
		out[i] = filepath.ToSlash(r)
	}
	return out
}

// snapshot lists every path under base, relative and sorted.
func snapshot(t *testing.T, base string) []string {
	t.Helper()
	var paths []string
	require.NoError(t, filepath.WalkDir(base, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != base {
			paths = append(paths, rel(t, base, path)[0])
		}
		return nil
	}))
	sort.Strings(paths)
	return paths
}

// added returns the paths of after that before lacks.
func added(before, after []string) []string {
	seen := make(map[string]bool, len(before))
	for _, path := range before {
		seen[path] = true
	}
	var out []string
	for _, path := range after {
		if !seen[path] {
			out = append(out, path)
		}
	}
	return out
}
