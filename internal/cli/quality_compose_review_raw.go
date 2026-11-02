package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reviewRawProvider is the raw document's view of one provider, without
// environment expansion: its args values (nil when the key is absent or not a
// list) and its model, model_policy, backend, and binary scalars. Everything
// the tab shows of a provider comes from here (REQ-RVM-26).
type reviewRawProvider struct {
	Args    []string
	Model   string
	Policy  string
	Backend string
	Binary  string
}

// reviewRawDocument is what the review tab reads of autopus.yaml as written,
// without environment expansion (REQ-RVM-26).
type reviewRawDocument struct {
	// Providers is the view of each provider entry that holds its own values,
	// by its key as written.
	Providers map[string]reviewRawProvider
	// Names reports that every provider name and the judge read as written:
	// the file parses, the orchestra.providers and spec.review_gate mappings
	// take nothing from an alias or a merge key, and no provider key or
	// spec.review_gate.providers item holds a placeholder. An aliased list or
	// judge reads as the anchored node it names.
	Names bool
	// Judge is spec.review_gate.judge as written, when HasJudge.
	Judge    string
	HasJudge bool
	// JudgeModel and JudgeEffort are spec.review_gate.judge_model and
	// judge_effort as written, "" when absent (SPEC-JUDGEMODEL-001).
	JudgeModel, JudgeEffort string
}

// reviewRawOf is the view of a config no file holds, such as init's.
func reviewRawOf(cfg *config.HarnessConfig) map[string]reviewRawProvider {
	raw := make(map[string]reviewRawProvider, len(cfg.Orchestra.Providers))
	for name, entry := range cfg.Orchestra.Providers {
		raw[name] = reviewRawProvider{Args: entry.Args, Model: entry.Model, Policy: entry.ModelPolicy, Backend: entry.Backend, Binary: entry.Binary}
	}
	return raw
}

// reviewRawDocumentOf is the document of a config no file holds: its own
// values are what is written.
func reviewRawDocumentOf(cfg *config.HarnessConfig) *reviewRawDocument {
	gate := cfg.Spec.ReviewGate
	return &reviewRawDocument{
		Providers: reviewRawOf(cfg), Names: true, Judge: gate.Judge, HasJudge: true, JudgeModel: gate.JudgeModel, JudgeEffort: gate.JudgeEffort,
	}
}

// readReviewRaw reads the review settings of dir's autopus.yaml as written.
// A file that cannot be read or parsed gives no provider a view and no name,
// never the expanded config's: the tab then shows and changes nothing it
// cannot read as written (REQ-RVM-26).
func readReviewRaw(dir string) *reviewRawDocument {
	raw := &reviewRawDocument{Providers: map[string]reviewRawProvider{}}
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	if err != nil {
		return raw
	}
	document, _, err := parseAutopusConfigDocument(data)
	if err != nil {
		return raw
	}
	root := document.Content[0]
	providers := readReviewRawProviders(root, raw.Providers)
	raw.Names = readReviewRawGate(root, raw) && providers
	return raw
}

// readReviewRawProviders fills views with the provider entries as written and
// reports whether every provider key reads as written: a scalar of its own,
// without an anchor, a tag, or a placeholder. An alias key reads as its
// anchor's name, never as the value the anchor holds, so it is unreadable.
func readReviewRawProviders(root *yaml.Node, views map[string]reviewRawProvider) bool {
	providers, ok := reviewPlainPath(root, "orchestra", "providers")
	switch {
	case !ok:
		return false
	case providers == nil || reviewNull(providers):
		return true
	case providers.Kind != yaml.MappingNode || yamlMappingHasMerge(providers):
		return false
	}
	names := true
	for i := 0; i+1 < len(providers.Content); i += 2 {
		key := providers.Content[i]
		if key.Kind != yaml.ScalarNode || key.Anchor != "" || key.Style&yaml.TaggedStyle != 0 || strings.Contains(key.Value, "${") {
			names = false
			continue
		}
		if view, ok := reviewRawViewOf(providers.Content[i+1]); ok {
			views[key.Value] = view
		}
	}
	return names
}

