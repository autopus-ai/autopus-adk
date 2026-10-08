package learn

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/secretscan"
)

type writableItem struct {
	line   int
	isSkip bool
	entry  LearningEntry
	skip   SkipRecord
}

// rewriteStore rewrites the store file with the given entries and skips merged in order of line numbers.
//
// Parsed entries are re-encoded as stored, without redacting them again, so a
// stored value (and the fingerprint computed from it) survives prune. A line
// that never parsed never crossed the write boundary, so it is redacted here.
func rewriteStore(store *Store, entries []LearningEntry, skips []SkipRecord) error {
	f, err := os.Create(store.path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	var items []writableItem
	for _, e := range entries {
		items = append(items, writableItem{line: e.Line, isSkip: false, entry: e})
	}
	for _, s := range skips {
		items = append(items, writableItem{line: s.Line, isSkip: true, skip: s})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].line < items[j].line
	})

	for _, item := range items {
		if item.isSkip {
			raw, _ := secretscan.Redact(item.skip.Raw)
			if !strings.HasSuffix(raw, "\n") {
				raw += "\n"
			}
			if _, err := f.WriteString(raw); err != nil {
				return fmt.Errorf("write skip entry: %w", err)
			}
		} else {
			data, err := json.Marshal(item.entry)
			if err != nil {
				return fmt.Errorf("marshal entry: %w", err)
			}
			if _, err := f.Write(append(data, '\n')); err != nil {
				return fmt.Errorf("write entry: %w", err)
			}
		}
	}
	return nil
}
