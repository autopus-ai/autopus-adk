package evidence

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Replay is the failing generated-spec step of a GUI journey, located from
// the runner output and the compiler's step map (SPEC-QALOOP-001 REQ-13).
//
// Evidence receives the located step instead of finding it. The step-map
// decoder lives in pkg/qa/scenario, which already depends on this package
// (scenario -> journey -> evidence), so loading step maps here would be an
// import cycle, and a second, looser decoder here could send a repair agent to
// the wrong step. pkg/qa/triage sits above both and hands the result in.
type Replay struct {
	ScenarioID string `json:"scenario_id,omitempty"`
	SpecPath   string `json:"spec_path"`
	Line       int    `json:"line"`
	Screen     string `json:"screen"`
	Index      int    `json:"index"`
	Kind       string `json:"kind"`
	Ac         string `json:"ac,omitempty"`
	// Excerpt is the bounded runner output around the failing line.
	Excerpt string `json:"excerpt,omitempty"`
	// ProjectDir lets capture paths print project-relative. It is never
	// printed itself.
	ProjectDir string `json:"-"`
}

const (
	// maxFailureReadBytes bounds one runner-output read: only the tail is ever
	// quoted, so a runaway log must not be read whole.
	maxFailureReadBytes = 64 << 10
	maxReplayCaptures   = 12
	runDirLabel         = "<run>"
)

// runnerOutputKinds are the artifact kinds that hold what the runner printed.
var runnerOutputKinds = map[string]bool{"stdout": true, "stderr": true, "command_output": true}

// captureRoles orders the capture evidence a replay cites.
var captureRoles = []string{"trace", "screenshot", "console", "network"}

// WriteFeedbackBundleWithReplay writes the same bundle as WriteFeedbackBundle
// and adds a `## Replay` section when replay names a located step. A nil
// replay produces exactly the WriteFeedbackBundle output.
func WriteFeedbackBundleWithReplay(manifest Manifest, target, outputDir string, replay *Replay) (FeedbackResult, error) {
	return writeFeedbackBundle(manifest, target, outputDir, replay)
}

// FailureExcerpt returns the bounded, redacted tail of the runner output a
// manifest recorded, which is the text a repair prompt quotes. Runner output
// kinds outrank other artifacts because a console or network log can carry an
// unrelated error, such as a refused analytics beacon, that would misdirect
// triage; the failed checks' artifacts are the fallback, as in the prompt.
// Local-only artifacts are never read, so nothing returned here carries
// evidence the bundle withholds. manifestDir anchors relative artifact paths
// when the manifest was not loaded from disk.
func FailureExcerpt(manifest Manifest, manifestDir string) string {
	if manifest.sourceDir == "" && manifestDir != "" {
		manifest.sourceDir = manifestDir
	}
	parts := make([]string, 0, maxExcerptArtifacts)
	for _, artifact := range failureOutputArtifacts(manifest) {
		source, ok := resolveArtifactSource(manifest, artifact.Path)
		if !ok {
			continue
		}
		body, err := readTail(source, maxFailureReadBytes)
		if err != nil {
			continue
		}
		if tail := excerptTail(RedactText(body)); tail != "" {
			parts = append(parts, tail)
		}
	}
	return strings.Join(parts, "\n")
}

func failureOutputArtifacts(manifest Manifest) []ArtifactRef {
	runnerOutput := publishableArtifacts(manifest, func(artifact ArtifactRef) bool {
		return runnerOutputKinds[strings.ToLower(strings.TrimSpace(artifact.Kind))]
	})
	if len(runnerOutput) > 0 {
		return runnerOutput
	}
	wanted := map[string]bool{}
	for _, check := range failedChecks(manifest.OracleResults.Checks) {
		for _, ref := range check.ArtifactRefs {
			wanted[ref] = true
		}
	}
	return publishableArtifacts(manifest, func(artifact ArtifactRef) bool {
		return wanted[artifact.Kind] && isTextArtifactPath(artifact.Path)
	})
}

func publishableArtifacts(manifest Manifest, keep func(ArtifactRef) bool) []ArtifactRef {
	out := make([]ArtifactRef, 0, maxExcerptArtifacts)
	for _, artifact := range manifest.Artifacts {
		if !artifact.Publishable || !keep(artifact) {
			continue
		}
		out = append(out, artifact)
		if len(out) == maxExcerptArtifacts {
			break
		}
	}
	return out
}

func readTail(path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file")
	}
	if info.Size() > limit {
		if _, err := file.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return "", err
		}
	}
	body, err := io.ReadAll(io.LimitReader(file, limit))
	return string(body), err
}

type replayCapture struct {
	Role     string
	Kind     string
	Path     string
	InRunDir bool
}

