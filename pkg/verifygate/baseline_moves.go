package verifygate

import (
	"bytes"
	"context"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// fileMoves maps the base name of each Go file the change moved to its new
// base name: a rename git records (R), or a deleted file whose content an
// untracked file has unchanged (a plain mv). A failure line names the file
// by its base name ("pre_test.go:6: ..."), so HEAD's line for a moved test
// names the old file and would never match the same failure now.
func fileMoves(ctx context.Context, root string) map[string]string {
	moves := map[string]string{}
	out, err := gitOutput(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return moves
	}
	var deleted, added []string
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		status, p := string(entry[:2]), string(entry[3:])
		switch {
		case status[0] == 'R' && i+1 < len(fields):
			i++
			moves[path.Base(string(fields[i]))] = path.Base(p)
		case status[0] == 'C':
			i++
		case status == "??":
			added = append(added, p)
		case status[0] == 'D' || status[1] == 'D':
			deleted = append(deleted, p)
		}
	}
	for _, old := range deleted {
		if !strings.HasSuffix(old, ".go") {
			continue
		}
		before, err := gitOutput(ctx, root, "show", "HEAD:"+old)
		if err != nil {
			continue
		}
		for _, p := range added {
			if !strings.HasSuffix(p, ".go") {
				continue
			}
			if now, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))); err == nil && bytes.Equal(now, before) {
				moves[path.Base(old)] = path.Base(p)
				break
			}
		}
	}
	for old, now := range moves {
		if old == now {
			delete(moves, old)
		}
	}
	return moves
}

// lineFile is the file a failure line starts with: "x_test.go:6: ..." or
// "./pkg/x.go:3:2: ...".
var lineFile = regexp.MustCompile(`^((?:[^\s:]*/)?)([^/\s:]+\.go)(:\d)`)

// movedRuns rewrites HEAD's failure lines to name a moved file by its new
// name, so the same failure from the moved file reads the same.
func movedRuns(runs map[string][]testRun, moves map[string]string) map[string][]testRun {
	if len(moves) == 0 {
		return runs
	}
	out := make(map[string][]testRun, len(runs))
	for key, list := range runs {
		for _, r := range list {
			lines := make([]string, 0, len(r.Lines))
			for _, l := range r.Lines {
				if m := lineFile.FindStringSubmatchIndex(l); m != nil {
					if now, ok := moves[l[m[4]:m[5]]]; ok {
						l = l[:m[4]] + now + l[m[5]:]
					}
				}
				lines = append(lines, l)
			}
			out[key] = append(out[key], testRun{Failed: r.Failed, Lines: lines})
		}
	}
	return out
}
