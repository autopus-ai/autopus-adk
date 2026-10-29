package memindex

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/learn"
)

func TestWorkspaceFolderPolicyClassifiesProfileCompatiblePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantClass  string
		wantReason string
	}{
		{name: "project docs", path: ".autopus/project/workspace.md", wantClass: WorkspaceFolderClassIndexable},
		{name: "spec docs", path: ".autopus/specs/SPEC-001/spec.md", wantClass: WorkspaceFolderClassIndexable},
		{name: "vault docs", path: ".autopus/vault/team.md", wantClass: WorkspaceFolderClassIndexable},
		{name: "readme", path: "README.md", wantClass: WorkspaceFolderClassIndexable},
		{name: "docs", path: "docs/guide.md", wantClass: WorkspaceFolderClassIndexable},
		{name: "inbox candidates", path: ".autopus/inbox/draft.md", wantClass: WorkspaceFolderClassCandidate},
		{name: "learning projection", path: ".autopus/learnings/pipeline.jsonl", wantClass: WorkspaceFolderClassProjectionOnly},
		{name: "runtime", path: ".autopus/runtime/memindex/autopus-mem.sqlite", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceRuntime},
		{name: "qa raw artifacts", path: ".autopus/qa/runs/run-1/raw.txt", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceRawArtifact},
		{name: "qa journeys", path: ".autopus/qa/journeys/login.yaml"},
		{name: "context signatures", path: ".autopus/context/signatures.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceContextSignature},
		{name: "manifest", path: ".autopus/root-manifest.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceManifest},
		{name: "plugins", path: ".autopus/plugins/index.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceGeneratedSurface},
		{name: "orchestra", path: ".autopus/orchestra/session.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceGeneratedSurface},
		{name: "brainstorms", path: ".autopus/brainstorms/idea.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceGeneratedSurface},
		{name: "design verify", path: ".autopus/design/verify/latest.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceGeneratedSurface},
		{name: "canary", path: ".autopus/canary/latest.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceGeneratedSurface},
		{name: "codex", path: ".codex/rules/autopus.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "claude", path: ".claude/commands/auto.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "gemini", path: ".gemini/commands/auto.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "opencode", path: ".opencode/rules/autopus.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "agents skills", path: ".agents/skills/auto/SKILL.md", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "agents plugins", path: ".agents/plugins/marketplace.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "agents commands", path: ".agents/commands/auto.toml", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "agents hooks", path: ".agents/hooks.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "symphony artifacts", path: ".symphony/artifacts/run.json", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceHarnessSurface},
		{name: "config", path: "config.toml", wantClass: WorkspaceFolderClassExcluded, wantReason: SkipReasonWorkspaceConfig},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyWorkspaceFolderPath(tt.path)

			assert.Equal(t, tt.wantClass, got.Class)
			assert.Equal(t, tt.wantReason, got.ReasonCode)
		})
	}
}

func TestScanWorkspaceFolderPolicyIncludesCandidatesAndSkipReasons(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	writeMemIndexFile(t, filepath.Join(projectDir, "README.md"), "# Workspace\n\nRoot overview.\n")
	writeMemIndexFile(t, filepath.Join(projectDir, "docs", "guide.md"), "# Guide\n\nHuman managed docs.\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "vault", "curated.md"), "# Curated\n\nVault knowledge.\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "inbox", "draft.md"), "# Draft\n\nCandidate knowledge.\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "runtime", "memindex", "raw.md"), "# Runtime\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "qa", "runs", "raw", "artifact.md"), "# Raw\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "context", "signatures.md"), "# Signatures\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "root-manifest.json"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "plugins", "plugin.md"), "# Plugin\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "orchestra", "run.md"), "# Orchestra\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "brainstorms", "idea.md"), "# Brainstorm\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "design", "verify", "latest.json"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "canary", "latest.json"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".codex", "rules", "generated.md"), "# Codex\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".claude", "commands", "auto.md"), "# Claude\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".gemini", "commands", "auto.md"), "# Gemini\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".opencode", "rules", "auto.md"), "# OpenCode\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".agents", "skills", "auto", "SKILL.md"), "# Skill\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".agents", "plugins", "marketplace.json"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".agents", "commands", "auto.toml"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".agents", "hooks.json"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, ".symphony", "artifacts", "run.json"), "{}\n")
	writeMemIndexFile(t, filepath.Join(projectDir, "config.toml"), "generated = true\n")

	records, skips, err := Scan(projectDir)
	require.NoError(t, err)

	assert.Contains(t, sourceRefs(records), "README.md")
	assert.Contains(t, sourceRefs(records), "docs/guide.md")
	assert.Contains(t, sourceRefs(records), ".autopus/vault/curated.md")
	assert.Contains(t, sourceRefs(records), ".autopus/inbox/draft.md")
	candidate := recordByRef(records, ".autopus/inbox/draft.md")
	require.NotNil(t, candidate)
	assert.Equal(t, "candidate_doc", candidate.SourceType)
	assert.Equal(t, true, candidate.SourceMetadata["candidate"])
	assert.Equal(t, true, candidate.SourceMetadata["promotion_required"])
	assert.Equal(t, false, candidate.SourceMetadata["canonical_knowledge_hub"])

	counts := skippedCounts(skips)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceRuntime], 1)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceRawArtifact], 1)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceContextSignature], 1)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceManifest], 1)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceGeneratedSurface], 3)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceHarnessSurface], 6)
	assert.GreaterOrEqual(t, counts[SkipReasonWorkspaceConfig], 1)
}

