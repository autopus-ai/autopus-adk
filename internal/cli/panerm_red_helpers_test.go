package cli_test

// SPEC-PANERM-001 T3: helpers shared by the internal/cli red oracles for S6,
// S7, S12, and S13. They read the legacy config fixtures of pkg/config
// (testdata/legacy_pane, C1-C7) and compute group K paths independently of
// the loader under test.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const legacyPaneFixtureDir = "../../pkg/config/testdata/legacy_pane"

// panermGroupK lists the group K paths; "*" matches exactly one mapping key.
var panermGroupK = []string{
	"orchestra.providers.*.pane_args",
	"orchestra.providers.*.interactive_input",
	"orchestra.providers.*.working_patterns",
	"orchestra.subprocess.enabled",
	"features.cc21.monitor_pattern_timeout_ms",
}

// panermP2 is the pruned path list of C2 in byte order (acceptance.md).
var panermP2 = []string{
	"features.cc21.monitor_pattern_timeout_ms",
	"orchestra.providers.claude.pane_args",
	"orchestra.providers.claude.working_patterns",
	"orchestra.providers.codex.interactive_input",
	"orchestra.providers.my-local.interactive_input",
	"orchestra.providers.my-local.pane_args",
	"orchestra.providers.my-local.working_patterns",
	"orchestra.subprocess.enabled",
}

func readLegacyPaneConfig(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(legacyPaneFixtureDir, name))
	require.NoError(t, err)
	return data
}

func parseYAMLRoot(t *testing.T, data []byte) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.NotEmpty(t, doc.Content)
	return doc.Content[0]
}

// groupKPaths returns the concrete group K paths a YAML document holds, in
// byte order.
func groupKPaths(t *testing.T, data []byte) []string {
	t.Helper()
	root := parseYAMLRoot(t, data)
	var paths []string
	for _, pattern := range panermGroupK {
		paths = append(paths, matchYAMLPaths(root, strings.Split(pattern, "."), nil)...)
	}
	sort.Strings(paths)
	return paths
}

func matchYAMLPaths(node *yaml.Node, pattern, prefix []string) []string {
	if len(pattern) == 0 {
		return []string{strings.Join(prefix, ".")}
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	var paths []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		if key := node.Content[i].Value; pattern[0] == "*" || pattern[0] == key {
			next := append(append([]string(nil), prefix...), key)
			paths = append(paths, matchYAMLPaths(node.Content[i+1], pattern[1:], next)...)
		}
	}
	return paths
}

// yamlShape lists every scalar of a document as "path=tag:value" in document
// order, skipping the subtrees at the dropped concrete paths. Two documents
// with equal shapes hold the same keys, order, and values; comments and
// formatting are ignored.
func yamlShape(t *testing.T, data []byte, dropped []string) []string {
	t.Helper()
	drop := map[string]bool{}
	for _, path := range dropped {
		drop[path] = true
	}
	var shape []string
	var walk func(node *yaml.Node, path string)
	walk = func(node *yaml.Node, path string) {
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				child := strings.TrimPrefix(path+"."+node.Content[i].Value, ".")
				if !drop[child] {
					walk(node.Content[i+1], child)
				}
			}
			if len(node.Content) == 0 {
				shape = append(shape, path+"={}")
			}
		case yaml.SequenceNode:
			for i, item := range node.Content {
				walk(item, fmt.Sprintf("%s[%d]", path, i))
			}
			if len(node.Content) == 0 {
				shape = append(shape, path+"=[]")
			}
		default:
			shape = append(shape, path+"="+node.ShortTag()+":"+node.Value)
		}
	}
	walk(parseYAMLRoot(t, data), "")
	return shape
}
