package spec

import (
	"fmt"
	"strings"
)

// diffOp is one line of a line diff: ' ' kept, '-' removed, '+' added, with
// its 0-based index in the old and new text (-1 where it has none).
type diffOp struct {
	kind     byte
	text     string
	oldIndex int
	newIndex int
}

// DiffHunk is one unified-diff hunk of a changed document. Removed and Added
// hold the 0-based line indexes of the changed lines in the old and new text.
type DiffHunk struct {
	Header  string
	Lines   []string
	Removed []int
	Added   []int
}

// maxDiffCells bounds the LCS table; a larger changed region is diffed as a
// whole replacement, still a correct diff, only not a minimal one.
const maxDiffCells = 4_000_000

func splitLines(content string) []string {
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

func diffLines(oldLines, newLines []string) []diffOp {
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	ops := make([]diffOp, 0, len(oldLines)+len(newLines))
	for i := 0; i < prefix; i++ {
		ops = append(ops, diffOp{' ', oldLines[i], i, i})
	}
	ops = append(ops, diffMiddle(oldLines[prefix:len(oldLines)-suffix], newLines[prefix:len(newLines)-suffix], prefix)...)
	for i := suffix; i > 0; i-- {
		ops = append(ops, diffOp{' ', oldLines[len(oldLines)-i], len(oldLines) - i, len(newLines) - i})
	}
	return ops
}

// diffMiddle diffs the region between the common prefix and suffix, which
// starts at offset in both texts, by longest common subsequence.
func diffMiddle(a, b []string, offset int) []diffOp {
	n, m := len(a), len(b)
	var ops []diffOp
	if n*m > maxDiffCells || n == 0 || m == 0 {
		for i, line := range a {
			ops = append(ops, diffOp{'-', line, offset + i, -1})
		}
		for j, line := range b {
			ops = append(ops, diffOp{'+', line, -1, offset + j})
		}
		return ops
	}
	lcs := make([]int32, (n+1)*(m+1))
	at := func(i, j int) int32 { return lcs[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i*(m+1)+j] = at(i+1, j+1) + 1
			} else {
				lcs[i*(m+1)+j] = max(at(i+1, j), at(i, j+1))
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i], offset + i, offset + j})
			i, j = i+1, j+1
		case at(i+1, j) >= at(i, j+1):
			ops = append(ops, diffOp{'-', a[i], offset + i, -1})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j], -1, offset + j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i], offset + i, -1})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j], -1, offset + j})
	}
	return ops
}

// buildHunks groups changes closer than two context windows into unified
// hunks with context lines of kept text around them.
func buildHunks(ops []diffOp, context int) []DiffHunk {
	var changes []int
	for k, op := range ops {
		if op.kind != ' ' {
			changes = append(changes, k)
		}
	}
	var hunks []DiffHunk
	for start := 0; start < len(changes); {
		end := start
		for end+1 < len(changes) && changes[end+1]-changes[end] <= 2*context {
			end++
		}
		from, to := max(0, changes[start]-context), min(len(ops), changes[end]+context+1)
		hunks = append(hunks, newHunk(ops, from, to))
		start = end + 1
	}
	return hunks
}

func newHunk(ops []diffOp, from, to int) DiffHunk {
	oldStart, newStart := 0, 0
	for _, op := range ops[:from] {
		if op.kind != '+' {
			oldStart++
		}
		if op.kind != '-' {
			newStart++
		}
	}
	var hunk DiffHunk
	oldCount, newCount := 0, 0
	for _, op := range ops[from:to] {
		hunk.Lines = append(hunk.Lines, string(op.kind)+op.text)
		if op.kind != '+' {
			oldCount++
		}
		if op.kind != '-' {
			newCount++
		}
		switch op.kind {
		case '-':
			hunk.Removed = append(hunk.Removed, op.oldIndex)
		case '+':
			hunk.Added = append(hunk.Added, op.newIndex)
		}
	}
	hunk.Header = fmt.Sprintf("@@ -%d,%d +%d,%d @@", unifiedStart(oldStart, oldCount), oldCount, unifiedStart(newStart, newCount), newCount)
	return hunk
}

// unifiedStart is the 1-based first line of a hunk side; an empty side names
// the line before it, as diff -u does.
func unifiedStart(before, count int) int {
	if count == 0 {
		return before
	}
	return before + 1
}
