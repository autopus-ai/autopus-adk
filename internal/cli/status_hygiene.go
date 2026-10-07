package cli

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

type statusHygieneReport struct {
	Available         bool
	Status            string
	GeneratedDrift    []string
	TrackedButIgnored []string
	RuntimeUnignored  []string
	Diagnostic        string
}

type statusHygienePayload struct {
	Available              bool                         `json:"available"`
	Status                 string                       `json:"status"`
	GeneratedDrift         statusHygieneMetricPayload   `json:"generated_drift"`
	TrackedButIgnored      statusHygieneMetricPayload   `json:"tracked_but_ignored"`
	TrackedIgnoredFamilies []statusHygieneFamilyPayload `json:"tracked_but_ignored_families,omitempty"`
	RuntimeUnignored       statusHygieneMetricPayload   `json:"runtime_unignored"`
	Diagnostic             string                       `json:"diagnostic,omitempty"`
}

type statusHygieneMetricPayload struct {
	Status  string   `json:"status"`
	Count   int      `json:"count"`
	Paths   []string `json:"paths,omitempty"`
	Message string   `json:"message,omitempty"`
}

// statusHygieneFamilyPayload carries the per-family split so `--json` consumers
// get the same actionable grouping the text renderer shows.
type statusHygieneFamilyPayload struct {
	Label       string   `json:"label"`
	Disposition string   `json:"disposition"`
	Count       int      `json:"count"`
	Paths       []string `json:"paths"`
}

// runtimeUnignoredExtraPrefixes and runtimeUnignoredExtraExactPaths are the
// members the hygiene check adds on top of the drift-gate members. Both come
// from the categorized surface table in pkg/workflow (REQ-EG-01).
var runtimeUnignoredExtraPrefixes = workflow.SurfacePrefixes(workflow.ConsumerStatusHygiene)

var runtimeUnignoredExtraExactPaths = surfacePathSet(workflow.SurfaceExactPaths(workflow.ConsumerStatusHygiene))

func surfacePathSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, rel := range paths {
		set[rel] = true
	}
	return set
}

func collectStatusHygiene(dir string) statusHygieneReport {
	if _, err := hygieneGitLines(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return unavailableStatusHygiene(fmt.Sprintf("git worktree unavailable: %v", err))
	}

	statusLines, err := hygieneGitLines(dir, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return unavailableStatusHygiene(fmt.Sprintf("git status unavailable: %v", err))
	}
	changed, untracked := parseHygieneGitStatus(statusLines)

	trackedButIgnored, err := hygieneGitLines(dir, "ls-files", "-c", "-i", "--exclude-standard")
	if err != nil {
		return unavailableStatusHygiene(fmt.Sprintf("git tracked-but-ignored diagnostic unavailable: %v", err))
	}

	report := statusHygieneReport{
		Available:         true,
		Status:            "ok",
		GeneratedDrift:    generatedWorkingTreeDriftCandidates(changed),
		TrackedButIgnored: uniqueSortedGitPaths(trackedButIgnored),
		RuntimeUnignored:  runtimeUnignoredCandidates(untracked),
	}
	if report.hasWarning() {
		report.Status = "warn"
	}
	return report
}

func unavailableStatusHygiene(diagnostic string) statusHygieneReport {
	return statusHygieneReport{
		Available:  false,
		Status:     "unavailable",
		Diagnostic: diagnostic,
	}
}

func generatedWorkingTreeDriftCandidates(paths []string) []string {
	candidates := workflow.DetectGeneratedDrift(paths, false)
	for _, rel := range paths {
		clean := normalizeGitRel(rel)
		if clean == "" {
			continue
		}
		if isRuntimeUnignoredRisk(clean) {
			candidates = append(candidates, clean)
		}
	}
	return uniqueSortedGitPaths(candidates)
}

func runtimeUnignoredCandidates(paths []string) []string {
	var candidates []string
	for _, rel := range paths {
		clean := normalizeGitRel(rel)
		if clean == "" {
			continue
		}
		if isRuntimeUnignoredRisk(clean) {
			candidates = append(candidates, clean)
		}
	}
	return uniqueSortedGitPaths(candidates)
}

func isRuntimeUnignoredRisk(rel string) bool {
	if runtimeUnignoredExtraExactPaths[rel] || isRootAutopusManifestPath(rel) {
		return true
	}
	for _, exact := range workflow.GeneratedSurfaceExactPaths {
		if rel == exact {
			return true
		}
	}
	for _, prefix := range workflow.GeneratedSurfacePrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	for _, prefix := range runtimeUnignoredExtraPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func parseHygieneGitStatus(lines []string) ([]string, []string) {
	var changed []string
	var untracked []string
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		code := line[:2]
		rel := strings.TrimSpace(line[3:])
		if idx := strings.LastIndex(rel, " -> "); idx >= 0 {
			rel = rel[idx+4:]
		}
		rel = normalizeGitRel(strings.Trim(rel, `"`))
		if rel == "" {
			continue
		}
		changed = append(changed, rel)
		if code == "??" {
			untracked = append(untracked, rel)
		}
	}
	return uniqueSortedGitPaths(changed), uniqueSortedGitPaths(untracked)
}

func hygieneGitLines(dir string, args ...string) ([]string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	text := strings.TrimRight(stdout.String(), "\r\n")
	if text == "" {
		return nil, nil
	}

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func uniqueSortedGitPaths(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, rel := range paths {
		clean := normalizeGitRel(rel)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

func (r statusHygieneReport) hasWarning() bool {
	if !r.Available {
		return true
	}
	return len(r.GeneratedDrift) > 0 || len(r.TrackedButIgnored) > 0 || len(r.RuntimeUnignored) > 0
}
