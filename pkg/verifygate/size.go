package verifygate

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ChangedLines counts added plus deleted lines against HEAD, counting every
// line of an untracked file as added. Binary files count as zero.
func ChangedLines(ctx context.Context, root string, changed []string) int {
	total := 0
	tracked := map[string]bool{}
	if out, err := gitOutput(ctx, root, "diff", "--numstat", "HEAD", "--"); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(out))
		for sc.Scan() {
			fields := strings.SplitN(sc.Text(), "\t", 3)
			if len(fields) < 3 {
				continue
			}
			added, _ := strconv.Atoi(fields[0])
			deleted, _ := strconv.Atoi(fields[1])
			total += added + deleted
			tracked[fields[2]] = true
		}
	}
	for _, f := range changed {
		if tracked[f] {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil && !bytes.Contains(data, []byte{0}) {
			total += bytes.Count(data, []byte{'\n'})
		}
	}
	return total
}