// readReviewRawGate reads spec.review_gate.judge as written and reports
// whether the review gate's names read so: a mapping of its own whose
// providers list holds only scalars without a placeholder. The list and the
// judge may be aliases; they read as the nodes they name, which the line
// writer still refuses to edit.
func readReviewRawGate(root *yaml.Node, raw *reviewRawDocument) bool {
	gate, ok := reviewPlainPath(root, "spec", "review_gate")
	switch {
	case !ok:
		return false
	case gate == nil || reviewNull(gate):
		return true
	case gate.Kind != yaml.MappingNode || yamlMappingHasMerge(gate):
		return false
	}
	if _, list := yamlMappingPair(gate, "providers"); list != nil && !reviewNull(reviewAliased(list)) {
		list = reviewAliased(list)
		if list.Kind != yaml.SequenceNode || slices.ContainsFunc(list.Content, func(item *yaml.Node) bool {
			return item.Kind != yaml.ScalarNode || strings.Contains(item.Value, "${")
		}) {
			return false
		}
	}
	if _, judge := yamlMappingPair(gate, "judge"); judge != nil {
		if judge = reviewAliased(judge); judge.Kind != yaml.ScalarNode {
			return false
		}
		raw.Judge, raw.HasJudge = judge.Value, true
	}
	readReviewRawJudgeModel(gate, raw)
	return true
}

// reviewAliased is the node an alias names, or node itself.
func reviewAliased(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}
	return node
}

// reviewPlainPath returns the value at path below root when every mapping on
// the way holds its own content: no alias and no merge key. A missing step
// gives (nil, true), a borrowed one (nil, false).
func reviewPlainPath(root *yaml.Node, path ...string) (*yaml.Node, bool) {
	node := root
	for _, step := range path {
		if node.Kind != yaml.MappingNode || yamlMappingHasMerge(node) {
			return nil, false
		}
		_, value := yamlMappingPair(node, step)
		switch {
		case value == nil:
			return nil, true
		case value.Kind == yaml.AliasNode:
			return nil, false
		}
		node = value
	}
	return node, true
}

func reviewNull(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.ShortTag() == "!!null"
}

// reviewRawViewOf is the view of one provider entry, or false when the entry
// takes a value from elsewhere: it is an alias or holds a merge key, or its
// args, model, model_policy, backend, binary, or an args element is an
// alias. Nothing such an entry holds reads as written, and the line writer
// refuses it anyway.
func reviewRawViewOf(entry *yaml.Node) (reviewRawProvider, bool) {
	if entry.Kind != yaml.MappingNode || yamlMappingHasMerge(entry) {
		return reviewRawProvider{}, false
	}
	for _, key := range []string{"args", "model", "model_policy", "backend", "binary"} {
		if _, value := yamlMappingPair(entry, key); value != nil && value.Kind == yaml.AliasNode {
			return reviewRawProvider{}, false
		}
	}
	view := reviewRawProvider{
		Model: reviewRawScalar(entry, "model"), Policy: reviewRawScalar(entry, "model_policy"),
		Backend: reviewRawScalar(entry, "backend"), Binary: reviewRawScalar(entry, "binary"),
	}
	if _, args := yamlMappingPair(entry, "args"); args != nil && args.Kind == yaml.SequenceNode {
		if slices.ContainsFunc(args.Content, func(item *yaml.Node) bool { return item.Kind == yaml.AliasNode }) {
			return reviewRawProvider{}, false
		}
		view.Args = reviewArgsValues(args)
	}
	return view, true
}

func reviewRawScalar(mapping *yaml.Node, key string) string {
	if _, value := yamlMappingPair(mapping, key); value != nil && value.Kind == yaml.ScalarNode {
		return value.Value
	}
	return ""
}
