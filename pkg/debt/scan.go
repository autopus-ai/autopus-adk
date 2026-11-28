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
	"fmt"
	"os"
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

// DefaultMaxLines is the size limit when the project sets none.
const DefaultMaxLines = 300

var (
	// marker is a debt word right after a comment opener, in any language.
	marker = regexp.MustCompile(`(?://+|#+|/\*+|^\s*\*+|--|;+|<!--|%|\bREM\b)\s*(TODO|FIXME|HACK|XXX)\b[:(\s-]*(.*)`)
	// issueRef is tracked debt: an issue number, a ticket id or a link.
	issueRef = regexp.MustCompile(`#\d+|\b[A-Z][A-Z0-9]+-\d+\b|https?://`)
)

// Files lists the product source files of root's work tree: tracked and
// untracked files that are not ignored.
func Files(ctx context.Context, root string) ([]string, error) {
	return loadIndex(root).refresh(ctx, root, time.Time{})
}

// Options narrow a scan. MaxLines is the size limit (zero means
// DefaultMaxLines). SizeOf, when set, limits the near-limit count (the one
// costly measure) to the files it accepts: the Stop hook passes the changed
// files, since a near-limit file only warns.
type Options struct {
	MaxLines int
	SizeOf   func(file string) bool
	// Deadline bounds the reading of changed files (zero means none);
	// past it the scan returns ErrIncomplete and keeps what it read.
	Deadline time.Time
}

// Scan measures every debt item of root.
func Scan(ctx context.Context, root string, maxLines int) ([]Item, error) {
	return ScanWith(ctx, root, Options{MaxLines: maxLines})
}

// ScanWith is Scan with options. Unchanged files come from the index, so a
// repeated scan reads only what changed.
func ScanWith(ctx context.Context, root string, opt Options) ([]Item, error) {
	maxLines := opt.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	x := loadIndex(root)
	files, err := x.refresh(ctx, root, opt.Deadline)
	defer x.save(root)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, f := range files {
		e := x.Files[f]
		items = append(items, e.Markers...)
		// Counted lines never exceed physical lines, so only a file long
		// enough to be near the limit pays for the comment-aware count.
		if 10*e.Physical < 9*maxLines || (opt.SizeOf != nil && !opt.SizeOf(f)) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		if c, err := linecount.Source(f, data); err == nil && 10*c.Counted >= 9*maxLines {
			items = append(items, Item{Kind: KindNearLimit, Key: string(KindNearLimit) + "\x00" + f, File: f, Line: 1,
				Lines: c.Counted, Text: fmt.Sprintf("%d of %d lines", c.Counted, maxLines)})
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

// markers are the untracked debt markers of one file. A marker's key is its
// file and its normalized text, so it keeps its identity when it moves.
func markers(file string, lines []string) []Item {
	var out []Item
	seen := map[string]int{}
	for i, line := range lines {
		m := marker.FindStringSubmatch(line)
		if m == nil || issueRef.MatchString(m[0]) {
			continue
		}
		text := strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(m[2]), "*/")), " ")
		norm := m[1] + " " + text
		seen[norm]++
		out = append(out, Item{Kind: KindMarker, Key: fmt.Sprintf("%s\x00%s\x00%s\x00%d", KindMarker, file, norm, seen[norm]),
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
