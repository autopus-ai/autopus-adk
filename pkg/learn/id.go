package learn

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// An entry id is L-<sequence>, optionally followed by -<6 hex>. The store is
// committed, so two branches cut from the same main both see the same highest
// sequence and pick the same next number; the suffix, derived from the new
// entry's own content and timestamp, keeps those ids apart after a merge. The
// sequence stays first so ids still read and sort in recording order, and
// ids written before the suffix existed (L-001) remain valid.
var entryIDRegex = regexp.MustCompile(`^L-(\d{3,})(?:-([0-9a-f]{6}))?$`)

// IsValidEntryID reports whether id is L-NNN or L-NNN-xxxxxx.
func IsValidEntryID(id string) bool {
	return entryIDRegex.MatchString(id)
}

// ParseEntryID splits a valid id into its sequence number and suffix (empty
// for an id without one).
func ParseEntryID(id string) (seq int, suffix string, ok bool) {
	m := entryIDRegex.FindStringSubmatch(id)
	if m == nil {
		return 0, "", false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", false
	}
	return n, m[2], true
}

// EntryIDLess orders ids by sequence number, then by suffix, so L-999
// precedes L-1000 and the two L-004 ids of a merge sort next to each other.
// An invalid id sorts after every valid one, by its text.
func EntryIDLess(a, b string) bool {
	na, sa, okA := ParseEntryID(a)
	nb, sb, okB := ParseEntryID(b)
	switch {
	case okA != okB:
		return okA
	case !okA:
		return a < b
	case na != nb:
		return na < nb
	}
	return sa < sb
}

// entrySuffix is the branch-unique part of a new id: the first six hex of a
// hash over the timestamp and the entry's describing fields.
func entrySuffix(e LearningEntry) string {
	h := sha256.New()
	for _, part := range []string{e.Timestamp.Format(time.RFC3339Nano), string(e.Type), e.Phase, e.SpecID,
		e.Pattern, strings.Join(e.Files, "\x01"), strings.Join(e.Packages, "\x01")} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:6]
}

// dedupeEntries keeps one entry per id. The store merges with `merge=union`,
// which keeps both sides of a conflicting hunk: the same line twice, or two
// versions of an entry that a branch rewrote (reuse_count). The version with
// the highest reuse count wins, the first on a tie, at the position of the
// id's first line. Entries without an id are kept as they are.
func dedupeEntries(entries []LearningEntry) []LearningEntry {
	at := make(map[string]int, len(entries))
	out := entries[:0]
	for _, e := range entries {
		if e.ID == "" {
			out = append(out, e)
			continue
		}
		i, seen := at[e.ID]
		if !seen {
			at[e.ID] = len(out)
			out = append(out, e)
			continue
		}
		if e.ReuseCount > out[i].ReuseCount {
			e.Line = out[i].Line
			out[i] = e
		}
	}
	return out
}
