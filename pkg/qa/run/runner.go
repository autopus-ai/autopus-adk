package run

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	qaevidence "github.com/insajin/autopus-adk/pkg/qa/evidence"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

func Execute(opts Options) (Result, error) {
	opts = normalizeOptions(opts)
	plan, err := BuildPlan(opts)
	if err != nil {
		return Result{}, err
	}
	if opts.FeedbackTo != "" && !validFeedbackTarget(opts.FeedbackTo) {
		return Result{}, fmt.Errorf("unsupported feedback target %q", opts.FeedbackTo)
	}
	if opts.DryRun {
		return dryRunResult(plan), nil
	}
	started := time.Now().UTC()
	runID := "qa-" + uuid.NewString()
	runDir := filepath.Join(opts.Output, runID)
	result := initialResult(opts, plan, runID, runDir)
	if blocked, status := mobileReadinessExecutionStatus(plan); blocked {
		result.Status = status
		result = sanitizeResult(result)
		published, err := publishResult(result, opts, started, time.Now().UTC())
		if err != nil {
			return published, err
		}
		if status == "blocked" {
			return published, fmt.Errorf("qa run blocked")
		}
		return published, nil
	}
	packs, err := executionPacks(opts, plan)
	if err != nil {
		return Result{}, err
	}
	for _, pack := range packs {
		adapterResult, manifestPath, checks := executePack(opts, pack, filepath.Join(runDir, "_raw"), runDir)
		result.SourceRefs = appendUniqueStrings(result.SourceRefs, packSourceRefs(opts.ProjectDir, pack)...)
		result.AdapterResults = append(result.AdapterResults, adapterResult)
		if adapterResult.SetupGap != nil {
			result.SetupGaps = append(result.SetupGaps, *adapterResult.SetupGap)
		}
		if manifestPath != "" {
			result.ManifestPaths = append(result.ManifestPaths, manifestPath)
		}
		for _, check := range checks {
			result.Checks = append(result.Checks, check)
			if check.Status == "failed" || check.Status == "blocked" {
				result.FailedChecks = append(result.FailedChecks, check.ID)
			}
		}
	}
	noteEmptyExecution(&result, opts.Lane)
	result.Status = aggregateStatus(result)
	if hasBlockedAdapter(result.AdapterResults) {
		result.Status = "blocked"
	}
	result = sanitizeResult(result)
	if opts.FeedbackTo != "" {
		paths, err := writeFeedbackBundles(opts, result.ManifestPaths)
		if err != nil {
			return result, err
		}
		result.FeedbackBundlePaths = paths
		result.FeedbackAvailable = len(result.FeedbackBundlePaths) > 0
		result = sanitizeResult(result)
	}
	published, err := publishResult(result, opts, started, time.Now().UTC())
	if err != nil {
		return published, err
	}
	if published.Status == "failed" || published.Status == "blocked" {
		return published, fmt.Errorf("qa run %s", published.Status)
	}
	return published, nil
}

func validFeedbackTarget(target string) bool {
	switch target {
	case "codex", "claude", "gemini", "opencode":
		return true
	default:
		return false
	}
}

func dryRunResult(plan Plan) Result {
	status := "passed"
	if len(plan.SetupGaps) > 0 {
		status = "warning"
	}
	return Result{
		Status:              status,
		Lane:                plan.SelectedLane,
		DryRun:              true,
		HarnessContract:     plan.HarnessContract,
		SelectedJourneys:    plan.SelectedJourneys,
		SelectedAdapters:    plan.SelectedAdapters,
		OutputRoot:          plan.OutputRoot,
		RunIndexPreviewPath: plan.RunIndexPreviewPath,
		ManifestPreviews:    plan.ManifestOutputPreviewPaths,
		ArtifactPreviews:    plan.ArtifactPreviewRefs,
		CandidateJourneys:   plan.CandidateJourneys,
		ManifestPaths:       []string{},
		FailedChecks:        []string{},
		Checks:              []IndexCheck{},
		AdapterResults:      []AdapterResult{},
		SetupGaps:           plan.SetupGaps,
		RedactionStatus:     RedactionStatus{Status: "passed"},
	}
}

func initialResult(opts Options, plan Plan, runID, runDir string) Result {
	return Result{
		RunID:             runID,
		Status:            "passed",
		Lane:              opts.Lane,
		HarnessContract:   plan.HarnessContract,
		SelectedJourneys:  plan.SelectedJourneys,
		SelectedAdapters:  plan.SelectedAdapters,
		OutputRoot:        opts.Output,
		RunIndexPath:      filepath.Join(runDir, "run-index.json"),
		ManifestPaths:     []string{},
		FailedChecks:      []string{},
		Checks:            []IndexCheck{},
		AdapterResults:    []AdapterResult{},
		SetupGaps:         plan.SetupGaps,
		RedactionStatus:   RedactionStatus{Status: "passed"},
		FeedbackAvailable: false,
		Workspace:         workspaceRef(opts.ProjectDir),
		SourceRefs:        laneSourceRefs(opts.ProjectDir, opts.Lane),
	}
}