// writeReplay cites the failing step and where its capture evidence lives.
// Raw captures stay local: the section names paths, never their bytes.
func writeReplay(b *strings.Builder, manifest Manifest, replay *Replay) {
	if replay == nil || strings.TrimSpace(replay.Kind) == "" {
		return
	}
	fmt.Fprintf(b, "\n## Replay\n\n")
	fmt.Fprintf(b, "- Failing step: screen `%s`, step %d, kind `%s`\n", promptInline(replay.Screen), replay.Index, promptInline(replay.Kind))
	if replay.Ac != "" {
		fmt.Fprintf(b, "- Acceptance criterion (`ac`): `%s`\n", promptInline(replay.Ac))
	}
	if replay.ScenarioID != "" {
		fmt.Fprintf(b, "- Scenario: `%s`\n", promptInline(replay.ScenarioID))
	}
	if replay.SpecPath != "" {
		fmt.Fprintf(b, "- Generated spec line: `%s:%d`\n", promptInline(replay.SpecPath), replay.Line)
	}
	writeReplayCaptures(b, replayCaptures(manifest, replay.ProjectDir))
	if excerpt := excerptTail(replay.Excerpt); excerpt != "" {
		fmt.Fprintf(b, "\nFailure excerpt at the failing step:\n\n```text\n%s\n```\n", promptBlock(excerpt))
	}
}

func writeReplayCaptures(b *strings.Builder, captures []replayCapture) {
	if len(captures) == 0 {
		fmt.Fprintf(b, "- Captures: none recorded on disk for this run\n")
		return
	}
	fmt.Fprintf(b, "- Captures (local paths only; open them, their bytes are not inlined):\n")
	inRunDir := false
	for _, capture := range captures {
		fmt.Fprintf(b, "  - %s (`%s`): `%s`\n", capture.Role, promptInline(capture.Kind), promptInline(capture.Path))
		inRunDir = inRunDir || capture.InRunDir
	}
	if inRunDir {
		fmt.Fprintf(b, "  - `%s` is the run directory that holds the evidence manifest\n", runDirLabel)
	}
}

func replayCaptures(manifest Manifest, projectDir string) []replayCapture {
	out := make([]replayCapture, 0, maxReplayCaptures)
	for _, role := range captureRoles {
		for _, artifact := range manifest.Artifacts {
			if captureRole(artifact) != role {
				continue
			}
			resolved, ok := locateCapture(manifest, projectDir, artifact.Path)
			if !ok {
				continue
			}
			path, inRunDir := capturePromptPath(manifest, projectDir, resolved, artifact.Path)
			out = append(out, replayCapture{Role: role, Kind: artifact.Kind, Path: path, InRunDir: inRunDir})
			if len(out) == maxReplayCaptures {
				return out
			}
		}
	}
	return out
}

// captureRole names the replay evidence an artifact holds, or "" when it is
// not capture evidence. The extension decides for binary media; a text
// artifact needs its kind to say what it captured.
func captureRole(artifact ArtifactRef) string {
	kind := strings.ToLower(artifact.Kind)
	switch strings.ToLower(filepath.Ext(artifact.Path)) {
	case ".zip":
		return "trace"
	case ".png", ".jpg", ".jpeg", ".webp":
		return "screenshot"
	case ".har":
		return "network"
	case ".json", ".jsonl", ".log":
		for _, role := range captureRoles {
			if strings.Contains(kind, role) {
				return role
			}
		}
	}
	return ""
}

// locateCapture resolves a capture that exists inside the run directory or
// the project. Only the path is printed, but containment still matters: an
// untrusted manifest must not make the prompt confirm arbitrary paths exist.
func locateCapture(manifest Manifest, projectDir, recorded string) (string, bool) {
	candidates := make([]string, 0, 2)
	if source, ok := resolveArtifactSource(manifest, recorded); ok {
		candidates = append(candidates, source)
	}
	if base, err := realAbsPath(projectDir); projectDir != "" && err == nil {
		candidate := recorded
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(base, candidate)
		}
		if resolved, err := realAbsPath(candidate); err == nil && isPathWithin(resolved, base) {
			candidates = append(candidates, resolved)
		}
	}
	for _, candidate := range candidates {
		resolved, err := realAbsPath(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(resolved); err == nil && info.Mode().IsRegular() {
			return resolved, true
		}
	}
	return "", false
}

// capturePromptPath prefers a project-relative path, which a repair agent
// working in the project can open as is, then a run-relative one.
func capturePromptPath(manifest Manifest, projectDir, resolved, recorded string) (string, bool) {
	if rel, ok := relativeWithin(projectDir, resolved); ok {
		return rel, false
	}
	if rel, ok := relativeWithin(manifest.sourceDir, resolved); ok {
		return runDirLabel + "/" + rel, true
	}
	return recorded, false
}

func relativeWithin(root, path string) (string, bool) {
	if root == "" {
		return "", false
	}
	base, err := realAbsPath(root)
	if err != nil || !isPathWithin(path, base) {
		return "", false
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
