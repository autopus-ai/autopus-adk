package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// pruneRetiredConfig removes the retired orchestra keys (SPEC-PANERM-001 group
// K) from raw autopus.yaml bytes and returns the result with the concrete
// dotted paths it removed, in byte order. Bytes that hold none come back
// unchanged, so a writer keeps every byte it kept before the retirement.
// Otherwise the lines of each retired entry are cut and nothing else changes.
// When an entry does not own whole lines (a flow mapping, a mapping the cut
// would leave without a value), the document is re-encoded from its node tree
// instead; that keeps comments, quoting, env placeholders, and reserved blocks,
// though not blank lines or indentation widths (REQ-10). A retired entry that
// defines a YAML anchor is refused (config.ErrRetiredKeyAnchor), and either
// rewrite must parse back to the pruned tree with every alias resolving to the
// same data, or nothing is returned.
func pruneRetiredConfig(data []byte) ([]byte, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse config: %w", err)
	}
	entries := yamlMappingEntries(&doc)
	paths, err := config.PruneRetiredKeysForRewrite(&doc)
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		return data, nil, nil
	}
	if cut, ok := cutRetiredEntryLines(data, removedYAMLEntries(&doc, entries)); ok {
		var check yaml.Node
		if yaml.Unmarshal(cut, &check) == nil && sameYAMLData(&check, &doc) {
			return cut, paths, nil
		}
	}
	encoded, err := yaml.Marshal(&doc)
	if err == nil {
		var check yaml.Node
		if err = yaml.Unmarshal(encoded, &check); err == nil && !sameYAMLData(&check, &doc) {
			err = errors.New("the re-encoded document does not parse back to the same data")
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("remove retired orchestra keys %s: %w", strings.Join(paths, ", "), err)
	}
	return encoded, paths, nil
}

// pruneRetiredConfigFile removes the retired orchestra keys from dir's
// autopus.yaml in place and returns the paths it removed. A missing file or
// one without such keys is not written.
func pruneRetiredConfigFile(dir string) ([]string, error) {
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	pruned, paths, err := pruneRetiredConfig(data)
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	if err := atomicWriteQualityConfig(path, pruned); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}
	return paths, nil
}

// retiredConfigKeysInFile lists the retired orchestra keys that dir's
// autopus.yaml holds, for previews that must announce the rewrite.
func retiredConfigKeysInFile(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	if err != nil {
		return nil
	}
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil {
		return nil
	}
	return config.PruneRetiredKeys(&doc)
}

type yamlMappingEntry struct{ parent, key *yaml.Node }

// yamlMappingEntries lists every mapping entry under node in document order.
// Aliases are not followed, so a cyclic document terminates.
func yamlMappingEntries(node *yaml.Node) []yamlMappingEntry {
	var entries []yamlMappingEntry
	var walk func(*yaml.Node)
	walk = func(current *yaml.Node) {
		if current.Kind != yaml.MappingNode {
			for _, child := range current.Content {
				walk(child)
			}
			return
		}
		for i := 0; i+1 < len(current.Content); i += 2 {
			entries = append(entries, yamlMappingEntry{parent: current, key: current.Content[i]})
			walk(current.Content[i+1])
		}
	}
	walk(node)
	return entries
}

// removedYAMLEntries returns the entries a prune detached from doc whose
// mapping is still part of doc, that is the outermost removed entries.
func removedYAMLEntries(doc *yaml.Node, before []yamlMappingEntry) []yamlMappingEntry {
	live := map[*yaml.Node]bool{}
	var walk func(*yaml.Node)
	walk = func(node *yaml.Node) {
		live[node] = true
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(doc)
	var removed []yamlMappingEntry
	for _, entry := range before {
		if !live[entry.key] && live[entry.parent] {
			removed = append(removed, entry)
		}
	}
	return removed
}

// cutRetiredEntryLines drops each entry's lines: its key line, then every line
// up to the next one that is neither blank, a comment, nor indented deeper
// than the key (a block sequence may sit at the key's column). Trailing blank
// and comment lines stay with whatever follows. It refuses an entry that
// shares its first line with other content.
func cutRetiredEntryLines(data []byte, removed []yamlMappingEntry) ([]byte, bool) {
	lines := strings.SplitAfter(string(data), "\n")
	drop := make([]bool, len(lines))
	for _, entry := range removed {
		start, column := entry.key.Line-1, entry.key.Column-1
		if entry.parent.Style&yaml.FlowStyle != 0 || start < 0 || start >= len(lines) ||
			leadingSpaces(lines[start]) != column {
			return nil, false
		}
		end := start + 1
		for end < len(lines) && !endsYAMLEntry(lines[end], column) {
			end++
		}
		for end > start+1 && isBlankOrCommentLine(lines[end-1]) {
			end--
		}
		for i := start; i < end; i++ {
			drop[i] = true
		}
	}
	var kept strings.Builder
	for i, line := range lines {
		if !drop[i] {
			kept.WriteString(line)
		}
	}
	return []byte(kept.String()), true
}

func endsYAMLEntry(line string, column int) bool {
	if isBlankOrCommentLine(line) {
		return false
	}
	indent := leadingSpaces(line)
	trimmed := strings.TrimSpace(line)
	return indent < column || indent == column && trimmed != "-" && !strings.HasPrefix(trimmed, "- ")
}

func isBlankOrCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

// sameYAMLData reports whether two node trees hold the same data: kinds,
// resolved tags, values, and anchors in the same order, with every alias also
// compared by the node it resolves to, not only by its name, so a rewrite that
// rebinds an alias to another anchor of the same name differs. Comments,
// styles, and positions are ignored. Each node pair is compared once, so a
// cyclic or alias-heavy document stays linear.
func sameYAMLData(a, b *yaml.Node) bool {
	return sameYAMLNode(a, b, map[[2]*yaml.Node]bool{})
}

func sameYAMLNode(a, b *yaml.Node, seen map[[2]*yaml.Node]bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	pair := [2]*yaml.Node{a, b}
	if seen[pair] {
		return true
	}
	seen[pair] = true
	if a.Kind != b.Kind || a.ShortTag() != b.ShortTag() || a.Value != b.Value || a.Anchor != b.Anchor ||
		len(a.Content) != len(b.Content) {
		return false
	}
	if a.Kind == yaml.AliasNode && !sameYAMLNode(a.Alias, b.Alias, seen) {
		return false
	}
	for i := range a.Content {
		if !sameYAMLNode(a.Content[i], b.Content[i], seen) {
			return false
		}
	}
	return true
}
