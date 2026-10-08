package healthband

import (
	"regexp"
	"strconv"
	"strings"
)

// Patch Policy items 2–3, in process: the diff is parsed the way git apply
// reads it, and every rule that gives patch_invalid without the base is
// checked on the way. A file section is a git header (`diff --git`, its
// extended header lines, then `---` and `+++`) or a traditional `---` and
// `+++` pair, followed by hunks. Hunk bodies are counted from the hunk
// header (`@@ -a[,b] +c[,d] @@`, an omitted count is 1): a line starting with
// a space, or an empty line as git reads it, counts on both sides, `-` on the
// old side, `+` on the new side, and `\ ` on neither; every `+` line of a
// body is an added line, even when it reads `+++`, and no line outside a
// body is content (CD-3 N4). Every rename, copy, deletion, mode change,
// symlink, gitlink, binary patch, new file whose mode is not 100644, header
// that names two paths, and line git would read otherwise refuses the diff.

// addedLine is one added line without its `+`; noEOL marks the last line of
// a file that a `\ No newline at end of file` marker leaves without its LF.
type addedLine struct {
	text  string
	noEOL bool
}

// diffFile is one file section of a diff. oldEnded and newEnded record a
// side that a no-newline marker ended.
type diffFile struct {
	path               string
	isNew              bool
	added              int
	removed            int
	lines              []addedLine
	oldEnded, newEnded bool
}

const (
	gitHeaderPrefix = "diff --git "
	hunkPrefix      = "@@ -"
	devNull         = "/dev/null"
	maxHunkCount    = 1 << 20
	minMarkerBytes  = 12 // git's shortest "\ " marker line, LF included
)

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]{1,9})(?:,([0-9]{1,9}))? \+([0-9]{1,9})(?:,([0-9]{1,9}))? @@`)

// refusedHeaders are the extended header lines of a rename, copy, deletion,
// mode change, rewrite, or binary patch (item 3).
var refusedHeaders = []string{
	"old mode ", "new mode ", "deleted file mode ", "copy from ", "copy to ", "rename from ", "rename to ",
	"rename old ", "rename new ", "similarity index ", "dissimilarity index ", "GIT binary patch", "Binary files ",
}

type diffParser struct {
	lines []string
	at    int
}

// parseDiff parses diff, whose every line ends in LF, into its file sections.
func parseDiff(diff string) ([]*diffFile, bool) {
	p := &diffParser{lines: strings.Split(strings.TrimSuffix(diff, "\n"), "\n")}
	var files []*diffFile
	seen := map[string]bool{}
	for p.skipBlankTail(); p.at < len(p.lines); p.skipBlankTail() {
		file, ok := p.section()
		if !ok || seen[file.path] {
			return nil, false
		}
		seen[file.path] = true
		files = append(files, file)
	}
	return files, len(files) > 0
}

// skipBlankTail moves past the rest of the diff when only empty lines remain.
func (p *diffParser) skipBlankTail() {
	for i := p.at; i < len(p.lines); i++ {
		if p.lines[i] != "" {
			return
		}
	}
	p.at = len(p.lines)
}

func (p *diffParser) next() (string, bool) {
	if p.at >= len(p.lines) {
		return "", false
	}
	p.at++
	return p.lines[p.at-1], true
}

func (p *diffParser) peek(prefix string) bool {
	return p.at < len(p.lines) && strings.HasPrefix(p.lines[p.at], prefix)
}

// section parses one file section and its hunks.
func (p *diffParser) section() (*diffFile, bool) {
	line, _ := p.next()
	file := &diffFile{}
	hasNames := false
	switch {
	case strings.HasPrefix(line, gitHeaderPrefix):
		path, ok := gitHeaderPath(line[len(gitHeaderPrefix):])
		if !ok || !p.extendedHeaders(file) {
			return nil, false
		}
		file.path = path
		if p.peek("--- ") {
			line, _ = p.next()
			hasNames = true
		}
	case strings.HasPrefix(line, "--- "):
		hasNames = true
	default:
		return nil, false
	}
	if hasNames && !p.names(file, line) {
		return nil, false
	}
	hunks := 0
	for ; p.peek(hunkPrefix) && hasNames; hunks++ {
		if !p.hunk(file) {
			return nil, false
		}
	}
	// Only a new empty file has no hunk; a following line must start the
	// next section or the blank tail.
	if (hunks == 0 && !(file.isNew && !hasNames)) || (p.at < len(p.lines) && !p.peek(gitHeaderPrefix) && !p.peek("--- ") && p.lines[p.at] != "") {
		return nil, false
	}
	return file, true
}

// extendedHeaders reads a git header's extended lines up to `---`, a hunk,
// the next section, or the end.
func (p *diffParser) extendedHeaders(file *diffFile) bool {
	for p.at < len(p.lines) && !p.peek("--- ") && !p.peek(gitHeaderPrefix) && p.lines[p.at] != "" {
		line, _ := p.next()
		for _, refused := range refusedHeaders {
			if strings.HasPrefix(line, refused) {
				return false
			}
		}
		switch {
		case line == "new file mode 100644":
			file.isNew = true
		case strings.HasPrefix(line, "index "):
			if !validIndexLine(line) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// validIndexLine accepts `index <a>..<b>` with an optional regular-file
// mode; the base entry decides between 100644 and 100755 (item 3).
func validIndexLine(line string) bool {
	fields := strings.Fields(strings.TrimPrefix(line, "index "))
	if len(fields) == 0 || len(fields) > 2 || !strings.Contains(fields[0], "..") {
		return false
	}
	return len(fields) == 1 || fields[1] == "100644" || fields[1] == "100755"
}

// names reads the `---` line already taken and the `+++` line after it: a
// new file has /dev/null on the old side, a modification the same path on
// both, and a deletion is refused.
func (p *diffParser) names(file *diffFile, oldLine string) bool {
	newLine, ok := p.next()
	if !ok || !strings.HasPrefix(newLine, "+++ ") {
		return false
	}
	oldName, okOld := headerName(oldLine[len("--- "):], "a/")
	newName, okNew := headerName(newLine[len("+++ "):], "b/")
	switch {
	case !okNew || newName == devNull || (file.path != "" && newName != file.path):
		return false
	case oldName == devNull && okOld:
		if file.path != "" && !file.isNew {
			return false
		}
		file.isNew = true
	case !okOld || oldName != newName || file.isNew:
		return false
	}
	file.path = newName
	return true
}

// headerName decodes a `---` or `+++` name: /dev/null, or prefix and a path,
// C-quoted when it starts with a quote.
func headerName(raw, prefix string) (string, bool) {
	if raw == devNull {
		return devNull, true
	}
	name := raw
	if strings.HasPrefix(raw, `"`) {
		decoded, used, ok := unquoteC(raw)
		if !ok || used != len(raw) {
			return "", false
		}
		name = decoded
	}
	path, ok := strings.CutPrefix(name, prefix)
	return path, ok && path != ""
}

