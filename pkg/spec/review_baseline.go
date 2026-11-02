package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReviewBaselineSchema versions the baseline record a delta verify round
// checks its input against (SPEC-REVIEWREUSE-001 REQ-RR-DELTA-02).
const ReviewBaselineSchema = "spec_review_baseline.v1"

// ReviewBaseline records what the last usable review round read: the content
// and digest of every SPEC document, the digests of every other required
// source and setting, and the findings it left open.
type ReviewBaseline struct {
	Schema     string                            `json:"schema"`
	RunID      string                            `json:"run_id"`
	Revision   int                               `json:"revision"`
	RecordedAt string                            `json:"recorded_at"`
	Documents  map[string]ReviewBaselineDocument `json:"documents"`
	Context    map[string]string                 `json:"context"`
	OpenIDs    []string                          `json:"open_ids"`
}

// ReviewBaselineDocument is one SPEC document as the baseline round read it.
type ReviewBaselineDocument struct {
	SHA256  string `json:"sha256"`
	Content string `json:"content"`
}

// ReviewDocumentContents reads every *.md file in specDir except review.md.
func ReviewDocumentContents(specDir string) (map[string]string, error) {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil, err
	}
	contents := make(map[string]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || name == "review.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			return nil, err
		}
		contents[name] = string(data)
	}
	return contents, nil
}

// BaselineDocuments pairs each document with its digest.
func BaselineDocuments(contents map[string]string) map[string]ReviewBaselineDocument {
	documents := make(map[string]ReviewBaselineDocument, len(contents))
	for name, content := range contents {
		documents[name] = ReviewBaselineDocument{SHA256: SHA256Hex([]byte(content)), Content: content}
	}
	return documents
}

// OpenFindingIDs lists, sorted, the ids of open or regressed findings.
func OpenFindingIDs(findings []ReviewFinding) []string {
	ids := []string{}
	for _, finding := range findings {
		if isOpenOrRegressed(finding.Status) && finding.ID != "" {
			ids = append(ids, finding.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// PersistReviewBaseline stores baseline in review-findings.json beside the
// findings and coverage already written there.
func PersistReviewBaseline(dir string, baseline *ReviewBaseline) error {
	path := filepath.Join(dir, "review-findings.json")
	sidecar, err := readFindingsSidecar(path)
	if err != nil {
		return err
	}
	sidecar.Baseline = baseline
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal findings sidecar: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadReviewBaseline returns the stored baseline, nil when the sidecar or the
// baseline is absent, and an error when the sidecar does not parse.
func LoadReviewBaseline(dir string) (*ReviewBaseline, error) {
	sidecar, err := readFindingsSidecar(filepath.Join(dir, "review-findings.json"))
	if err != nil {
		return nil, err
	}
	return sidecar.Baseline, nil
}

func readFindingsSidecar(path string) (findingsSidecar, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return findingsSidecar{Findings: []ReviewFinding{}, DocCoverages: []DocCoverage{}}, nil
	}
	if err != nil {
		return findingsSidecar{}, fmt.Errorf("read findings file: %w", err)
	}
	var sidecar findingsSidecar
	if trimmed := bytes.TrimLeft(data, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
		err = json.Unmarshal(data, &sidecar.Findings)
	} else {
		err = json.Unmarshal(data, &sidecar)
	}
	if err != nil {
		return findingsSidecar{}, fmt.Errorf("unmarshal findings sidecar: %w", err)
	}
	return sidecar, nil
}
