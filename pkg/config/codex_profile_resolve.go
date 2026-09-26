// Catalog resolution for Codex profiles: parsing the runtime model catalog
// and resolving a requested model/effort against what the account actually has.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParseCodexModelCatalog parses and bounds `codex debug models` JSON.
func ParseCodexModelCatalog(data []byte) (CodexModelCatalog, error) {
	var catalog CodexModelCatalog
	if len(data) > MaxCodexModelCatalogBytes {
		return catalog, fmt.Errorf("codex model catalog exceeds %d bytes", MaxCodexModelCatalogBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return catalog, fmt.Errorf("codex model catalog is empty")
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return CodexModelCatalog{}, fmt.Errorf("parse codex model catalog: %w", err)
	}
	if err := validateCodexModelCatalog(catalog); err != nil {
		return CodexModelCatalog{}, err
	}
	return catalog, nil
}

// Supports reports whether a catalog model advertises the requested effort.
func (c CodexModelCatalog) Supports(model, effort string) bool {
	entry, ok := c.findModel(model)
	return ok && entry.supportsEffort(effort)
}

// codexModelFallbackCandidates lists substitutes to try, best first, when the
// requested model is absent. Astra and Sol descend through the GPT-6 and 5.6
// tiers before crossing the generation boundary to legacy, so a Codex catalog
// that predates GPT-6 Sol or Luna keeps the same rung on 5.6.
func codexModelFallbackCandidates(requested string) []string {
	switch requested {
	case CodexAstraModel:
		return []string{CodexSolModel, CodexPreviousSolModel, CodexTerraModel, CodexLegacyModel}
	case CodexSolModel:
		return []string{CodexPreviousSolModel, CodexTerraModel, CodexLegacyModel}
	case CodexLunaModel:
		return []string{CodexPreviousLunaModel, CodexLegacyModel}
	default:
		return []string{CodexLegacyModel}
	}
}

// ResolveCodexProfile resolves a requested profile against a runtime catalog.
func ResolveCodexProfile(requested CodexProfile, catalogJSON []byte) CodexProfileResolution {
	catalog, err := ParseCodexModelCatalog(catalogJSON)
	if err != nil {
		return CodexProfileResolution{
			Requested:    requested,
			Effective:    legacyCodexProfile(requested.Effort),
			Fallback:     true,
			Reason:       CodexResolutionCatalogUnknown,
			CatalogError: err,
		}
	}
	if requested.Model == CodexLegacyModel {
		return resolveRequestedLegacyCodexProfile(catalog, requested)
	}

	model, ok := catalog.findModel(requested.Model)
	if !ok {
		for _, candidate := range codexModelFallbackCandidates(requested.Model) {
			entry, entryOK := catalog.findModel(candidate)
			if !entryOK {
				continue
			}
			wanted := requested.Effort
			if candidate == CodexLegacyModel {
				wanted = capLegacyCodexEffort(wanted)
			}
			effort, effortOK := entry.highestCompatibleEffort(wanted)
			if !effortOK {
				continue
			}
			return CodexProfileResolution{
				Requested: requested,
				Effective: CodexProfile{Model: candidate, Effort: effort},
				Fallback:  true,
				Reason:    CodexResolutionModelUnavailable,
			}
		}
		return CodexProfileResolution{
			Requested: requested,
			Fallback:  true,
			Reason:    CodexResolutionRuntimeDefault,
		}
	}
	if model.supportsEffort(requested.Effort) {
		return CodexProfileResolution{
			Requested: requested,
			Effective: requested,
			Reason:    CodexResolutionSupported,
		}
	}

	effectiveEffort, ok := model.highestCompatibleEffort(requested.Effort)
	if !ok {
		return CodexProfileResolution{
			Requested: requested,
			Effective: CodexProfile{Model: requested.Model},
			Fallback:  true,
			Reason:    CodexResolutionRuntimeDefault,
		}
	}
	return CodexProfileResolution{
		Requested: requested,
		Effective: CodexProfile{Model: requested.Model, Effort: effectiveEffort},
		Fallback:  true,
		Reason:    CodexResolutionEffortUnavailable,
	}
}

func (c CodexModelCatalog) findModel(slug string) (CodexCatalogModel, bool) {
	for _, model := range c.Models {
		if model.Slug == slug {
			return model, true
		}
	}
	return CodexCatalogModel{}, false
}

func (m CodexCatalogModel) supportsEffort(effort string) bool {
	for _, level := range m.SupportedReasoningLevels {
		if level.Effort == effort {
			return true
		}
	}
	return false
}

func (m CodexCatalogModel) highestCompatibleEffort(requested string) (string, bool) {
	requestedRank := codexEffortRank(requested)
	if requestedRank < 0 {
		requestedRank = codexEffortRank(m.DefaultReasoningLevel)
	}
	if requestedRank < 0 {
		requestedRank = len(codexEffortOrder) - 1
	}

	bestEffort := ""
	bestRank := -1
	for _, level := range m.SupportedReasoningLevels {
		rank := codexEffortRank(level.Effort)
		if rank >= 0 && rank <= requestedRank && rank > bestRank {
			bestEffort = level.Effort
			bestRank = rank
		}
	}
	return bestEffort, bestRank >= 0
}

func codexEffortRank(effort string) int {
	for rank, candidate := range codexEffortOrder {
		if candidate == effort {
			return rank
		}
	}
	return -1
}
