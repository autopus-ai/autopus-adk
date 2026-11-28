package verifygate

import (
	"context"
	"strings"
)

// Added are the lines a change adds or edits against HEAD: per file, the
// new-side line numbers of `git diff --unified=0 HEAD`, and every line of
// an untracked file that is not ignored.
type Added struct {
	Lines map[string]map[int]bool
	Whole map[string]bool
}

// Has reports whether line of file is added or edited.
func (a Added) Has(file string, line int) bool { return a.Whole[file] || a.Lines[file][line] }

// Touches reports whether any of lines from..to of file is added or edited.
func (a Added) Touches(file string, from, to int) bool {
	if a.Whole[file] {
		return true
	}
	for l := from; l <= to; l++ {
		if a.Lines[file][l] {
			return true
		}
	}
	return false
}

// AddedLines reads the lines root's work tree adds or edits against HEAD,
// harness directories excluded. Errors leave the result empty.
func AddedLines(ctx context.Context, root string) Added {
	a := Added{Lines: map[string]map[int]bool{}, Whole: map[string]bool{}}
	if diff, err := gitOutput(ctx, root, "-c", "core.quotePath=false", "diff", "--unified=0", "--no-color",
		"--no-ext-diff", "--no-renames", "HEAD"); err == nil {
		walkAdded(diff, int(^uint(0)>>1), func(file string, line int, _ string) {
			if a.Lines[file] == nil {
				a.Lines[file] = map[int]bool{}
			}
			a.Lines[file][line] = true
		})
	}
	if out, err := gitOutput(ctx, root, "ls-files", "--others", "--exclude-standard", "-z"); err == nil {
		for _, f := range withoutHarnessPaths(strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")) {
			if f != "" {
				a.Whole[f] = true
			}
		}
	}
	return a
}
