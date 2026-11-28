package debt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BaselinePath is where a project commits the debt it already carries.
const BaselinePath = ".autopus/debt/baseline.json"

// ErrNoBaseline means the project has not committed a baseline yet.
var ErrNoBaseline = errors.New("no debt baseline: run `auto debt baseline` and commit " + BaselinePath)

// Baseline is the committed record of known debt.
type Baseline struct {
	Version int    `json:"version"` // 2: markers keyed by text alone, duplicates with their windows
	Items   []Item `json:"items"`
}

// LoadBaseline reads root's baseline.
func LoadBaseline(root string) (Baseline, error) {
	var b Baseline
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(BaselinePath)))
	if errors.Is(err, os.ErrNotExist) {
		return b, ErrNoBaseline
	}
	if err != nil {
		return b, err
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("debt: %s: %w", BaselinePath, err)
	}
	return b, nil
}

// New are the items not in the baseline (see Unknown).
func New(items []Item, base Baseline) []Item { return Unknown(items, base, nil) }

// InChange reports an item a change added; Unknown uses it to decide which
// of several identical markers are the new ones.
type InChange func(Item) bool

// Unknown are the items the baseline does not account for, compared one by
// one: identical markers by count (one deleted and one added is still new),
// near-limit files by path, and a duplicate as known when every window of
// it was in a baseline duplicate, or it spans the same files as one and has
// not grown (a copy edited, or renamed in both). When markers outnumber the
// baseline, the ones the change added (inChange) are the new ones.
func Unknown(items []Item, base Baseline, inChange InChange) []Item {
	known := map[string]bool{}
	markerCount := map[string]int{}
	baseWins := map[string]bool{}
	setCount := map[string]int{}
	setLines := map[string]int{}
	for _, b := range base.Items {
		switch b.Kind {
		case KindMarker:
			markerCount[b.Key]++
		case KindDuplicate:
			for _, w := range b.Wins {
				baseWins[w] = true
			}
			setCount[placeSet(b)]++
			setLines[placeSet(b)] = max(setLines[placeSet(b)], b.Lines)
		}
		known[b.Key] = true
	}
	var out []Item
	byText := map[string][]Item{}
	var dups []Item
	for _, it := range items {
		switch it.Kind {
		case KindMarker:
			byText[it.Key] = append(byText[it.Key], it)
		case KindDuplicate:
			dups = append(dups, it)
		default:
			if !known[it.Key] {
				out = append(out, it)
			}
		}
	}
	// A duplicate is known by its windows; failing that, by the files it
	// spans, but only as many duplicates per set of files as the baseline
	// had there and none larger (an edited or renamed copy, not a new one).
	used := map[string]int{}
	var byFiles []Item
	for _, it := range dups {
		if known[it.Key] || (len(it.Wins) > 0 && allIn(it.Wins, baseWins)) {
			used[placeSet(it)]++
			continue
		}
		byFiles = append(byFiles, it)
	}
	for _, it := range byFiles {
		set := placeSet(it)
		if it.Lines <= setLines[set] && used[set] < setCount[set] {
			used[set]++
			continue
		}
		out = append(out, it)
	}
	for key, same := range byText {
		extra := len(same) - markerCount[key]
		if extra <= 0 {
			continue
		}
		if inChange != nil {
			sort.SliceStable(same, func(a, b int) bool { return inChange(same[a]) && !inChange(same[b]) })
		}
		out = append(out, same[:extra]...)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].File != out[b].File {
			return out[a].File < out[b].File
		}
		return out[a].Line < out[b].Line
	})
	return out
}

func placeSet(it Item) string {
	p := it.Places()
	sort.Strings(p)
	return strings.Join(p, "\x00")
}

func allIn(ws []string, set map[string]bool) bool {
	for _, w := range ws {
		if !set[w] {
			return false
		}
	}
	return true
}

// Blocking are the new items that send a change back; a file near the
// size limit only warns.
func Blocking(items []Item) (block, warn []Item) {
	for _, it := range items {
		if it.Kind == KindNearLimit {
			warn = append(warn, it)
		} else {
			block = append(block, it)
		}
	}
	return block, warn
}

// ErrNewDebt refuses a baseline rewrite that would accept new items.
var ErrNewDebt = errors.New("the work tree has debt items the baseline does not: remove them, or pass --force to record them")

// WriteBaseline records items as root's baseline. Over an existing baseline
// it refuses to accept new items unless force: the baseline only shrinks on
// its own.
func WriteBaseline(root string, items []Item, force bool) (added, removed int, err error) {
	old, err := LoadBaseline(root)
	switch {
	case errors.Is(err, ErrNoBaseline):
	case err != nil && force:
		// An unreadable baseline (a merge conflict in it) is rewritten from
		// the tree, by a person who says so.
	case err != nil:
		return 0, 0, fmt.Errorf("%w; fix the file, or rewrite it from the tree with --force", err)
	default:
		blockNew, _ := Blocking(New(items, old))
		if len(blockNew) > 0 && !force {
			return len(blockNew), 0, ErrNewDebt
		}
		added = len(New(items, old))
		removed = len(New(old.Items, Baseline{Items: items}))
	}
	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].Key < sorted[b].Key })
	data, err := json.MarshalIndent(Baseline{Version: 2, Items: sorted}, "", "  ")
	if err != nil {
		return 0, 0, err
	}
	dst := filepath.Join(root, filepath.FromSlash(BaselinePath))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, 0, err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return 0, 0, err
	}
	return added, removed, os.Rename(tmp, dst)
}
