package healthband

import "strings"

// Patch Policy item 1: the reply holds exactly one diff fence of at most
// 64 KiB, and a capture past its bound refuses the reply before any fence is
// read. Fences open and close at column 0 only, with three or more backticks
// or tildes; a fence whose info string's first word is diff is a diff fence,
// and every line inside another fence is that fence's content, so a diff
// fence nested in a Markdown example is not one.

// fenceOpen is an open fenced block.
type fenceOpen struct {
	char byte
	size int
	diff bool
}

// extractDiff returns the content of the reply's one diff fence, every line
// ending in LF, or the item 1 code.
func extractDiff(reply PatchReply) (string, string) {
	if reply.Dropped || len(reply.Text) > ProviderCaptureBytes {
		return "", PatchCodeTooLarge
	}
	diffs, closed := diffFences(reply.Text)
	if !closed || len(diffs) != 1 {
		return "", PatchCodeNoPatch
	}
	if len(diffs[0]) > PatchMaxDiffBytes {
		return "", PatchCodeTooLarge
	}
	return diffs[0], ""
}

// diffFences returns the content of every diff fence and whether every diff
// fence was closed. A line keeps a CR before its LF, so CRLF lines survive.
func diffFences(text string) ([]string, bool) {
	var (
		diffs []string
		open  *fenceOpen
		body  strings.Builder
	)
	for _, line := range strings.SplitAfter(text, "\n") {
		line = strings.TrimSuffix(line, "\n")
		switch {
		case open == nil:
			if fence, ok := openingFence(line); ok {
				open = &fence
				body.Reset()
			}
		case closesFence(line, *open):
			if open.diff {
				diffs = append(diffs, body.String())
			}
			open = nil
		case open.diff:
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	return diffs, open == nil || !open.diff
}

// openingFence reads a fence opening line. A backtick fence's info string
// holds no backtick, as in CommonMark.
func openingFence(line string) (fenceOpen, bool) {
	size := fenceRun(line)
	if size < 3 {
		return fenceOpen{}, false
	}
	info := strings.TrimSpace(line[size:])
	if line[0] == '`' && strings.Contains(info, "`") {
		return fenceOpen{}, false
	}
	words := strings.Fields(info)
	return fenceOpen{char: line[0], size: size, diff: len(words) > 0 && words[0] == "diff"}, true
}

// closesFence reports a run of the open fence's character at least as long
// as its opening, followed only by blanks.
func closesFence(line string, open fenceOpen) bool {
	size := fenceRun(line)
	return size >= open.size && line[0] == open.char && strings.TrimRight(line[size:], " \t\r") == ""
}

// fenceRun is the length of the run of backticks or tildes that starts line.
func fenceRun(line string) int {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0
	}
	size := 0
	for size < len(line) && line[size] == line[0] {
		size++
	}
	return size
}
