package debt

import (
	"fmt"
	"strings"
)

// A duplicate is a run of at least dupWindow code lines that appears in
// two or more places, compared with indentation and spacing folded. Short
// lines (braces, returns) and small windows are ignored: they repeat in any
// codebase without being debt.
const (
	dupWindow   = 6
	dupMinChars = 160
	dupMinLine  = 4
)

type loc struct {
	file string
	line int // index into the file's windows
}

// codeLines are a file's lines that can take part in a duplicate, with
// their line indexes.
func codeLines(lines []string) (text []string, at []int) {
	for i, l := range lines {
		t := strings.Join(strings.Fields(l), " ")
		if len(t) < dupMinLine || isCommentLine(t) {
			continue
		}
		text = append(text, t)
		at = append(at, i)
	}
	return text, at
}

func isCommentLine(t string) bool {
	for _, p := range []string{"//", "#", "/*", "*", "--", ";", "<!--"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// duplicates finds runs of code lines repeated across files. Each run is
// one item at its first place (by file and line), naming the others; its
// key is the run's window hashes, so it survives moving.
func duplicates(files []string, entries map[string]*fileEntry) []Item {
	places := map[uint64][]loc{}
	for _, f := range files {
		for i, w := range entries[f].Wins {
			if w.H != 0 {
				places[w.H] = append(places[w.H], loc{f, i})
			}
		}
	}
	dup := func(f string, i int) bool {
		ws := entries[f].Wins
		return i >= 0 && i < len(ws) && ws[i].H != 0 && len(places[ws[i].H]) > 1
	}
	var out []Item
	seen := map[string]bool{}
	for _, f := range files {
		ws := entries[f].Wins
		for i := range ws {
			if !dup(f, i) || dup(f, i-1) {
				continue // not the start of a repeated run
			}
			end := i
			for dup(f, end+1) {
				end++
			}
			var sig strings.Builder
			for _, w := range ws[i : end+1] {
				fmt.Fprintf(&sig, "%x.", w.H)
			}
			key := string(KindDuplicate) + "\x00" + digest(sig.String())
			if seen[key] {
				continue // reported at its first place
			}
			seen[key] = true
			at := places[ws[i].H]
			var also []string
			for _, p := range at {
				if p.file != f || p.line != i {
					also = append(also, fmt.Sprintf("%s:%d", p.file, entries[p.file].Wins[p.line].Start))
				}
			}
			n := ws[end].End - ws[i].Start + 1
			out = append(out, Item{Kind: KindDuplicate, Key: key, File: f, Line: ws[i].Start, Lines: n * len(at), Also: also})
		}
	}
	return out
}
