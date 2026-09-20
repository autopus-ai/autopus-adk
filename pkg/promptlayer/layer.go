package promptlayer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type Kind string

const (
	KindStable    Kind = "stable"
	KindSnapshot  Kind = "snapshot"
	KindEphemeral Kind = "ephemeral"
)

type Group string

const (
	GroupIdentityRules    Group = "identity_rules"
	GroupMethodologyTools Group = "methodology_tools"
	GroupStableSkillIndex Group = "stable_skill_index"
	GroupProjectContext   Group = "project_context"
	GroupFrozenSnapshot   Group = "frozen_snapshot"
	GroupTaskContext      Group = "task_context"
	GroupUserRequest      Group = "user_request"
)

const (
	RedactionPassed   = "passed"
	RedactionRedacted = "redacted"
	RedactionSkipped  = "skipped"

	InvalidationNone                   = "none"
	InvalidationStableSourceChanged    = "stable_source_changed"
	InvalidationSnapshotRebuild        = "snapshot_rebuild"
	InvalidationEphemeralChanged       = "ephemeral_changed"
	InvalidationMissingOptionalContext = "missing_optional_context"
	InvalidationInjectionRisk          = "injection_risk"
	InvalidationSecretRisk             = "secret_risk"
	InvalidationSizeCap                = "size_cap"
)

type Layer struct {
	ID                 string
	Kind               Kind
	Group              Group
	SourceRef          string
	Content            string
	CacheEligible      bool
	RedactionStatus    string
	InvalidationReason string
}

type Manifest struct {
	SnapshotID string          `json:"snapshot_id,omitempty"`
	Entries    []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	ID                 string `json:"id"`
	Kind               Kind   `json:"kind"`
	Group              Group  `json:"group"`
	SourceRef          string `json:"source_ref"`
	Hash               string `json:"hash"`
	TokenEstimate      int    `json:"token_estimate"`
	CacheEligible      bool   `json:"cache_eligible"`
	RedactionStatus    string `json:"redaction_status"`
	InvalidationReason string `json:"invalidation_reason"`
}

type RenderResult struct {
	Prompt   string
	Manifest Manifest
}

type ManifestChange struct {
	ID           string
	Kind         Kind
	PreviousHash string
	CurrentHash  string
	Reason       string
}

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-AGENT-PROMPT-001: Render is the canonical prompt layer ordering and manifest materialization boundary.
// @AX:REASON: Pipeline and orchestra builders depend on stable group priority, hash, token estimate, and snapshot semantics for cache diagnostics.
func Render(layers []Layer) (RenderResult, error) {
	ordered := append([]Layer(nil), layers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		pi, pj := groupPriority(ordered[i].Group, ordered[i].Kind), groupPriority(ordered[j].Group, ordered[j].Kind)
		if pi != pj {
			return pi < pj
		}
		return ordered[i].ID < ordered[j].ID
	})

	var promptParts []string
	manifest := Manifest{Entries: make([]ManifestEntry, 0, len(ordered))}
	seenIDs := make(map[string]struct{}, len(ordered))
	for _, layer := range ordered {
		if layer.ID == "" {
			return RenderResult{}, fmt.Errorf("prompt layer id is required")
		}
		if _, exists := seenIDs[layer.ID]; exists {
			return RenderResult{}, fmt.Errorf("duplicate prompt layer id: %q", layer.ID)
		}
		seenIDs[layer.ID] = struct{}{}
		status := layer.RedactionStatus
		if status == "" {
			status = RedactionPassed
		}
		reason := layer.InvalidationReason
		if reason == "" {
			reason = InvalidationNone
		}
		hash := hashContent(layer.Content)
		manifest.Entries = append(manifest.Entries, ManifestEntry{
			ID:                 layer.ID,
			Kind:               layer.Kind,
			Group:              layer.Group,
			SourceRef:          layer.SourceRef,
			Hash:               hash,
			TokenEstimate:      EstimateTokens(layer.Content),
			CacheEligible:      layer.CacheEligible,
			RedactionStatus:    status,
			InvalidationReason: reason,
		})
		if layer.Kind == KindSnapshot && manifest.SnapshotID == "" {
			manifest.SnapshotID = layer.ID
		}
		if strings.TrimSpace(layer.Content) != "" {
			promptParts = append(promptParts, layer.Content)
		}
	}

	return RenderResult{Prompt: strings.Join(promptParts, "\n\n"), Manifest: manifest}, nil
}

func SnapshotLayer(id, sourceRef, content string) Layer {
	return Layer{
		ID:              id,
		Kind:            KindSnapshot,
		Group:           GroupFrozenSnapshot,
		SourceRef:       sourceRef,
		Content:         content,
		RedactionStatus: RedactionPassed,
	}
}

func CompareManifests(previous, current Manifest) []ManifestChange {
	prev := entriesByID(previous)
	seen := map[string]bool{}
	var changes []ManifestChange
	for _, next := range current.Entries {
		seen[next.ID] = true
		old, ok := prev[next.ID]
		if !ok {
			changes = append(changes, ManifestChange{ID: next.ID, Kind: next.Kind, CurrentHash: next.Hash, Reason: reasonForKind(next.Kind)})
			continue
		}
		if manifestEntryChanged(old, next) {
			changes = append(changes, ManifestChange{
				ID:           next.ID,
				Kind:         next.Kind,
				PreviousHash: old.Hash,
				CurrentHash:  next.Hash,
				Reason:       reasonForKind(next.Kind),
			})
		}
	}
	for _, old := range previous.Entries {
		if seen[old.ID] {
			continue
		}
		changes = append(changes, ManifestChange{
			ID:           old.ID,
			Kind:         old.Kind,
			PreviousHash: old.Hash,
			Reason:       reasonForKind(old.Kind),
		})
	}
	return changes
}

func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

func entriesByID(manifest Manifest) map[string]ManifestEntry {
	out := make(map[string]ManifestEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		out[entry.ID] = entry
	}
	return out
}

func manifestEntryChanged(old, next ManifestEntry) bool {
	return old.Hash != next.Hash ||
		old.Kind != next.Kind ||
		old.Group != next.Group ||
		old.SourceRef != next.SourceRef ||
		old.CacheEligible != next.CacheEligible ||
		old.RedactionStatus != next.RedactionStatus ||
		old.InvalidationReason != next.InvalidationReason
}

func reasonForKind(kind Kind) string {
	switch kind {
	case KindStable:
		return InvalidationStableSourceChanged
	case KindSnapshot:
		return InvalidationSnapshotRebuild
	default:
		return InvalidationEphemeralChanged
	}
}

// @AX:NOTE [AUTO] @AX:SPEC: SPEC-AGENT-PROMPT-001: numeric priorities encode deterministic prompt layer precedence; lower values render earlier.
func groupPriority(group Group, kind Kind) int {
	switch group {
	case GroupIdentityRules:
		return 10
	case GroupMethodologyTools:
		return 20
	case GroupStableSkillIndex:
		return 30
	case GroupProjectContext:
		return 40
	case GroupFrozenSnapshot:
		return 50
	case GroupTaskContext:
		return 60
	case GroupUserRequest:
		return 70
	}
	switch kind {
	case KindStable:
		return 40
	case KindSnapshot:
		return 50
	default:
		return 60
	}
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
