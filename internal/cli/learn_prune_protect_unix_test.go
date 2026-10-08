//go:build !windows

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLearnPrune_ControlCharacterFileName_IsEscapedInTheError(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)
	path := s7PruneProject(t, dir, true, true)
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "evals", "harness", "tasks", "surface", "GT-\x1b[2J.json")); err != nil {
		t.Fatal(err)
	}

	if msg := assertPruneRefused(t, path); !strings.Contains(msg, `GT-\u001b[2J.json`) || strings.Contains(msg, "\x1b") {
		t.Fatalf("the file name must reach the error escaped: %q", msg)
	}
}

func TestLearnPrune_S7S10_SymlinkedLinkOrTask_RefuseWithTheStoreUnchanged(t *testing.T) {
	cases := map[string]string{
		"symlinked promoted link":    "evals/harness/candidates/promoted/GT-INC-023E9302.json",
		"symlinked active task":      "evals/harness/tasks/surface/GT-INC-023E9302.json",
		"symlinked candidates":       "evals/harness/candidates",
		"symlinked surface task dir": "evals/harness/tasks/surface",
	}
	for name, rel := range cases {
		t.Run(name, func(t *testing.T) {
			dir := setupLearnDir(t)
			chdir(t, dir)
			path := s7PruneProject(t, dir, true, true)
			outside := t.TempDir()
			writeProjectFile(t, outside, "target.json", "{}\n")
			full := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.RemoveAll(full); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "target.json"), full); err != nil {
				t.Fatal(err)
			}
			before := fileSHA256(t, filepath.Join(outside, "target.json"))

			assertPruneRefused(t, path)

			if after := fileSHA256(t, filepath.Join(outside, "target.json")); after != before {
				t.Fatalf("the symlink target changed: %s -> %s", before, after)
			}
		})
	}
}
