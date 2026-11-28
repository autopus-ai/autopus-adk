package debt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// New are the items not in the baseline. Items are compared one by one by
// key, so removing one and adding another is still a new item.
func New(items []Item, base Baseline) []Item {
	known := map[string]bool{}
	for _, b := range base.Items {
		known[b.Key] = true
	}
	var out []Item
	for _, it := range items {
		if !known[it.Key] {
			out = append(out, it)
		}
	}
	return out
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
