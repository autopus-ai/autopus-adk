// Package debt measures technical debt as items code can count, so a change
// that adds one is caught the same way every time: debt markers in
// comments, product files near the size limit, and code blocks repeated in
// several places. A committed baseline records the items a project already
// carries; only items it lacks are new.
package debt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/linecount"
)

// Kind is the sort of a debt item.
type Kind string

const (
	KindMarker    Kind = "marker"     // TODO, FIXME, HACK or XXX in a comment, with no issue reference
	KindNearLimit Kind = "near-limit" // a product file within a tenth of the size limit
	KindDuplicate Kind = "duplicate"  // a code block repeated in several places
)

// Item is one debt item. Key names it apart from its line, so code moved
// up or down does not make it new.
type Item struct {
	Kind  Kind     `json:"kind"`
	Key   string   `json:"key"`
	File  string   `json:"file"`
	Line  int      `json:"line"`
	Lines int      `json:"lines"` // the lines involved: the cleanup order
	Text  string   `json:"text,omitempty"`
	Also  []string `json:"also,omitempty"` // the other places of a duplicate
	// Wins are a duplicate's window hashes: a run that only lost windows
	// (one copy edited, part extracted) is the same debt, not new debt.
	Wins []string `json:"wins,omitempty"`
	// Ends are the last line of each place (File first, then Also): copies
	// with comments in between span different lines.
	Ends []int `json:"ends,omitempty"`
}

// Places are the files an item is in: a duplicate's every copy.
func (i Item) Places() []string {
	out := []string{i.File}
	for _, a := range i.Also {
		f, _, _ := strings.Cut(a, ":")
		out = append(out, f)
	}
	return out
}

func (i Item) String() string {
	s := fmt.Sprintf("%s:%d %s", i.File, i.Line, i.Kind)
	switch i.Kind {
	case KindMarker:
		s += " " + i.Text
	case KindNearLimit:
		s += fmt.Sprintf(" (%s)", i.Text)
	case KindDuplicate:
		s += fmt.Sprintf(" (%d lines, also at %s)", i.Lines, strings.Join(i.Also, ", "))
	}
	return s
}

var (
	// markerWord is a debt word after a comment opener and optional spaces.
	markerWord = regexp.MustCompile(`^\s*(TODO|FIXME|HACK|XXX)\b[:(\s-]*(.*)`)
	// issueRef is tracked debt: an issue number, a link, or a ticket id the
	// marker names in parentheses or right after it (TODO(ABC-12), TODO
	// ABC-12:); a ticket-shaped word later in the text (IE-11, Q3-2027) is not.
	issueRef = regexp.MustCompile(`#\d+|https?://|^\(?\s*[A-Z][A-Z0-9]+-\d+\b`)
)

// openers are the line-comment and block-comment starts of each language;
// a debt word counts only right after one of its file's openers.
func openers(file string) []string {
	switch strings.ToLower(path.Ext(file)) {
	case ".py", ".rb", ".sh", ".bash", ".zsh", ".pl", ".pm", ".r", ".yaml", ".yml", ".toml", ".ex", ".exs", ".jl", ".nim", ".ps1", ".tf":
		return []string{"#"}
	case ".sql", ".lua", ".hs":
		return []string{"--"}
	case ".clj", ".lisp", ".el", ".scm", ".asm", ".s":
		return []string{";"}
	case ".erl", ".tex":
		return []string{"%"}
	case ".m": // Objective-C or MATLAB
		return []string{"//", "/*", "%"}
	case ".ml", ".mli":
		return []string{"(*"}
	case ".html", ".xml", ".vue", ".svelte":
		return []string{"<!--", "//", "/*"}
	case ".php":
		return []string{"//", "#", "/*"}
	case ".bat":
		return []string{"REM ", "::"}
	case ".vb":
		return []string{"'"}
	}
	return []string{"//", "/*", "*"}
}

// commentAt finds a debt marker in line: after one of the file's comment
// openers that is not inside a string literal or part of a URL.
func commentAt(line string, opens []string) (word, rest string, ok bool) {
	for _, op := range opens {
		for from := 0; from < len(line); {
			i := strings.Index(line[from:], op)
			if i < 0 {
				break
			}
			i += from
			from = i + len(op)
			if op == "*" && strings.TrimSpace(line[:i]) != "" {
				continue // a block-comment continuation starts the line
			}
			if op == "//" && i > 0 && line[i-1] == ':' {
				continue // a URL, not a comment
			}
			if inString(line[:i]) {
				continue
			}
			m := markerWord.FindStringSubmatch(strings.TrimLeft(line[from:], op[len(op)-1:]))
			if m != nil {
				return m[1], m[2], true
			}
		}
	}
	return "", "", false
}

