package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/version"
)

// sourceRepoMarkers are the paths that together identify an ADK source repo, as
// opposed to an end-user install. All three must exist for the source-repo drift
// checks (template regeneration, binary staleness) to run (REQ-005, REQ-008).
var sourceRepoMarkers = []string{"content", "templates", filepath.Join("cmd", "generate-templates")}

// sourceDriftReport is the source-repo drift verdict. IsSourceRepo gates the
// whole section. RegenChecked reports whether template regeneration ran (false
// on a non-source repo or a regeneration error); StaleTemplates lists the
// templates the current content sources would regenerate differently.
// BinaryChecked reports whether the build-commit comparison ran (false when the
// commit is unset or git is unavailable — a graceful skip, F-005).
type sourceDriftReport struct {
	IsSourceRepo   bool
	RegenChecked   bool
	StaleTemplates []string
	BinaryChecked  bool
	BinaryStale    bool
	BuildCommit    string
	HeadPrefix     string
}

// isDriftSourceRepo reports whether dir is an ADK source repo (all markers
// present). End-user installs lack content/ and templates/, so the source-repo
// checks are skipped there.
func isDriftSourceRepo(dir string) bool {
	for _, marker := range sourceRepoMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err != nil {
			return false
		}
	}
	return true
}

// collectSourceDrift runs the source-repo drift checks. On a non-source repo it
// returns a zero report (IsSourceRepo false) so callers emit no checks.
func collectSourceDrift(dir string) sourceDriftReport {
	rep := sourceDriftReport{}
	if !isDriftSourceRepo(dir) {
		return rep
	}
	rep.IsSourceRepo = true

	if stale, ok := detectTemplateRegenDrift(dir); ok {
		rep.RegenChecked = true
		rep.StaleTemplates = stale
	}

	applyBinaryStaleness(dir, &rep)
	return rep
}

// detectTemplateRegenDrift is the doctor's advisory view of
// content.DetectTemplateRegenDrift: ok is false when the comparison could not
// complete, degrading to a silent skip rather than a false signal. The
// committed templates/ tree is never mutated.
func detectTemplateRegenDrift(dir string) ([]string, bool) {
	stale, err := content.DetectTemplateRegenDrift(dir)
	if err != nil {
		return nil, false
	}
	return stale, true
}

// diffRegeneratedTemplates keeps the doctor's name for
// content.DiffRegeneratedTemplates and drops its error: an incomplete
// comparison reports nothing stale.
func diffRegeneratedTemplates(committedDir, regenDir string) []string {
	stale, err := content.DiffRegeneratedTemplates(committedDir, regenDir)
	if err != nil {
		return nil
	}
	return stale
}

// applyBinaryStaleness compares the 7-char build commit against the repo HEAD
// full hash by prefix (F-001). It is skipped without warning when the build
// commit is unset ("none"/empty, F-003) or when git is unavailable / the dir is
// not a git repo (F-005).
func applyBinaryStaleness(dir string, rep *sourceDriftReport) {
	buildCommit := version.Commit()
	if buildCommit == "" || buildCommit == "none" {
		return
	}

	lines, err := hygieneGitLines(dir, "rev-parse", "HEAD")
	if err != nil || len(lines) == 0 {
		// git unavailable or not a git repo — graceful skip, no check emitted.
		return
	}
	headFull := strings.TrimSpace(lines[0])
	if headFull == "" {
		return
	}

	rep.BinaryChecked = true
	rep.BuildCommit = buildCommit
	rep.HeadPrefix = headPrefixForDisplay(headFull, len(buildCommit))
	rep.BinaryStale = commitIsStale(buildCommit, headFull)
}

// commitIsStale reports whether the truncated build commit is NOT a prefix of
// the full HEAD hash (INV-005). Prefix comparison — not equality — is what makes
// the 7-char build commit compare correctly against the 40-char HEAD without a
// length-mismatch false positive (F-001).
func commitIsStale(buildCommit, headFull string) bool {
	return !strings.HasPrefix(headFull, buildCommit)
}

// headPrefixForDisplay truncates the full HEAD hash to the build-commit width so
// the report shows comparable prefixes rather than a 7-vs-40 length mismatch.
func headPrefixForDisplay(headFull string, width int) string {
	if width <= 0 || width > len(headFull) {
		return headFull
	}
	return headFull[:width]
}
