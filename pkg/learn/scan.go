package learn

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"
)

// StoreLine is one parsed store line and its trimmed raw text.
type StoreLine struct {
	Entry LearningEntry
	Raw   string
}

// IDConflict is an id that more than one distinct lesson holds: lessons with
// the same id and different timestamps, as two writers that predate the id
// suffix produce on two branches. Both lessons are kept.
type IDConflict struct {
	ID    string
	Lines []int
}

// ScanStore parses the store's JSONL tolerantly and merges the copies a
// union merge leaves behind (see mergeCopies). Every reader of the store
// uses it, so they agree on which line represents an entry.
func ScanStore(r io.Reader) ([]StoreLine, []SkipRecord, error) {
	var lines []StoreLine
	var skips []SkipRecord
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry LearningEntry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			skips = append(skips, SkipRecord{Line: lineNum, Raw: line, Reason: err.Error()})
			continue
		}
		entry.Line = lineNum
		lines = append(lines, StoreLine{Entry: entry, Raw: trimmed})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return mergeCopies(lines), skips, nil
}

// copyKey identifies one lesson across its copies. The timestamp is set once
// at recording and never rewritten, so two copies of one lesson share it and
// two lessons that happen to share an old-form id do not.
func copyKey(e LearningEntry) string {
	return e.ID + "\x00" + e.Timestamp.UTC().Format(time.RFC3339Nano)
}

// mergeCopies keeps one line per lesson. The store merges with
// `merge=union`, which keeps both sides of a conflicting hunk: the same line
// twice, or two versions of a lesson a branch rewrote (reuse_count). The
// version with the highest reuse count wins, the first on a tie, at the
// position of the lesson's first line. Lines without an id are kept as they
// are.
func mergeCopies(lines []StoreLine) []StoreLine {
	at := make(map[string]int, len(lines))
	out := lines[:0]
	for _, l := range lines {
		if l.Entry.ID == "" {
			out = append(out, l)
			continue
		}
		key := copyKey(l.Entry)
		i, seen := at[key]
		if !seen {
			at[key] = len(out)
			out = append(out, l)
			continue
		}
		if l.Entry.ReuseCount > out[i].Entry.ReuseCount {
			l.Entry.Line = out[i].Entry.Line
			out[i] = l
		}
	}
	return out
}

// IDConflicts reports the ids that more than one distinct lesson holds,
// sorted by id. A command that addresses such an id acts on every holder.
func IDConflicts(entries []LearningEntry) []IDConflict {
	byID := map[string][]int{}
	for _, e := range entries {
		if e.ID != "" {
			byID[e.ID] = append(byID[e.ID], e.Line)
		}
	}
	var out []IDConflict
	for id, lines := range byID {
		if len(lines) > 1 {
			out = append(out, IDConflict{ID: id, Lines: lines})
		}
	}
	sort.Slice(out, func(i, j int) bool { return EntryIDLess(out[i].ID, out[j].ID) })
	return out
}
