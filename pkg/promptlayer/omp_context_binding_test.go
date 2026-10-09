package promptlayer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ompContextSpecDir = ".autopus/specs/SPEC-OMP-004"

func TestBuildOMPContextBinding_FullRefsEphemeralHistoryAndShadow_AreDeterministic(t *testing.T) {
	t.Parallel()

	root, opts, delivery := buildOMPContextFixture(t)
	input := validOMPContextBindingInput(opts, delivery)
	input.ProviderLabel = "anthropic"
	first, err := BuildOMPContextBinding(input)
	require.NoError(t, err)

	input.ProviderLabel = "openai"
	second, err := BuildOMPContextBinding(input)
	require.NoError(t, err)

	assert.Equal(t, first.BindingHash, second.BindingHash, "provider label must not affect the binding")
	assert.Equal(t, OMPContextReceiptSchemaVersion, first.SchemaVersion)
	assert.Equal(t, "checkpoint", first.Event)
	assert.Equal(t, delivery.SnapshotHash, first.SnapshotHash)
	assert.Equal(t, delivery.PromptManifestHash, first.PromptManifestHash)
	assert.Len(t, first.FullDocumentRefs, len(delivery.RequiredDocuments))
	assert.Len(t, first.RequiredEphemeralRefs, 6)
	require.Len(t, first.EligibleHistoryRefs, 1)
	assert.Equal(t, "old-read", first.EligibleHistoryRefs[0].ID)
	assert.Equal(t, "completed-superseded", first.EligibleHistoryRefs[0].Reason)
	require.Len(t, first.ShadowPlanRefs, 2)

	wantRefs := make([]string, 0, len(delivery.RequiredDocuments))
	for _, document := range delivery.RequiredDocuments {
		wantRefs = append(wantRefs, document.SourceRef)
		assert.True(t, document.Complete)
	}
	sort.Strings(wantRefs)
	gotRefs := make([]string, 0, len(first.FullDocumentRefs))
	for _, ref := range first.FullDocumentRefs {
		gotRefs = append(gotRefs, ref.SourceRef)
		assert.True(t, ref.Complete)
	}
	assert.Equal(t, wantRefs, gotRefs)

	documentIDs := map[string]bool{}
	for _, ref := range first.FullDocumentRefs {
		documentIDs[ref.SourceRef] = true
	}
	for _, ref := range first.RequiredEphemeralRefs {
		assert.False(t, documentIDs[ref.ID], "ephemeral IDs must be disjoint from document refs")
		assertCanonicalOMPHash(t, ref.Hash)
	}

	raw, err := json.Marshal(first)
	require.NoError(t, err)
	assertOMPBindingReceiptAllowlist(t, raw)
	assert.NotContains(t, string(raw), root)
	assert.NotContains(t, string(raw), input.Ephemeral.OriginalTask)
	assert.NotContains(t, string(raw), input.History[0].Body)
}

func TestBuildOMPContextBinding_HistoryCreditOnlyIncludesCompletedSupersededRows(t *testing.T) {
	t.Parallel()

	_, opts, delivery := buildOMPContextFixture(t)
	input := validOMPContextBindingInput(opts, delivery)
	input.History = []OMPContextHistoryRow{
		{ID: "eligible", SourceRef: "tool/read-old", Body: "old body", Completed: true, Superseded: true},
		{ID: "document", SourceRef: "docs/a.md", Body: "doc body", Completed: true, Superseded: true, Document: true},
		{ID: "unresolved", SourceRef: "tool/error", Body: "error", Completed: true, Superseded: true, Unresolved: true},
		{ID: "current", SourceRef: "tool/read-new", Body: "new", Completed: true},
		{ID: "incomplete", SourceRef: "tool/running", Body: "running", Superseded: true},
	}

	binding, err := BuildOMPContextBinding(input)
	require.NoError(t, err)
	require.Len(t, binding.EligibleHistoryRefs, 1)
	assert.Equal(t, "eligible", binding.EligibleHistoryRefs[0].ID)
	assert.Equal(t, EstimateTokens("old body"), binding.EligibleHistoryRefs[0].TokenEstimate)
}

func TestBuildOMPContextBinding_InvalidDeliveryOrShadowPlan_FailsClosed(t *testing.T) {
	t.Parallel()

	_, opts, delivery := buildOMPContextFixture(t)
	tests := []struct {
		name   string
		mutate func(*OMPContextBindingInput)
	}{
		{name: "incomplete document", mutate: func(input *OMPContextBindingInput) {
			input.Delivery.RequiredDocuments[0].Complete = false
		}},
		{name: "non shadow plan", mutate: func(input *OMPContextBindingInput) {
			input.ShadowPlan.ShadowOnly = false
		}},
		{name: "active jit plan", mutate: func(input *OMPContextBindingInput) {
			input.ShadowPlan.ActiveMode = "jit"
		}},
		{name: "shadow hash mismatch", mutate: func(input *OMPContextBindingInput) {
			input.ShadowPlan.SelectedReferences[0].SourceHash = canonicalHash([]byte("wrong"))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validOMPContextBindingInput(opts, delivery)
			tt.mutate(&input)
			_, err := BuildOMPContextBinding(input)
			require.Error(t, err)
		})
	}
}