// inString reports whether position len(prefix) is inside a quoted string:
// it follows the quote that opened, so an apostrophe inside "can't" opens
// nothing.
func inString(prefix string) bool {
	var open byte
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		switch {
		case open != 0 && c == '\\' && open != '`':
			i++
		case open != 0 && c == open:
			open = 0
		case open == 0 && (c == '"' || c == '\'' || c == '`'):
			open = c
		}
	}
	return open != 0
}

// Options narrow a scan. SizeOf, when set, limits the near-limit count (the one
// costly measure) to the files it accepts: the Stop hook passes the changed
// files, since a near-limit file only warns.
type Options struct {
	// MaxLines is the project's declared size limit; zero means none, and
	// then nothing is near a limit (an undeclared ceiling is advisory).
	MaxLines int
	SizeOf   func(file string) bool
	// Deadline bounds the reading of changed files (zero means none);
	// past it the scan returns ErrIncomplete and keeps what it read.
	Deadline time.Time
}

// Scan measures every debt item of root; maxLines is the declared size
// limit (zero: none).
func Scan(ctx context.Context, root string, maxLines int) ([]Item, error) {
	return ScanWith(ctx, root, Options{MaxLines: maxLines})
}

// ScanWith is Scan with options. Unchanged files come from the index, so a
// repeated scan reads only what changed.
func ScanWith(ctx context.Context, root string, opt Options) ([]Item, error) {
	maxLines := opt.MaxLines
	x := loadIndex(root)
	files, err := x.refresh(ctx, root, opt.Deadline)
	if errors.Is(err, ErrIncomplete) {
		x.save(root) // keep what was read for the next scan
	}
	if err != nil {
		return nil, err
	}
	defer x.save(root)
	var items []Item
	for _, f := range files {
		e := x.Files[f]
		items = append(items, e.Markers...)
		// Counted lines never exceed physical lines, so only a file long
		// enough to be near the limit pays for the comment-aware count.
		if maxLines <= 0 || 10*e.Physical < 9*maxLines || (opt.SizeOf != nil && !opt.SizeOf(f)) {
			continue
		}
		if e.Counted < 0 {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
			if err != nil {
				continue
			}
			c, err := linecount.Source(f, data)
			if err != nil {
				continue
			}
			e.Counted, x.dirty = c.Counted, true // kept until the file changes
		}
		if 10*e.Counted >= 9*maxLines {
			// Lines is how far into the last tenth it is: near the limit is
			// not yet a mistake, so it must not crowd the cleanup order.
			items = append(items, Item{Kind: KindNearLimit, Key: string(KindNearLimit) + "\x00" + strings.ToValidUTF8(f, "?"), File: f, Line: 1,
				Lines: e.Counted - (9*maxLines+9)/10 + 1, Text: fmt.Sprintf("%d of %d lines", e.Counted, maxLines)})
		}
	}
	items = append(items, duplicates(files, x.Files)...)
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].File != items[b].File {
			return items[a].File < items[b].File
		}
		return items[a].Line < items[b].Line
	})
	return items, nil
}

// markers are the untracked debt markers of one file. A marker's key is
// its normalized text alone, so it keeps its identity when its line or its
// file moves; identical markers are counted (see Unknown).
func markers(file string, lines []string) []Item { return Markers(file, lines) }

// Markers are the untracked debt markers of one file's lines.
func Markers(file string, lines []string) []Item {
	var out []Item
	opens := openers(file)
	for i, line := range lines {
		word, rest, ok := commentAt(line, opens)
		if !ok {
			continue
		}
		if issueRef.MatchString(strings.TrimSpace(rest)) {
			continue
		}
		text := strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(rest), "*/")), " ")
		norm := strings.ToValidUTF8(word+" "+text, "?")
		out = append(out, Item{Kind: KindMarker, Key: string(KindMarker) + "\x00" + digest(norm),
			File: file, Line: i + 1, Lines: 1, Text: truncate(norm, 80)})
	}
	return out
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}
