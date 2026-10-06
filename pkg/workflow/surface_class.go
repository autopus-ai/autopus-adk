package workflow

import (
	"path"
	"strings"
)

// SurfaceCategory classifies a project-relative path (REQ-EG-01).
type SurfaceCategory string

const (
	// SurfaceGenerated marks the namespace `auto update` regenerates. It is the
	// edit-guard namespace.
	SurfaceGenerated SurfaceCategory = "generated"
	// SurfaceRuntimeState marks state, caches, logs, and manifests the Autopus
	// runtime writes.
	SurfaceRuntimeState SurfaceCategory = "runtime_state"
	// SurfaceAgentArtifact marks files a person or an agent owns: authored
	// documents, agent worktrees, merge-managed tool configuration outside the
	// regenerated namespace, and every path the table does not list.
	SurfaceAgentArtifact SurfaceCategory = "agent_artifact"
)

// SurfaceShape says how a member's Path is matched.
type SurfaceShape uint8

const (
	// ShapePrefix is a directory prefix ending in "/".
	ShapePrefix SurfaceShape = iota + 1
	// ShapeExact is one exact project-relative path.
	ShapeExact
	// ShapeRootManifest is the `.autopus/<platform>-manifest.json` family.
	ShapeRootManifest
	// ShapeFragment is a path fragment a consumer matches below any directory.
	ShapeFragment
)

// SurfaceConsumer is a bit set of the consumers that read a member. Each
// consumer keeps its own matching rule; the table only supplies members.
type SurfaceConsumer uint8

const (
	// ConsumerDriftGate is the release-hygiene drift gate of this package.
	ConsumerDriftGate SurfaceConsumer = 1 << iota
	// ConsumerStatusHygiene is the extra member set the status hygiene check in
	// internal/cli adds on top of the drift-gate members.
	ConsumerStatusHygiene
	// ConsumerQualityLoop is the generated-surface safety check of
	// pkg/qualityloop.
	ConsumerQualityLoop
)

// Members that a consumer matches with a member-specific rule, named so the
// consumer reads them from the table instead of restating the string.
const (
	SurfaceAgentsMarketplace = ".agents/plugins/marketplace.json"
	SurfaceContextSignatures = ".autopus/context/signatures.md"
	SurfaceRootConfig        = "config.toml"
	SurfacePluginCache       = "plugins/cache/"
	// SurfaceManifestDir and SurfaceManifestSuffix bound the root manifest
	// family SurfaceRootManifests.
	SurfaceManifestDir    = ".autopus/"
	SurfaceManifestSuffix = "-manifest.json"
	SurfaceRootManifests  = SurfaceManifestDir + "*" + SurfaceManifestSuffix
)

// SurfaceMember is one row of the categorized surface table.
type SurfaceMember struct {
	Path      string
	Shape     SurfaceShape
	Category  SurfaceCategory
	Consumers SurfaceConsumer
}

const (
	cDrift   = ConsumerDriftGate
	cHygiene = ConsumerStatusHygiene
	cQuality = ConsumerQualityLoop
)

