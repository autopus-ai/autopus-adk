package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// ReviewDocumentDigests returns the SHA-256 hex digest of every *.md file in
// specDir except review.md: the documents a SPEC review reads. spec.md is
// hashed with its status value normalized, because the review's own status
// promotion rewrites it (SPEC-REVIEWREUSE-001 REQ-RR-REUSE-01).
func ReviewDocumentDigests(specDir string) (map[string]string, error) {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil, err
	}
	digests := make(map[string]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || name == "review.md" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			return nil, err
		}
		if name == "spec.md" {
			data = []byte(normalizeSpecStatus(string(data)))
		}
		digests[name] = SHA256Hex(data)
	}
	return digests, nil
}

// SHA256Hex is the lowercase hex SHA-256 of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeSpecStatus(content string) string {
	if normalized, err := rewriteSpecStatus(content, "-"); err == nil {
		return normalized
	}
	return content
}

// ReviewChecklistBody returns the quality checklist a review prompt injects.
func ReviewChecklistBody() (string, error) {
	body, _, err := loadChecklistForPrompt(ReviewPromptOptions{})
	return body, err
}

// ReviewPromptContract returns the discover and verify prompts built for a
// fixed probe SPEC. It changes whenever the builder's instructions, verdict
// rules, checklist, or policy section change, so a recorded result is tied
// to the prompt contract that produced it.
func ReviewPromptContract() string {
	probe := &SpecDocument{ID: "SPEC-PROBE-001", Title: "Probe",
		RawContent: "# SPEC-PROBE-001: Probe\n\n## Requirements\n\n**REQ-001**\nTHE SYSTEM SHALL answer.\n"}
	discover := BuildReviewPrompt(probe, "", ReviewPromptOptions{Mode: ReviewModeDiscover})
	verify := BuildReviewPrompt(probe, "", ReviewPromptOptions{Mode: ReviewModeVerify,
		PriorFindings: []ReviewFinding{{ID: "F-001", Severity: "major", ScopeRef: "REQ-001", Description: "probe"}}})
	return discover + "\x00" + verify
}
