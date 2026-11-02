package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// ReviewBaselineSchema versions the baseline record a delta verify round
// checks its input against (SPEC-REVIEWREUSE-001 REQ-RR-DELTA-02).
const ReviewBaselineSchema = "spec_review_baseline.v1"

// Delivered scopes of a baseline document: whether the baseline round's full
// input carried the whole document or a compaction of it (REQ-RR-DELTA-02).
const (
	BaselineDeliveredComplete = "complete"
	BaselineDeliveredPartial  = "partial"
)

// ReviewBaseline records what the last usable review round delivered: every
// SPEC document its full input carries, in the delivered representation and
// with its digests, whether the round delivered each one complete, the
// digests of every other required source and setting, and the findings it
// left open.
type ReviewBaseline struct {
	Schema     string                            `json:"schema"`
	RunID      string                            `json:"run_id"`
	Revision   int                               `json:"revision"`
	RecordedAt string                            `json:"recorded_at"`
	Documents  map[string]ReviewBaselineDocument `json:"documents"`
	Delivered  map[string]string                 `json:"delivered"`
	Context    map[string]string                 `json:"context"`
	OpenIDs    []string                          `json:"open_ids"`
}

// ReviewBaselineDocument is one SPEC document of the baseline round: the
// digest of its source file, its delivered representation, and that
// representation's digest. Content is never the raw source, so a secret the
// complete input withholds is not copied into review-findings.json.
type ReviewBaselineDocument struct {
	SourceSHA256 string `json:"source_sha256"`
	PromptSHA256 string `json:"prompt_sha256"`
	Content      string `json:"content"`
}

// ReviewInputDocument is a SPEC document as a review request delivers it.
type ReviewInputDocument struct {
	SourceSHA256 string
	PromptSHA256 string
	Content      string
}

// NewReviewInputDocument applies the sanitization of the complete review
// input (loadCompleteReviewDocuments): secrets redacted, injection markers
// neutralized in place, whitespace kept. The source digest proves what the
// file held and the prompt digest what a request carries, so the two are
// never confused (REQ-RR-DELTA-02, REQ-RR-DELTA-04).
func NewReviewInputDocument(raw []byte) ReviewInputDocument {
	sanitized := promptlayer.SanitizeContent(string(raw), promptlayer.ContextOptions{Required: true, PreserveInjectionEvidence: true})
	return ReviewInputDocument{SourceSHA256: SHA256Hex(raw), PromptSHA256: SHA256Hex([]byte(sanitized.Content)), Content: sanitized.Content}
}

// CarriedReviewDocuments names the SPEC documents a full review input
// carries when they exist; any other file in the SPEC directory is not part
// of what a reviewer reads.
func CarriedReviewDocuments() []string {
	return slices.Clone(requiredReviewDocumentNames)
}

// ReviewInputDocuments reads the carried SPEC documents present in specDir
// in their delivered representation.
func ReviewInputDocuments(specDir string) (map[string]ReviewInputDocument, error) {
	documents := map[string]ReviewInputDocument{}
	for _, name := range requiredReviewDocumentNames {
		data, err := os.ReadFile(filepath.Join(specDir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		documents[name] = NewReviewInputDocument(data)
	}
	return documents, nil
}

// BaselineDocuments records each delivered document with its digests.
func BaselineDocuments(documents map[string]ReviewInputDocument) map[string]ReviewBaselineDocument {
	recorded := make(map[string]ReviewBaselineDocument, len(documents))
	for name, document := range documents {
		recorded[name] = ReviewBaselineDocument(document)
	}
	return recorded
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
