package editguard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// openCodeSpellings returns every path of an OpenCode payload in each
// spelling the host may write it as, the path as sent first, so a deny of any
// of them denies the call. The plugin sends patch header paths and argument
// paths in one list, so every path gets the same treatment.
//
// OpenCode 2.0.10 trims a patch header path, as the plugin does, and keeps a
// TAB or CR inside it (its patch parser, run from the 2.0.10 binary); Codex
// 0.160.0 removes every TAB and CR. Each path is therefore also judged in the
// spellings of patchPathSpellings, so a release that removes them is covered.
// FileAccess.resolve, which every 2.0.10 file tool uses, turns a leading `~`
// or `~/` into the home directory before path.resolve, so that spelling is
// judged too.
func openCodeSpellings(paths []string) []string {
	var spellings []string
	add := func(p string) {
		if !slices.Contains(spellings, p) {
			spellings = append(spellings, p)
		}
	}
	for _, p := range paths {
		for _, spelling := range patchPathSpellings(p) {
			add(spelling)
			if home, ok := openCodeHome(spelling); ok {
				add(home)
			}
		}
	}
	return spellings
}

// openCodeHome is a path with a leading `~` or `~/` below the home directory,
// as OpenCode 2.0.10 expands it; ok is false for any other path or when the
// home directory is unknown.
func openCodeHome(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, "~")
	if !ok || rest != "" && !strings.HasPrefix(rest, "/") {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	return filepath.Join(home, filepath.FromSlash(rest)), true
}
