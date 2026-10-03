package triage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/evidence"
)

// InputFromManifest builds an Input from a QAMESH evidence manifest written
// by `auto qa run`, reading the bounded failure excerpt from its artifacts.
func InputFromManifest(projectDir, manifestPath string) (Input, error) {
	manifest, err := evidence.LoadManifest(manifestPath)
	if err != nil {
		return Input{}, fmt.Errorf("triage: load manifest: %w", err)
	}
	return InputFor(projectDir, manifest, filepath.Dir(manifestPath)), nil
}

// InputFor builds an Input from a manifest already in memory. manifestDir
// anchors relative artifact paths when the manifest was not loaded from disk.
// The journey falls back to the scenario ref and the adapter to the runner
// name, so a manifest without journey refs still fingerprints stably.
func InputFor(projectDir string, manifest evidence.Manifest, manifestDir string) Input {
	journeyID := strings.TrimSpace(manifest.SourceRefs.JourneyID)
	if journeyID == "" {
		journeyID = manifest.ScenarioRef
	}
	adapter := strings.TrimSpace(manifest.SourceRefs.Adapter)
	if adapter == "" {
		adapter = manifest.Runner.Name
	}
	return Input{
		JourneyID:   journeyID,
		Adapter:     adapter,
		Status:      manifest.Status,
		FailureText: evidence.FailureExcerpt(manifest, manifestDir),
		ProjectDir:  projectDir,
	}
}

// ReplayFor locates the failing generated-spec step for the repair prompt's
// replay section (REQ-13), or returns nil when the output cites no line a
// step map knows. It is independent of the verdict: an environment failure
// on a known step still tells the repair agent where the run stopped.
//
// Pass the result to evidence.WriteFeedbackBundleWithReplay. Evidence cannot
// locate the step itself: the step-map decoder lives in pkg/qa/scenario,
// which imports evidence.
func ReplayFor(in Input) *evidence.Replay {
	hit, ok := locateGeneratedStep(in.ProjectDir, splitLines(in.FailureText))
	if !ok {
		return nil
	}
	return &evidence.Replay{
		ScenarioID: hit.ScenarioID,
		SpecPath:   hit.SpecPath,
		Line:       hit.Line,
		Screen:     hit.Step.Screen,
		Index:      hit.Step.Index,
		Kind:       hit.Step.Kind,
		Ac:         hit.Step.Ac,
		Excerpt:    hit.Window,
		ProjectDir: in.ProjectDir,
	}
}
