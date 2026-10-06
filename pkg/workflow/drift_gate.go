package workflow

import (
	"path"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/lore"
)

// GeneratedSurfacePrefixes are the path prefixes the release hygiene drift gate
// treats as generated surfaces. Staging any of these without a corresponding
// source-of-truth change is blocked (REQ-012). The .autopus entries are scoped
// so legitimate `.autopus/specs` and `.autopus/project` edits are not blocked.
// The members and their order come from the categorized surface table
// (surface_class.go, REQ-EG-01).
var GeneratedSurfacePrefixes = SurfacePrefixes(ConsumerDriftGate)

// GeneratedSurfaceExactPaths are generated/runtime files that are not cleanly
// expressible as directory prefixes without blocking human-managed project docs.
// The members come from the categorized surface table.
var GeneratedSurfaceExactPaths = SurfaceExactPaths(ConsumerDriftGate)

// DefaultSourceLimit is the hard per-file source line limit.
const DefaultSourceLimit = 300

// DetectGeneratedDrift returns the staged paths that live under a generated
// surface prefix when no source-of-truth change accompanies them. When
// sotChanged is true the generated regeneration is legitimate and nothing is
// blocked.
func DetectGeneratedDrift(stagedPaths []string, sotChanged bool) []string {
	if sotChanged {
		return nil
	}
	var blocked []string
	for _, p := range stagedPaths {
		if hasGeneratedPrefix(p) {
			blocked = append(blocked, p)
		}
	}
	sort.Strings(blocked)
	return blocked
}

func hasGeneratedPrefix(p string) bool {
	// Normalize slash-based git paths (e.g. ".//.claude/x", "a/../.claude/x")
	// so non-canonical inputs cannot bypass the generated-surface block-list.
	clean := strings.TrimPrefix(path.Clean(p), "./")
	for _, exact := range GeneratedSurfaceExactPaths {
		if clean == exact {
			return true
		}
	}
	if isRootAutopusManifest(clean) {
		return true
	}
	for _, prefix := range GeneratedSurfacePrefixes {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func isRootAutopusManifest(clean string) bool {
	if !strings.HasPrefix(clean, SurfaceManifestDir) {
		return false
	}
	if path.Dir(clean) != ".autopus" {
		return false
	}
	return strings.HasSuffix(path.Base(clean), SurfaceManifestSuffix)
}

// CheckStagedSourceSizes returns the paths whose line count exceeds limit.
func CheckStagedSourceSizes(fileLineCounts map[string]int, limit int) []string {
	var over []string
	for path, count := range fileLineCounts {
		if count > limit {
			over = append(over, path)
		}
	}
	sort.Strings(over)
	return over
}

// CheckPendingLore validates the pending commit message against the Lore format
// and returns human-readable violation messages (empty when the message is
// valid). It reuses the canonical lore.Validate.
func CheckPendingLore(msg string, cfg lore.LoreConfig) []string {
	errs := lore.Validate(msg, cfg)
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		out = append(out, e.Message)
	}
	return out
}

// HygieneReport is the aggregated release hygiene verdict. Blocked is true when
// any generated-surface drift, oversized source file, or Lore violation exists.
type HygieneReport struct {
	Blocked      bool     `json:"blocked"`
	BlockedPaths []string `json:"blocked_paths"`
	Reasons      []string `json:"reasons"`
}

// Hygiene aggregates the drift, source-size, and Lore checks into one report.
// staged/sotChanged feed the generated-surface drift gate; sizes/limit feed the
// 300-line source limit; msg/cfg feed the pending-message Lore check. The
// function is pure and injectable — no live git access — so it stays hermetic.
func Hygiene(staged []string, sotChanged bool, sizes map[string]int, limit int, msg string, cfg lore.LoreConfig) HygieneReport {
	var report HygieneReport

	drift := DetectGeneratedDrift(staged, sotChanged)
	for _, p := range drift {
		report.BlockedPaths = append(report.BlockedPaths, p)
		report.Reasons = append(report.Reasons, "generated-surface drift without source-of-truth change: "+p)
	}

	oversize := CheckStagedSourceSizes(sizes, limit)
	for _, p := range oversize {
		report.BlockedPaths = append(report.BlockedPaths, p)
		report.Reasons = append(report.Reasons, "source file exceeds line limit: "+p)
	}

	loreViolations := CheckPendingLore(msg, cfg)
	for _, v := range loreViolations {
		report.Reasons = append(report.Reasons, "lore format violation: "+v)
	}

	report.Blocked = len(report.BlockedPaths) > 0 || len(loreViolations) > 0
	return report
}
