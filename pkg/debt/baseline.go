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
	Version int    `json:"version"`
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

// Unknown are the items the baseline does not account for, compared one by
// one: identical markers by count (one deleted and one added is still new),
// near-limit files by path, and a duplicate as known when every window of
// it was in a baseline duplicate or it spans the same files as one (a copy
// edited, renamed in both, or partly extracted). When markers outnumber the
// baseline, the ones in changed files are the new ones.
func Unknown(items []Item, base Baseline, changed map[string]bool) []Item {
	known := map[string]bool{}
	markerCount := map[string]int{}
	baseWins := map[string]bool{}
	baseSets := map[string]bool{}
	for _, b := range base.Items {
		switch b.Kind {
		case KindMarker:
			markerCount[b.Key]++
		case KindDuplicate:
			for _, w := range b.Wins {
				baseWins[w] = true
			}
			baseSets[placeSet(b)] = true
		}
		known[b.Key] = true
	}
	var out []Item
	byText := map[string][]Item{}
	for _, it := range items {
		switch it.Kind {
		case KindMarker:
			byText[it.Key] = append(byText[it.Key], it)
		case KindDuplicate:
			if known[it.Key] || baseSets[placeSet(it)] || (len(it.Wins) > 0 && allIn(it.Wins, baseWins)) {
				continue
			}
			out = append(out, it)
		default:
			if !known[it.Key] {
				out = append(out, it)
			}
		}
	}
	for key, same := range byText {
		extra := len(same) - markerCount[key]
		if extra <= 0 {
			continue
		}
		sort.SliceStable(same, func(a, b int) bool { return changed[same[a].File] && !changed[same[b].File] })
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
	case err != nil:
		return 0, 0, err
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
	data, err := json.MarshalIndent(Baseline{Version: 1, Items: sorted}, "", "  ")
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