func TestBuildOMPContextBinding_OptionalShadowAndHistoryValidation_IsFailClosed(t *testing.T) {
	t.Parallel()

	_, opts, delivery := buildOMPContextFixture(t)
	withoutOptional := validOMPContextBindingInput(opts, delivery)
	withoutOptional.ShadowPlan = nil
	withoutOptional.History = nil
	binding, err := BuildOMPContextBinding(withoutOptional)
	require.NoError(t, err)
	assert.Empty(t, binding.ShadowPlanRefs)
	assert.Empty(t, binding.EligibleHistoryRefs)

	tests := []struct {
		name   string
		mutate func(*OMPContextBindingInput)
	}{
		{name: "empty original task", mutate: func(input *OMPContextBindingInput) {
			input.Ephemeral.OriginalTask = " "
		}},
		{name: "duplicate eligible history", mutate: func(input *OMPContextBindingInput) {
			input.History = append(input.History, input.History[0])
		}},
		{name: "empty eligible history body", mutate: func(input *OMPContextBindingInput) {
			input.History[0].Body = ""
		}},
		{name: "duplicate shadow ref", mutate: func(input *OMPContextBindingInput) {
			input.ShadowPlan.SelectedReferences = append(input.ShadowPlan.SelectedReferences, input.ShadowPlan.PinnedReferences[0])
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validOMPContextBindingInput(opts, delivery)
			tt.mutate(&input)
			_, err := BuildOMPContextBinding(input)
			require.Error(t, err)
		})
	}
}

func buildOMPContextFixture(t *testing.T) (string, ContextDeliveryOptions, ContextDeliveryResult) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"AGENTS.md":                          "project rules",
		".autopus/project/workspace.md":      "workspace policy",
		ompContextSpecDir + "/spec.md":       "# SPEC-OMP-004: Context\n\n---\nid: SPEC-OMP-004\n---\n\nrequirements",
		ompContextSpecDir + "/plan.md":       "implementation plan",
		ompContextSpecDir + "/acceptance.md": "acceptance oracle",
		"docs/task-contract.md":              "task contract",
	}
	for ref, body := range files {
		path := filepath.Join(root, filepath.FromSlash(ref))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	opts := ContextDeliveryOptions{
		Root: root, Command: "go", SpecDir: ompContextSpecDir,
		RequiredReferences: []string{"docs/task-contract.md"},
	}
	delivery, err := BuildContextDelivery(opts)
	require.NoError(t, err)
	require.NoError(t, VerifyContextDeliveryForOptions(opts, delivery))
	return root, opts, delivery
}

func validOMPContextBindingInput(opts ContextDeliveryOptions, delivery ContextDeliveryResult) OMPContextBindingInput {
	byRef := map[string]ContextDeliveryDocument{}
	for _, document := range delivery.RequiredDocuments {
		byRef[document.SourceRef] = document
	}
	spec := byRef[ompContextSpecDir+"/spec.md"]
	task := byRef["docs/task-contract.md"]
	return OMPContextBindingInput{
		WorkspaceID: "autopus-adk", SpecID: "SPEC-OMP-004", TaskID: "TASK-7",
		Phase: "go", SessionID: "session-1", DeliveryOptions: opts, Delivery: delivery,
		Ephemeral: OMPContextEphemeral{
			OriginalTask: "implement context binding", DecisionDelta: "reuse v1 authority",
			FrozenFindingIDs: []string{"F-002", "F-010"},
			OwnershipPaths:   []string{"pkg/promptlayer"}, ForbiddenPaths: []string{"pkg/adapter"},
		},
		History: []OMPContextHistoryRow{
			// Split at the provider prefix so no scanner matches the source.
			{ID: "old-read", SourceRef: "tool/read-old", Body: "superseded tool body sk-" + "test-secret-123456", Completed: true, Superseded: true},
			{ID: "active-error", SourceRef: "tool/error", Body: "unresolved", Completed: true, Superseded: true, Unresolved: true},
		},
		ShadowPlan: &OMPContextShadowPlan{
			SchemaVersion: "autopus.context_plan.v2", ShadowOnly: true, ActiveMode: "full", CandidateMode: "jit",
			PinnedReferences:   []OMPContextPlanReference{{SourceRef: spec.SourceRef, SourceHash: spec.SourceHash}},
			SelectedReferences: []OMPContextPlanReference{{SourceRef: task.SourceRef, SourceHash: task.SourceHash}},
		},
	}
}

func assertCanonicalOMPHash(t *testing.T, value string) {
	t.Helper()
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, value)
}

func assertOMPBindingReceiptAllowlist(t *testing.T, raw []byte) {
	t.Helper()
	var value map[string]any
	require.NoError(t, json.Unmarshal(raw, &value))
	want := []string{
		"binding_hash", "eligible_history_refs", "event", "full_document_refs", "options_hash",
		"phase", "prompt_manifest_hash", "required_ephemeral_refs", "schema_version", "session_id",
		"shadow_plan_refs", "snapshot_hash", "spec_id", "task_id", "workspace_id",
	}
	got := make([]string, 0, len(value))
	for key := range value {
		got = append(got, key)
	}
	sort.Strings(got)
	assert.Equal(t, want, got)
	keys := map[string]bool{}
	collectOMPContextJSONKeys(value, keys)
	for _, forbidden := range []string{"provider", "root", "prompt", "body", "content", "secret"} {
		assert.False(t, keys[forbidden], "receipt must not serialize raw %s fields", forbidden)
	}
}

func sortedOMPContextMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func collectOMPContextJSONKeys(value any, keys map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			keys[key] = true
			collectOMPContextJSONKeys(child, keys)
		}
	case []any:
		for _, child := range typed {
			collectOMPContextJSONKeys(child, keys)
		}
	}
}
