package verifygate

import (
	"bufio"
	"bytes"
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
		var file string
		var line int
		inHunk := false
		scanner := bufio.NewScanner(bytes.NewReader(diff))
		scanner.Buffer(make([]byte, 0, 64*1024), maxUntrackedBytes)
		for scanner.Scan() {
			text := scanner.Text()
			switch {
			case strings.HasPrefix(text, "diff --git "):
				file, inHunk = "", false
			case !inHunk && strings.HasPrefix(text, "+++ "):
				file = diffPath(strings.TrimPrefix(text, "+++ "))
				if len(withoutHarnessPaths([]string{file})) == 0 {
					file = ""
				}
			case strings.HasPrefix(text, "@@ "):
				inHunk, line = true, hunkStart(text)
			case inHunk && strings.HasPrefix(text, "+"):
				if file != "" {
					if a.Lines[file] == nil {
						a.Lines[file] = map[int]bool{}
					}
					a.Lines[file][line] = true
				}
				line++
			}
		}
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