// surfaceTable is the one categorized list behind the drift gate, the status
// hygiene check, pkg/qualityloop, and the edit-guard namespace.
//
// Row order is visible to consumers: GeneratedSurfacePrefixes,
// GeneratedSurfaceExactPaths, and the status-hygiene extra prefixes are this
// order filtered by consumer, and S10 pins those orders.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-EDITGUARD-001: single source of the generated, runtime-state, and agent-artifact surface members; four consumers read it.
// @AX:REASON: Adding a generated row outside the seven namespace roots widens what the edit guard can deny, and a category change on an existing row silently moves a path in or out of that namespace.
var surfaceTable = []SurfaceMember{
	{".claude/", ShapePrefix, SurfaceGenerated, cDrift | cQuality},
	{".claude/worktrees/", ShapePrefix, SurfaceAgentArtifact, 0},
	{".codex/", ShapePrefix, SurfaceGenerated, cDrift | cQuality},
	{".gemini/", ShapePrefix, SurfaceGenerated, cDrift | cQuality},
	{".opencode/", ShapePrefix, SurfaceGenerated, cDrift | cQuality},
	{".agents/", ShapePrefix, SurfaceGenerated, cQuality},
	{".agents/plugins/", ShapePrefix, SurfaceGenerated, cDrift},
	{".agents/commands/", ShapePrefix, SurfaceGenerated, cHygiene},
	{".agents/skills/", ShapePrefix, SurfaceGenerated, cHygiene},
	{".omp/", ShapePrefix, SurfaceGenerated, 0},
	{".autopus/brainstorms/", ShapePrefix, SurfaceAgentArtifact, cDrift | cQuality},
	{".autopus/orchestra/", ShapePrefix, SurfaceRuntimeState, cDrift | cQuality},
	{".autopus/plugins/", ShapePrefix, SurfaceGenerated, cDrift | cQuality},
	{".autopus/txns/", ShapePrefix, SurfaceRuntimeState, cDrift},
	{".autopus/backup/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/cache/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/canary/", ShapePrefix, SurfaceRuntimeState, cHygiene | cQuality},
	{".autopus/design/imports/", ShapePrefix, SurfaceAgentArtifact, cHygiene | cQuality},
	{".autopus/design/verify/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/docs/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/qa/cache/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/qa/evidence/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/qa/feedback/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/qa/gui/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/qa/releases/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/qa/runs/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{".autopus/runtime/", ShapePrefix, SurfaceRuntimeState, cHygiene | cQuality},
	{".autopus/telemetry/", ShapePrefix, SurfaceRuntimeState, cHygiene},
	{SurfaceAgentsMarketplace, ShapeExact, SurfaceGenerated, cDrift | cQuality},
	{SurfaceContextSignatures, ShapeExact, SurfaceRuntimeState, cDrift | cQuality},
	{SurfaceRootConfig, ShapeExact, SurfaceAgentArtifact, cDrift | cQuality},
	{".agents/hooks.json", ShapeExact, SurfaceGenerated, cHygiene},
	{".autopus/audit.jsonl", ShapeExact, SurfaceRuntimeState, cHygiene},
	{".autopus/state.json", ShapeExact, SurfaceRuntimeState, cHygiene},
	{".claude.json", ShapeExact, SurfaceRuntimeState, cHygiene},
	{".mcp.json", ShapeExact, SurfaceAgentArtifact, cHygiene},
	{SurfaceRootManifests, ShapeRootManifest, SurfaceRuntimeState, cDrift | cHygiene | cQuality},
	{SurfacePluginCache, ShapeFragment, SurfaceRuntimeState, cQuality},
}

// SurfaceTable returns a copy of the categorized surface table in row order.
func SurfaceTable() []SurfaceMember {
	return append([]SurfaceMember(nil), surfaceTable...)
}

// SurfacePrefixes returns, in table order, the prefix members a consumer reads.
func SurfacePrefixes(consumer SurfaceConsumer) []string {
	return surfacePaths(consumer, ShapePrefix)
}

// SurfaceExactPaths returns, in table order, the exact-path members a consumer
// reads.
func SurfaceExactPaths(consumer SurfaceConsumer) []string {
	return surfacePaths(consumer, ShapeExact)
}

func surfacePaths(consumer SurfaceConsumer, shape SurfaceShape) []string {
	var out []string
	for _, member := range surfaceTable {
		if member.Shape == shape && member.Consumers&consumer != 0 {
			out = append(out, member.Path)
		}
	}
	return out
}

// ClassifySurface returns the category of a slash-separated project-relative
// path: the category of its most specific prefix, exact, or root-manifest
// member, or SurfaceAgentArtifact when none matches. A fragment member applies
// only when nothing else matched, so a fragment never carves a path out of a
// prefix.
func ClassifySurface(rel string) SurfaceCategory {
	clean := strings.TrimPrefix(path.Clean(rel), "./")
	best, bestLen := SurfaceAgentArtifact, -1
	fragment := SurfaceCategory("")
	for _, member := range surfaceTable {
		matched := false
		switch member.Shape {
		case ShapePrefix:
			matched = strings.HasPrefix(clean, member.Path)
		case ShapeExact:
			matched = clean == member.Path
		case ShapeRootManifest:
			matched = isRootAutopusManifest(clean)
		case ShapeFragment:
			if fragment == "" && strings.Contains("/"+clean, "/"+member.Path) {
				fragment = member.Category
			}
		}
		if matched && len(member.Path) > bestLen {
			best, bestLen = member.Category, len(member.Path)
		}
	}
	if bestLen < 0 && fragment != "" {
		return fragment
	}
	return best
}

// InEditGuardNamespace reports whether a slash-separated project-relative path
// lies in the edit-guard namespace, which is the generated category:
// `.claude/`, `.codex/`, `.gemini/`, `.opencode/`, `.agents/`, `.omp/`, and
// `.autopus/plugins/`, minus `.claude/worktrees/`.
func InEditGuardNamespace(rel string) bool {
	return ClassifySurface(rel) == SurfaceGenerated
}