// hunk reads one hunk header and its body.
func (p *diffParser) hunk(file *diffFile) bool {
	header, _ := p.next()
	match := hunkHeader.FindStringSubmatch(header)
	if match == nil {
		return false
	}
	oldCount, newCount := hunkCount(match[2]), hunkCount(match[4])
	if oldCount > maxHunkCount || newCount > maxHunkCount || (file.isNew && oldCount != 0) {
		return false
	}
	if file.oldEnded || file.newEnded {
		return false
	}
	changed := false
	for oldCount > 0 || newCount > 0 {
		line, ok := p.next()
		if !ok {
			return false
		}
		switch {
		case line == "" || line[0] == ' ':
			oldCount, newCount = oldCount-1, newCount-1
		case line[0] == '-':
			oldCount, file.removed, changed = oldCount-1, file.removed+1, true
		case line[0] == '+':
			newCount, file.added, changed = newCount-1, file.added+1, true
			file.lines = append(file.lines, addedLine{text: line[1:]})
		case !p.marker(file, line):
			return false
		}
		// A side that a marker ended takes no further line: git would join
		// it to the line before the marker.
		if oldCount < 0 || newCount < 0 || (file.oldEnded && oldSide(line)) || (file.newEnded && newSide(line)) {
			return false
		}
	}
	// A marker right after the body ends its last line without an LF.
	if p.peek(`\ `) {
		line, _ := p.next()
		if !p.marker(file, line) {
			return false
		}
	}
	return changed
}

// marker accepts a `\ No newline at end of file` line, which git knows only
// by its `\ ` start and length. git drops the LF of the line before it, so
// the marker ends that line's side of the file (both sides after a context
// line), and after an added line it marks that line.
func (p *diffParser) marker(file *diffFile, line string) bool {
	if !strings.HasPrefix(line, `\ `) || len(line)+1 < minMarkerBytes {
		return false
	}
	previous := p.lines[p.at-2]
	switch {
	case strings.HasPrefix(previous, "+") && len(file.lines) > 0:
		file.lines[len(file.lines)-1].noEOL, file.newEnded = true, true
	case strings.HasPrefix(previous, "-"):
		file.oldEnded = true
	case previous == "" || previous[0] == ' ':
		file.oldEnded, file.newEnded = true, true
	default: // a second marker, or one that opens the body
		return false
	}
	return true
}

// oldSide and newSide report body lines that count on that side.
func oldSide(line string) bool { return line == "" || line[0] == ' ' || line[0] == '-' }
func newSide(line string) bool { return line == "" || line[0] == ' ' || line[0] == '+' }

func hunkCount(raw string) int {
	if raw == "" {
		return 1
	}
	count, _ := strconv.Atoi(raw)
	return count
}
