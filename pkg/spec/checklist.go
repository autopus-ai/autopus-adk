package spec

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/content"
)

// checklistEmbedPath is where `auto spec review` reads its checklist. It sat
// under rules/ until the verification harness stopped installing the
// spec-quality rule; checklists/ is embedded but never installed.
const checklistEmbedPath = "checklists/spec-quality.md"

// LoadChecklist returns the runtime review checklist content.
func LoadChecklist(fsys fs.FS) (string, error) {
	if fsys == nil {
		fsys = content.FS
	}
	body, _, err := loadChecklistWithFallback(fsys, defaultChecklistDiskPaths())
	return body, err
}

// InjectChecklistSection appends the quality checklist section to a review prompt.
// The line budget is resolved through ResolveAuxTotalBudget, the single
// budget-resolution authority (SPEC-ADK-REVIEW-INTEGRITY-001 REQ-RINT-FULL-02):
// DocContextMaxLines is only a compression-fallback threshold, so using it
// directly head-trimmed the checklist and silently dropped the trailing Q-*
// items the reviewer is required to answer.
func InjectChecklistSection(sb *strings.Builder, body string, maxLines int) {
	sb.WriteString("## Quality Checklist\n\n")
	sb.WriteString(trimToLines(strings.TrimSpace(body), ResolveAuxTotalBudget(maxLines)))
	sb.WriteString("\n\n")
}

func writeChecklistExamples(sb *strings.Builder) {
	sb.WriteString("Use the structured CHECKLIST format exactly as shown:\n\n")
	sb.WriteString("  CHECKLIST: <항목 ID> | PASS\n")
	sb.WriteString("  CHECKLIST: <항목 ID> | FAIL | <reason>\n")
}

func loadChecklistForPrompt(opts ReviewPromptOptions) (string, string, error) {
	fsys := opts.checklistFS
	if fsys == nil {
		fsys = content.FS
	}

	diskPaths := opts.checklistDiskPaths
	if len(diskPaths) == 0 {
		diskPaths = defaultChecklistDiskPaths()
	}

	return loadChecklistWithFallback(fsys, diskPaths)
}

func loadChecklistWithFallback(fsys fs.FS, diskPaths []string) (string, string, error) {
	data, err := fs.ReadFile(fsys, checklistEmbedPath)
	if err == nil {
		return string(data), checklistEmbedPath, nil
	}

	lastPath := checklistEmbedPath
	lastErr := err
	for _, path := range diskPaths {
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			return string(data), path, nil
		}
		lastPath = path
		lastErr = readErr
	}

	return "", lastPath, lastErr
}

func defaultChecklistDiskPaths() []string {
	return []string{
		filepath.Join("content", "checklists", "spec-quality.md"),
		filepath.Join("autopus-adk", "content", "checklists", "spec-quality.md"),
	}
}