func executionPacks(opts Options, plan Plan) ([]journey.Pack, error) {
	packs, err := selectedPacks(opts)
	if err != nil {
		return nil, err
	}
	if len(packs) > 0 {
		return packs, nil
	}
	if candidates := candidatePacks(plan); len(candidates) > 0 {
		return candidates, nil
	}
	return detectedPacks(plan), nil
}

func selectedPacks(opts Options) ([]journey.Pack, error) {
	packs, err := journey.LoadDir(opts.ProjectDir)
	if err != nil {
		return nil, err
	}
	out := make([]journey.Pack, 0, len(packs))
	for _, pack := range packs {
		if deferredPackReason(pack) != "" {
			continue
		}
		if includePack(pack, opts) {
			out = append(out, pack)
		}
	}
	return out, nil
}

func executePack(opts Options, pack journey.Pack, rawRoot, runDir string) (AdapterResult, string, []IndexCheck) {
	check := IndexCheck{ID: firstCheckID(pack), JourneyID: pack.ID, Adapter: pack.Adapter.ID, Expected: "exit_code=0"}
	if pack.Adapter.ID == "desktop-accessibility-observe" {
		return executeDesktopObservationPack(opts, pack, runDir)
	}
	if gap := setupGapFor(opts, pack); gap != nil {
		check.Status = "skipped"
		return AdapterResult{Adapter: pack.Adapter.ID, JourneyID: pack.ID, Status: "skipped", SetupGap: gap}, "", []IndexCheck{check}
	}
	if strings.EqualFold(opts.Lane, laneMobileScripted) && mobileAdapter(pack.Adapter.ID) {
		return executeMobilePack(opts, pack, rawRoot, runDir)
	}
	cmdResult := runCommand(opts.ProjectDir, pack, filepath.Join(rawRoot, safeSegment(pack.ID)))
	applyOracle(opts.ProjectDir, pack, &cmdResult, &check)
	checks := []IndexCheck{check}
	if policyCheck, ok := applyGUIPolicyOracle(opts.ProjectDir, pack, &cmdResult); ok {
		if policyCheck.Status == "blocked" && check.Status == "failed" {
			check.Status = "blocked"
			check.FailureSummary = policyCheck.FailureSummary
			checks[0] = check
		}
		checks = append(checks, policyCheck)
	}
	if publicationCheck, ok := applyGUIArtifactPublicationOracle(opts.ProjectDir, pack, &cmdResult); ok {
		checks = append(checks, publicationCheck)
	}
	if captureCheck, ok := applyGUICaptureOracle(pack, &cmdResult); ok {
		if captureCheck.Status == "blocked" {
			checks[0].Status = "blocked"
			checks[0].FailureSummary = captureCheck.FailureSummary
		}
		checks = append(checks, captureCheck)
	}
	manifest := buildManifest(opts, pack, cmdResult, checks)
	manifestPath, err := qaevidence.WriteFinalManifest(manifest, manifestOutputDir(runDir, pack.ID))
	if err != nil {
		checks[0].Status = "blocked"
		checks[0].FailureSummary = err.Error()
		return AdapterResult{
			Adapter:               pack.Adapter.ID,
			JourneyID:             pack.ID,
			Status:                "blocked",
			RepairPromptAvailable: false,
			FailureSummary:        err.Error(),
		}, "", checks
	}
	return AdapterResult{
		Adapter:               pack.Adapter.ID,
		JourneyID:             pack.ID,
		Status:                cmdResult.Status,
		QAMESHManifestPath:    manifestPath,
		RepairPromptAvailable: cmdResult.Status == "failed" || cmdResult.Status == "blocked",
		FailureSummary:        cmdResult.FailureSummary,
	}, manifestPath, checks
}

func hasBlockedAdapter(results []AdapterResult) bool {
	for _, result := range results {
		if result.Status == "blocked" {
			return true
		}
	}
	return false
}

func writeFeedbackBundles(opts Options, manifestPaths []string) ([]string, error) {
	paths := []string{}
	for _, path := range manifestPaths {
		manifest, err := qaevidence.LoadManifest(path)
		if err != nil || manifest.Status != "failed" {
			continue
		}
		output := filepath.Join(filepath.Dir(filepath.Dir(path)), "feedback")
		replay := triage.ReplayForManifest(opts.ProjectDir, manifest, filepath.Dir(path))
		result, err := qaevidence.WriteFeedbackBundleWithReplay(manifest, opts.FeedbackTo, output, replay)
		if err != nil {
			return paths, err
		}
		paths = append(paths, result.BundlePath)
	}
	return paths, nil
}
