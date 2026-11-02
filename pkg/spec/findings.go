package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// findingsSidecar is the object shape of review-findings.json introduced by
// SPEC-ADK-REVIEW-INTEGRITY-001. Prior artifacts are a top-level array; loaders
// sniff the first non-space byte to accept both shapes (REQ-RINT-COMPAT-10).
// @AX:ANCHOR: [AUTO] on-disk backward-compat contract — every review-findings.json written before this SPEC is a bare array; changing the '{' sniff in LoadFindingsWithCoverage or this struct's JSON tags breaks parsing of all pre-existing sidecars
// @AX:REASON: REQ-RINT-COMPAT-10 requires prior-schema files to keep loading without error; this is the sole shape-detection point
type findingsSidecar struct {
	Findings     []ReviewFinding `json:"findings"`
	DocCoverages []DocCoverage   `json:"doc_coverages"`
	// Baseline is what the last usable round reviewed; PersistReviewBaseline
	// writes it after the findings (SPEC-REVIEWREUSE-001 REQ-RR-DELTA-02).
	Baseline *ReviewBaseline `json:"baseline,omitempty"`
}

// PersistFindings writes the current findings state to review-findings.json.
// A nil findings slice is normalized to an empty slice so the sidecar always
// holds a valid JSON array `[]` instead of the literal `null` (issue #58):
// `json.MarshalIndent` emits `null` for a nil slice, which makes the file look
// corrupt to downstream tooling even when review.md still carries the findings.
func PersistFindings(dir string, findings []ReviewFinding) error {
	if findings == nil {
		findings = []ReviewFinding{}
	}
	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal findings: %w", err)
	}
	path := filepath.Join(dir, "review-findings.json")
	return os.WriteFile(path, data, 0o644)
}

// PersistFindingsWithCoverage writes findings and per-document observation
// coverage as the object-shaped review-findings.json (SPEC-ADK-REVIEW-INTEGRITY-001
// REQ-RINT-COV-01). Nil slices normalize to empty arrays so the sidecar never
// serializes the literal null (issue #58). LoadFindings reads this shape and the
// legacy array shape interchangeably.
func PersistFindingsWithCoverage(dir string, findings []ReviewFinding, coverages []DocCoverage) error {
	if findings == nil {
		findings = []ReviewFinding{}
	}
	if coverages == nil {
		coverages = []DocCoverage{}
	}
	data, err := json.MarshalIndent(findingsSidecar{Findings: findings, DocCoverages: coverages}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal findings sidecar: %w", err)
	}
	path := filepath.Join(dir, "review-findings.json")
	return os.WriteFile(path, data, 0o644)
}

// NormalizeAdvisoryFindings keeps suggestion-level feedback visible without
// letting it block review-gate convergence.
func NormalizeAdvisoryFindings(findings []ReviewFinding) []ReviewFinding {
	if len(findings) == 0 {
		return nil
	}

	updated := make([]ReviewFinding, len(findings))
	for i, f := range findings {
		updated[i] = f
		if IsAdvisoryFinding(f) && isOpenOrRegressed(f.Status) {
			updated[i].Status = FindingStatusDeferred
		}
	}
	return updated
}

// IsAdvisoryFinding reports whether a finding is non-blocking review feedback.
// Blocking is a function of category/impact first and the severity label second
// (issue #187): a correctness, contract, data-loss or security finding blocks at
// any severity, an advisory-category finding blocks only when a provider
// escalated it to critical/major or the escape hatch flagged it, and any other
// category falls back to the severity label.
func IsAdvisoryFinding(f ReviewFinding) bool {
	if f.EscapeHatch {
		return false
	}
	category := normalizedFindingCategory(f.Category)
	if _, blocking := impactCategories[category]; blocking {
		return false
	}
	if _, advisory := advisoryCategories[category]; advisory {
		return !isEscalatedSeverity(f.Severity)
	}
	return isSuggestionSeverity(f.Severity)
}

// IsActiveBlockingFinding reports whether a finding should block PASS.
func IsActiveBlockingFinding(f ReviewFinding) bool {
	if !isOpenOrRegressed(f.Status) {
		return false
	}
	return !IsAdvisoryFinding(f)
}

func isOpenOrRegressed(status FindingStatus) bool {
	return status == FindingStatusOpen || status == FindingStatusRegressed
}

// LoadFindings reads prior findings from review-findings.json.
// Returns empty slice (not error) if file doesn't exist.
// Returns error if file exists but is corrupted.
// Accepts both the legacy top-level array and the object shape written by
// PersistFindingsWithCoverage (REQ-RINT-COMPAT-10).
func LoadFindings(dir string) ([]ReviewFinding, error) {
	findings, _, err := LoadFindingsWithCoverage(dir)
	return findings, err
}

// LoadFindingsWithCoverage reads findings and any recorded observation coverage
// from review-findings.json. Legacy array-shaped sidecars load with an empty
// coverage set, treating the coverage fields as additive and optional
// (REQ-RINT-COMPAT-10). A missing file is not an error.
func LoadFindingsWithCoverage(dir string) ([]ReviewFinding, []DocCoverage, error) {
	path := filepath.Join(dir, "review-findings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ReviewFinding{}, nil, nil
		}
		return nil, nil, fmt.Errorf("read findings file: %w", err)
	}

	// Sniff the first non-space byte: '{' is the object shape, otherwise the
	// legacy top-level array.
	if trimmed := bytes.TrimLeft(data, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '{' {
		var sidecar findingsSidecar
		if err := json.Unmarshal(data, &sidecar); err != nil {
			return nil, nil, fmt.Errorf("unmarshal findings sidecar: %w", err)
		}
		return sidecar.Findings, sidecar.DocCoverages, nil
	}

	var findings []ReviewFinding
	if err := json.Unmarshal(data, &findings); err != nil {
		return nil, nil, fmt.Errorf("unmarshal findings: %w", err)
	}
	return findings, nil, nil
}

func normalizedDescriptionPrefix(description string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(description), " "))
	runes := []rune(normalized)
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return normalized
}

// NormalizeScopeRef normalizes a scope reference for comparison (REQ-012):
// 1. Strip leading "./"
// 2. Strip basePath prefix for absolute paths (if provided)
// 3. Lowercase for file paths
// 4. Requirement refs (REQ-xxx) are kept as-is
// 5. Line number info is preserved
func NormalizeScopeRef(ref, basePath string) string {
	if ref == "" {
		return ref
	}

	ref = strings.TrimPrefix(ref, "./")

	// Requirement references are exact match — no normalization.
	if strings.HasPrefix(strings.ToUpper(ref), "REQ-") {
		return strings.ToUpper(ref)
	}

	// Strip absolute basePath prefix to get relative path.
	if basePath != "" {
		prefix := strings.TrimSuffix(basePath, "/") + "/"
		ref = strings.TrimPrefix(ref, prefix)
	}

	return strings.ToLower(ref)
}
