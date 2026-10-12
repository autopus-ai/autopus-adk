package omp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/insajin/autopus-adk/pkg/config"
)

var ompModelCapabilityAllowlist = map[string]struct{}{
	"coding_tool_use":         {},
	"deep_reasoning":          {},
	"deterministic_transform": {},
	"fast_validation":         {},
	"independent_dissent":     {},
	"vision_design":           {},
}

type rawOMPModelCatalog struct {
	Models *[]rawOMPModelMetadata `json:"models"`
}

type rawOMPModelMetadata struct {
	Provider     string    `json:"provider"`
	ID           string    `json:"id"`
	Selector     string    `json:"selector"`
	Family       *string   `json:"family"`
	Capabilities *[]string `json:"capabilities"`
	Thinking     *[]string `json:"thinking"`
	AuthEnabled  *bool     `json:"auth_enabled"`
	Available    *bool     `json:"available"`
	Keyless      bool      `json:"keyless"`
	Disabled     bool      `json:"disabled"`
}

// NormalizeOMPModelCatalog drops unrecognized fields before fingerprinting.
func NormalizeOMPModelCatalog(data []byte, maxOutput int) (OMPModelCatalog, string) {
	if maxOutput <= 0 || len(data) > maxOutput {
		return OMPModelCatalog{}, "catalog_oversized"
	}
	if rejectDuplicateOMPModelReceiptJSON(data) != nil {
		return OMPModelCatalog{}, "catalog_invalid"
	}
	var raw rawOMPModelCatalog
	if !decodeOMPModelCatalogJSON(data, &raw) {
		return OMPModelCatalog{}, "catalog_invalid"
	}
	if raw.Models == nil {
		return OMPModelCatalog{}, "catalog_invalid"
	}
	if len(*raw.Models) == 0 {
		return OMPModelCatalog{}, "catalog_empty"
	}

	models := make([]OMPModelMetadata, 0, len(*raw.Models))
	seen := make(map[string]struct{}, len(*raw.Models))
	for _, entry := range *raw.Models {
		model, reason := normalizeOMPModelMetadata(entry)
		if reason != "" {
			return OMPModelCatalog{}, reason
		}
		selector := model.Provider + "/" + model.Model
		if _, duplicate := seen[selector]; duplicate {
			return OMPModelCatalog{}, "catalog_invalid"
		}
		seen[selector] = struct{}{}
		models = append(models, model)
	}
	return canonicalOMPModelCatalog(models)
}

func canonicalOMPModelCatalog(models []OMPModelMetadata) (OMPModelCatalog, string) {
	models = append([]OMPModelMetadata(nil), models...)
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].Model < models[j].Model
	})
	payload, err := json.Marshal(struct {
		Models []OMPModelMetadata `json:"models"`
	}{Models: models})
	if err != nil {
		return OMPModelCatalog{}, "catalog_invalid"
	}
	sum := sha256.Sum256(payload)
	return OMPModelCatalog{Models: models, Fingerprint: "sha256:" + hex.EncodeToString(sum[:])}, "catalog_ready"
}

func decodeOMPModelCatalogJSON(data []byte, target any) bool {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

// @AX:WARN [AUTO]: model metadata normalization has cyclomatic complexity 17.
// @AX:REASON [AUTO]: gocyclo reports 17 across token, capability, thinking-level, context-window, and duplicate-value checks.
func normalizeOMPModelMetadata(raw rawOMPModelMetadata) (OMPModelMetadata, string) {
	if !safeOMPModelToken(raw.Provider) || !safeOMPModelToken(raw.ID) {
		return OMPModelMetadata{}, "catalog_invalid"
	}
	selector := raw.Provider + "/" + raw.ID
	if raw.Selector != "" && raw.Selector != selector {
		return OMPModelMetadata{}, "catalog_invalid"
	}
	semantics, reason := normalizePresentOMPModelSemantics(raw)
	if reason != "" {
		return OMPModelMetadata{}, reason
	}
	if raw.Family == nil || raw.Capabilities == nil || raw.Thinking == nil ||
		(!raw.Keyless && raw.AuthEnabled == nil && raw.Available == nil) {
		return OMPModelMetadata{}, "catalog_metadata_insufficient"
	}
	authEnabled := raw.AuthEnabled != nil && *raw.AuthEnabled
	if raw.AuthEnabled == nil && raw.Available != nil {
		authEnabled = *raw.Available
	}
	return OMPModelMetadata{
		Provider: raw.Provider, Model: raw.ID, Family: semantics.family,
		Capabilities: semantics.capabilities, Thinking: semantics.thinking,
		AuthEnabled: authEnabled, Keyless: raw.Keyless, Disabled: raw.Disabled,
	}, ""
}

type normalizedOMPModelSemantics struct {
	family       string
	capabilities []string
	thinking     []string
}

func normalizePresentOMPModelSemantics(raw rawOMPModelMetadata) (normalizedOMPModelSemantics, string) {
	var result normalizedOMPModelSemantics
	if raw.Family != nil {
		if !safeOMPModelToken(*raw.Family) {
			return normalizedOMPModelSemantics{}, "catalog_invalid"
		}
		result.family = *raw.Family
	}
	if raw.Capabilities != nil {
		if len(*raw.Capabilities) == 0 {
			return normalizedOMPModelSemantics{}, "catalog_invalid"
		}
		values, ok := normalizeOMPAllowlistedValues(*raw.Capabilities, ompModelCapabilityAllowlist)
		if !ok {
			return normalizedOMPModelSemantics{}, "catalog_invalid"
		}
		result.capabilities = values
	}
	if raw.Thinking != nil {
		if len(*raw.Thinking) == 0 {
			return normalizedOMPModelSemantics{}, "catalog_invalid"
		}
		values, ok := normalizeOMPNativeThinking(*raw.Thinking)
		if !ok {
			return normalizedOMPModelSemantics{}, "catalog_invalid"
		}
		result.thinking = values
	}
	if raw.AuthEnabled != nil && raw.Available != nil && *raw.AuthEnabled != *raw.Available {
		return normalizedOMPModelSemantics{}, "catalog_invalid"
	}
	return result, ""
}

func normalizeOMPAllowlistedValues(values []string, allowed map[string]struct{}) ([]string, bool) {
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	for index, value := range normalized {
		if _, ok := allowed[value]; !ok {
			return nil, false
		}
		if index > 0 && normalized[index-1] == value {
			return nil, false
		}
	}
	return normalized, true
}

func normalizeOMPNativeThinking(values []string) ([]string, bool) {
	normalized := append([]string(nil), values...)
	sort.Strings(normalized)
	for index, value := range normalized {
		if !config.IsOMPNativeThinkingLevel(value) {
			return nil, false
		}
		if index > 0 && normalized[index-1] == value {
			return nil, false
		}
	}
	return normalized, true
}

// safeOMPModelToken accepts a provider or model token of an installed
// catalog. The token reaches selectors, the generated OMP config, and the
// terminal, so a C0 or C1 control, a format character such as a bidi
// override or a zero-width space, and a space separator are all refused.
func safeOMPModelToken(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.Contains(value, "/") {
		return false
	}
	for _, char := range value {
		if char < 0x21 || unicode.IsControl(char) || unicode.In(char, unicode.Cf) || unicode.IsSpace(char) {
			return false
		}
	}
	return true
}
