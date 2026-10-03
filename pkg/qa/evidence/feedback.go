package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FeedbackResult struct {
	Target     string `json:"target"`
	BundlePath string `json:"prompt_bundle_path"`
	PromptPath string `json:"prompt_path"`
}

// feedbackTarget carries the facts that make `--to` more than a title change.
// Adapter and CLIBinary mirror the platform adapters under pkg/adapter (asserted
// against them in feedback_target_test.go); InstructionDoc is the root document
// each adapter owns and injects its managed section into.
type feedbackTarget struct {
	Flag           string
	Display        string
	Adapter        string
	CLIBinary      string
	InstructionDoc string
}

var supportedFeedbackTargets = map[string]feedbackTarget{
	"codex":    {Flag: "codex", Display: "Codex", Adapter: "codex", CLIBinary: "codex", InstructionDoc: "AGENTS.md"},
	"claude":   {Flag: "claude", Display: "Claude Code", Adapter: "claude-code", CLIBinary: "claude", InstructionDoc: "CLAUDE.md"},
	"gemini":   {Flag: "gemini", Display: "Gemini / Antigravity CLI", Adapter: "antigravity-cli", CLIBinary: "agy", InstructionDoc: "GEMINI.md"},
	"opencode": {Flag: "opencode", Display: "OpenCode", Adapter: "opencode", CLIBinary: "opencode", InstructionDoc: "AGENTS.md"},
}

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-QAMESH-001: feedback bundle generation is the cross-agent repair prompt contract.
// @AX:REASON: Codex, Claude, Gemini, and OpenCode repair flows depend on failed-only validation and safe prompt material at this boundary.
func WriteFeedbackBundle(manifest Manifest, target, outputDir string) (FeedbackResult, error) {
	return writeFeedbackBundle(manifest, target, outputDir, nil)
}

// writeFeedbackBundle is the shared writer; replay is nil when no failing
// generated-spec step was located.
func writeFeedbackBundle(manifest Manifest, target, outputDir string, replay *Replay) (FeedbackResult, error) {
	normalizedTarget := strings.ToLower(strings.TrimSpace(target))
	profile, ok := supportedFeedbackTargets[normalizedTarget]
	if !ok {
		return FeedbackResult{}, fmt.Errorf("unsupported feedback target %q", target)
	}
	if manifest.Status != "failed" {
		return FeedbackResult{}, fmt.Errorf("feedback requires failed deterministic evidence")
	}
	if err := manifest.Validate(); err != nil {
		return FeedbackResult{}, err
	}
	if len(manifest.SourceRefs.OwnedPaths) == 0 || len(manifest.SourceRefs.DoNotModifyPaths) == 0 {
		return FeedbackResult{}, fmt.Errorf("feedback requires owned_paths and do_not_modify_paths")
	}
	bundlePath := filepath.Join(outputDir, safePathSegment(manifest.QAResultID)+"-"+normalizedTarget)
	if err := os.MkdirAll(bundlePath, 0o755); err != nil {
		return FeedbackResult{}, err
	}
	// Artifacts are exported first: the prompt quotes the bundled bytes, so the
	// copy has to exist and be proven safe before the prompt names it.
	artifacts, err := exportBundleArtifacts(manifest, bundlePath)
	if err != nil {
		return FeedbackResult{}, err
	}
	promptPath := filepath.Join(bundlePath, "repair-prompt.md")
	prompt := renderPrompt(manifest, profile, artifacts, replay)
	if err := AssertSafeText(prompt, promptPath); err != nil {
		return FeedbackResult{}, err
	}
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		return FeedbackResult{}, err
	}
	if err := writeBundleMetadata(manifest, profile, artifacts, bundlePath, promptPath); err != nil {
		return FeedbackResult{}, err
	}
	return FeedbackResult{Target: normalizedTarget, BundlePath: bundlePath, PromptPath: promptPath}, nil
}

func writeBundleMetadata(manifest Manifest, target feedbackTarget, artifacts []bundleArtifact, bundlePath, promptPath string) error {
	metadataPath := filepath.Join(bundlePath, "bundle.json")
	metadata := map[string]any{
		"schema_version":      bundleSchemaVersion,
		"target":              target.Flag,
		"target_adapter":      target.Adapter,
		"target_cli":          target.CLIBinary,
		"target_instructions": target.InstructionDoc,
		"qa_result_id":        manifest.QAResultID,
		"prompt_path":         filepath.Base(promptPath),
		"acceptance_refs":     manifest.SourceRefs.AcceptanceRefs,
		"evidence_artifacts":  artifactSummaries(artifacts),
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := AssertSafeText(string(body), metadataPath); err != nil {
		return err
	}
	return os.WriteFile(metadataPath, append(body, '\n'), 0o644)
}