func TestScanLearningRowsAreProjectionOnly(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	entry := learn.LearningEntry{
		ID:         "L-WFP-001",
		Timestamp:  time.Date(2026, 5, 21, 1, 2, 3, 0, time.UTC),
		Type:       learn.EntryTypeReviewIssue,
		Phase:      "executor",
		SpecID:     "SPEC-WORKSPACE-FOLDER-PROFILE-001",
		Pattern:    "profile-compatible projection-only learning row",
		Resolution: "keep learnings in ADK Decision/Quality Index projection",
		Severity:   learn.SeverityMedium,
	}
	body, err := json.Marshal(entry)
	require.NoError(t, err)
	writeMemIndexFile(t, filepath.Join(projectDir, ".autopus", "learnings", "pipeline.jsonl"), string(body)+"\n")

	records, _, err := Scan(projectDir)
	require.NoError(t, err)

	record := recordByRef(records, "L-WFP-001")
	require.NotNil(t, record)
	assert.Equal(t, "learning", record.SourceType)
	assert.Equal(t, true, record.SourceMetadata["projection_only"])
	assert.Equal(t, "adk_decision_quality_index", record.SourceMetadata["projection_destination"])
	assert.Equal(t, false, record.SourceMetadata["canonical_knowledge_hub"])
}

func TestWorkspaceFolderPolicyHelpersHandleUnknownAndEmptyInputs(t *testing.T) {
	t.Parallel()

	assert.Empty(t, cleanWorkspaceRel("."))
	assert.Empty(t, cleanWorkspaceRel(""))
	assert.Empty(t, classifyWorkspaceFolderPath("src/main.go").Class)
	assert.Nil(t, workspaceMarkdownMetadata(WorkspaceFolderClassification{}))
	assert.Equal(t, "document", workspaceSourceKind("README.md", WorkspaceFolderClassification{}))
	assert.Equal(t, "vault_doc", workspaceSourceKind(".autopus/vault/handbook.md", WorkspaceFolderClassification{Class: WorkspaceFolderClassIndexable}))
	var memErr *Error
	assert.Nil(t, memErr.Unwrap())
	assert.Empty(t, memErr.Error())
	assert.Empty(t, (&Error{}).Error())
	assert.Equal(t, "plain", (&Error{Err: errors.New("plain")}).Error())
	assert.Equal(t, 0, approxTokens(""))
}

func sourceRefs(records []Record) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.SourceRef)
	}
	return out
}

func recordByRef(records []Record, ref string) *Record {
	for i := range records {
		if records[i].SourceRef == ref {
			return &records[i]
		}
	}
	return nil
}
