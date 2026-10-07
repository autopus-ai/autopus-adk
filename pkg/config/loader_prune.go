package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrRetiredKeyAnchor marks a document whose retired orchestra keys a rewrite
// cannot drop, because a retired entry defines a YAML anchor.
var ErrRetiredKeyAnchor = errors.New("retired orchestra keys define a YAML anchor")

// PruneRetiredKeysForRewrite is PruneRetiredKeys for a writer that encodes doc
// again. An alias resolves by name when encoded text is parsed again, so
// dropping an entry that defines an anchor would bind every later alias of
// that name to an earlier anchor of the same name, or to none: a retired value
// could silently become a live provider argv, or the file could stop parsing.
// Such a document is refused with an error that names each such path and
// anchor, and doc is left unchanged. Loading never needs this: the decoder
// follows an alias to the node it was parsed with.
func PruneRetiredKeysForRewrite(doc *yaml.Node) ([]string, error) {
	var anchored []string
	for _, key := range retiredOrchestraConfigKeys {
		for _, entry := range matchNodePaths(doc, strings.Split(key, ".")) {
			anchor := subtreeAnchor(entry.key)
			if anchor == "" {
				anchor = subtreeAnchor(entry.value)
			}
			if anchor != "" {
				anchored = append(anchored, fmt.Sprintf("%q (&%s)", entry.path, anchor))
			}
		}
	}
	if len(anchored) > 0 {
		return nil, fmt.Errorf("%w: %s; move the anchor to a kept key or delete the key by hand",
			ErrRetiredKeyAnchor, strings.Join(sortedUniquePaths(anchored), ", "))
	}
	return PruneRetiredKeys(doc), nil
}

// subtreeAnchor returns the first anchor that node's subtree defines; aliases
// are not followed.
func subtreeAnchor(node *yaml.Node) string {
	if node.Anchor != "" {
		return node.Anchor
	}
	for _, child := range node.Content {
		if anchor := subtreeAnchor(child); anchor != "" {
			return anchor
		}
	}
	return ""
}

// nodeEntry is one mapping entry that a removed-key path addresses.
type nodeEntry struct {
	parent, key, value *yaml.Node
	path               string
}

// pruneNodePaths deletes the mapping entries addressed by path and returns
// their concrete dotted paths.
func pruneNodePaths(doc *yaml.Node, path []string) []string {
	entries := matchNodePaths(doc, path)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		for i := 0; i+1 < len(entry.parent.Content); i += 2 {
			if entry.parent.Content[i] == entry.key {
				entry.parent.Content = append(entry.parent.Content[:i], entry.parent.Content[i+2:]...)
				break
			}
		}
		paths = append(paths, entry.path)
	}
	return paths
}

// matchNodePaths returns the mapping entries addressed by path in document
// order. A literal segment matches the first equal key, as it always has,
// except that a final segment two keys of one mapping equal matches neither,
// so the decoder still reports the duplicate key as it did at B. A "*"
// segment matches every scalar key at that depth. A merge key ("<<") is no
// segment: its inline mapping, or each inline mapping of its sequence, is
// walked as part of the mapping that holds it, as the decoder merges it, while
// an aliased merge value is shared with its anchor and is not walked from
// here. Only mapping nodes are walked, so a scalar, sequence, or missing node
// anywhere on the path is a miss rather than a panic.
func matchNodePaths(doc *yaml.Node, path []string) []nodeEntry {
	node := doc
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	return matchMappingPaths(node, path, nil)
}

func matchMappingPaths(node *yaml.Node, path, prefix []string) []nodeEntry {
	if node.Kind != yaml.MappingNode || len(path) == 0 {
		return nil
	}
	segment, rest := path[0], path[1:]
	wildcard := segment == "*"
	if !wildcard && len(rest) == 0 && countMappingKeys(node, segment) > 1 {
		return nil
	}
	var matched []nodeEntry
	literalMatched := false
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if isMergeKey(key) {
			matched = append(matched, matchMergedPaths(value, path, prefix)...)
			continue
		}
		if wildcard && key.Kind != yaml.ScalarNode || !wildcard && (literalMatched || key.Value != segment) {
			continue
		}
		literalMatched = !wildcard
		concrete := append(slices.Clone(prefix), key.Value)
		if len(rest) == 0 {
			matched = append(matched, nodeEntry{parent: node, key: key, value: value, path: strings.Join(concrete, ".")})
		} else {
			matched = append(matched, matchMappingPaths(value, rest, concrete)...)
		}
	}
	return matched
}

func matchMergedPaths(value *yaml.Node, path, prefix []string) []nodeEntry {
	if value.Kind != yaml.SequenceNode {
		return matchMappingPaths(value, path, prefix)
	}
	var matched []nodeEntry
	for _, item := range value.Content {
		matched = append(matched, matchMappingPaths(item, path, prefix)...)
	}
	return matched
}

func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.Value == "<<" && key.ShortTag() == "!!merge"
}

func countMappingKeys(node *yaml.Node, name string) int {
	count := 0
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == name {
			count++
		}
	}
	return count
}

// ParseConfig decodes raw autopus.yaml bytes through the steps Load applies to
// the file it reads: env placeholders expanded, strict decoding with the
// removed and reserved keys tolerated, missing defaults applied, platform
// names normalized, and Validate. It reads and writes no file and reports no
// retired key. A raw-node writer runs it on the bytes it is about to write, so
// a rewrite that Load would reject never lands.
func ParseConfig(data []byte) (*HarnessConfig, error) {
	expanded := []byte(expandEnvVars(string(data)))
	var cfg HarnessConfig
	if _, err := decodeStrict(expanded, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyMissingDefaults(&cfg, expanded)
	MigratePlatformNames(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, nil
}
