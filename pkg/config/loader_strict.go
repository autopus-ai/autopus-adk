package config

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// retiredOrchestraConfigKeys lists the autopus.yaml keys retired with the
// orchestra pane backend (SPEC-PANERM-001 group K). A "*" segment matches
// every key of the mapping at that depth, so the per-provider keys are
// tolerated under any provider name. Unlike the other removed keys, a load
// reports the concrete paths it pruned from this set, so the CLI can point the
// user at "auto update", and doctor and the raw-node writers prune through
// PruneRetiredKeys from the same list.
var retiredOrchestraConfigKeys = []string{
	"orchestra.providers.*.pane_args",
	"orchestra.providers.*.interactive_input",
	"orchestra.providers.*.working_patterns",
	"orchestra.subprocess.enabled",
	"features.cc21.monitor_pattern_timeout_ms",
}

// removedConfigKeys lists dotted autopus.yaml paths that the schema deliberately
// dropped. These are accepted and ignored so an existing workspace keeps
// loading after a cutover; Save then rewrites the file without them.
//
// Together with reservedConfigKeys this is the only opt-out from strict
// decoding, and every entry is named explicitly. An unrecognised key is not an
// opt-out: it is a typo that must be reported, because a silently ignored key
// looks like a working setting while having no effect, and the next Save
// deletes it from disk. A "*" segment is legal only here and stands for
// exactly one mapping key.
var removedConfigKeys = append([]string{
	// Removed by the workflow team-mode cutover; see
	// pkg/config/workflow_cutover_test.go for the retained ignore contract.
	"workflow.team_default",
}, retiredOrchestraConfigKeys...)

// reservedConfigKeys lists top-level keys the schema reserves for keys a newer
// binary may introduce. They are tolerated by name so an older binary keeps
// loading a newer file, which is a different thing from tolerating a typo. The
// raw-node save path in internal/cli preserves them verbatim; see
// TestQualitySupervisorCmdPreservesRawConfigAndEnvPlaceholder.
var reservedConfigKeys = []string{
	"future_extension",
	// Operator-managed block carried through platform profile apply; see
	// TestPlatformOMPProfileApplyPersistsAndRollsBackAtomically.
	"operator_extension",
}

// retiredKeyReporter holds the receiver installed by SetRetiredKeyReporter.
var retiredKeyReporter atomic.Pointer[func(paths []string)]

// SetRetiredKeyReporter installs report as the receiver of the retired
// orchestra keys that later loads ignore, and returns the receiver it
// replaced. Every successful Load, LoadPreview, or LoadPreviewWithMetadata that
// pruned at least one key of retiredOrchestraConfigKeys calls report once with
// the concrete dotted paths in byte order; a failing load, or one that pruned
// none, calls nothing. The loader never prints: the caller decides whether a
// notice appears and where. A nil report stops the reporting.
func SetRetiredKeyReporter(report func(paths []string)) func(paths []string) {
	var next *func(paths []string)
	if report != nil {
		next = &report
	}
	if previous := retiredKeyReporter.Swap(next); previous != nil {
		return *previous
	}
	return nil
}

func reportRetiredKeys(paths []string) {
	if len(paths) == 0 {
		return
	}
	if report := retiredKeyReporter.Load(); report != nil {
		(*report)(slices.Clone(paths))
	}
}

// PruneRetiredKeys deletes every retired orchestra key from a raw autopus.yaml
// document in place and returns the concrete dotted paths it deleted, in byte
// order. It touches no other node, so comments, styles, env placeholders, and
// reserved blocks survive a re-encode of doc.
func PruneRetiredKeys(doc *yaml.Node) []string {
	var pruned []string
	for _, key := range retiredOrchestraConfigKeys {
		pruned = append(pruned, pruneNodePaths(doc, strings.Split(key, "."))...)
	}
	return sortedUniquePaths(pruned)
}

// decodeStrict decodes autopus.yaml and rejects any key the schema does not
// declare, reusing the KnownFields(true) precedent from
// internal/cli/workflow_context_runtime_managed_rpc_authority.go. Deliberately
// removed keys are pruned from the parsed document first so only genuinely
// unknown keys fail, and the pruned document is decoded as parsed, never
// re-encoded, so every error names a line of the user's file. It returns the
// retired orchestra paths it pruned, in byte order; the other removed and
// reserved keys are dropped silently.
func decodeStrict(data []byte, out any) ([]string, error) {
	var doc yaml.Node
	// yaml.Unmarshal reads only the first document of a multi-document file
	// and ignores the rest without an error, as B's loader did; the node it
	// fills keeps the lines of the user's file for every later error.
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		// An empty file declares no keys at all. Validate reports the missing
		// required fields with a clearer message than a decoder error would.
		return nil, nil
	}
	var retired []string
	for _, key := range removedConfigKeys {
		pruned := pruneNodePaths(&doc, strings.Split(key, "."))
		if slices.Contains(retiredOrchestraConfigKeys, key) {
			retired = append(retired, pruned...)
		}
	}
	for _, key := range reservedConfigKeys {
		pruneNodePaths(&doc, []string{key})
	}
	if err := decodeNodeKnownFields(&doc, out); err != nil {
		return nil, fmt.Errorf("%w (unknown keys are rejected: fix the typo or delete the key)", err)
	}
	return sortedUniquePaths(retired), nil
}

// decodeNodeKnownFields decodes node into out exactly as a Decoder with
// KnownFields(true) decodes the document it parses. yaml.v3 offers
// KnownFields only on a Decoder, and Node.Decode is never strict, while
// encoding node again to feed a Decoder renumbers the lines: the encoder drops
// blank lines, and the pruned entries are gone. So the Decoder parses a
// placeholder and knownFieldsBridge hands it node, whose nodes keep the lines
// and columns of the file they were parsed from.
func decodeNodeKnownFields(node *yaml.Node, out any) error {
	dec := yaml.NewDecoder(strings.NewReader("{}"))
	dec.KnownFields(true)
	return dec.Decode(&knownFieldsBridge{node: node, out: out})
}

// knownFieldsBridge implements the callback form of yaml.v3's Unmarshaler,
// whose decode callback runs on the strict Decoder that called it. The first
// decode overwrites the parsed placeholder with node; the second decodes the
// result into out.
type knownFieldsBridge struct {
	node *yaml.Node
	out  any
}

func (b *knownFieldsBridge) UnmarshalYAML(decode func(any) error) error {
	if err := decode(&nodeSubstitute{node: b.node}); err != nil {
		return err
	}
	return decode(b.out)
}

// nodeSubstitute replaces the node it is decoded from with its own node.
type nodeSubstitute struct{ node *yaml.Node }

func (s *nodeSubstitute) UnmarshalYAML(parsed *yaml.Node) error {
	*parsed = *s.node
	return nil
}

func sortedUniquePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}
