package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/insajin/autopus-adk/pkg/harneval/intake"
	"github.com/insajin/autopus-adk/pkg/learn"
)

// Acceptance fingerprints of Y (L-002) and X (L-999, L-1000).
const (
	pruneFingerprintY = "023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86"
	pruneFingerprintX = "8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd"
)

// pruneOpenCandidate is the open candidate of the L-001 and L-004 group.
var pruneOpenCandidate = "evals/harness/candidates/GTC-" + pruneGroupFingerprint()[:12] + ".json"

func pruneGroupFingerprint() string {
	return intake.Fingerprint(intake.Entry{Type: "gate_fail", Pattern: "codex hook timeout"})
}

// s7PruneProject seeds the S7 store in dir: L-001 (open candidate ref),
// L-002 (promoted Y's representative), L-003 (no link), and L-1000 (promoted X's
// non-representative ref) dated 2020-01-01, and L-004 (open candidate ref) and
// L-005 (no link) dated now. withIntake adds the candidate, the two promoted
// links, and the .gitkeep and README of the intake area; withManifest adds the
// manifest and the two promoted incident tasks. It returns the store path.
func s7PruneProject(t *testing.T, dir string, withIntake, withManifest bool) string {
	t.Helper()
	store, err := learn.NewStore(dir)
	require.NoError(t, err)
	old, now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC()
	for _, entry := range []learn.LearningEntry{
		{ID: "L-001", Timestamp: old, Type: learn.EntryTypeGateFail, Pattern: "codex hook timeout"},
		{ID: "L-002", Timestamp: old, Type: learn.EntryTypeFixPattern, Pattern: "hook missing in codex"},
		{ID: "L-003", Timestamp: old, Type: learn.EntryTypeGateFail, Pattern: "unrelated flake"},
		{ID: "L-004", Timestamp: now, Type: learn.EntryTypeGateFail, Pattern: "codex hook timeout"},
		{ID: "L-005", Timestamp: now, Type: learn.EntryTypeGateFail, Pattern: "recent flake"},
		{ID: "L-1000", Timestamp: old, Type: learn.EntryTypeReviewIssue, Pattern: "router drops detail mapping"},
	} {
		require.NoError(t, store.Append(entry))
	}
	if withIntake {
		fingerprint := pruneGroupFingerprint()
		writeIntakeJSON(t, dir, pruneOpenCandidate, intake.Candidate{
			SchemaVersion: intake.CandidateSchemaV1, ID: "GTC-" + fingerprint[:12], FingerprintVersion: 1,
			Fingerprint: fingerprint, Representative: "L-001", LearningRefs: []string{"L-001", "L-004"},
			Expected: "e", Actual: "a", RedactedFields: []string{},
			Task: harneval.Task{SchemaVersion: harneval.TaskSchemaV1, ID: "GT-INC-" + strings.ToUpper(fingerprint[:8]),
				Kind: harneval.KindSurface, Variants: []harneval.Variant{}, Assertions: []harneval.Assertion{},
				Provenance: harneval.Provenance{Kind: "incident", Ref: "L-001", Fingerprint: fingerprint},
				Status:     harneval.TaskStatus{State: harneval.StateActive}},
		})
		writeIntakeJSON(t, dir, "evals/harness/candidates/promoted/GT-INC-023E9302.json", pruneLink("GT-INC-023E9302", pruneFingerprintY, "L-002"))
		writeIntakeJSON(t, dir, "evals/harness/candidates/promoted/GT-INC-8E80C7A1.json", pruneLink("GT-INC-8E80C7A1", pruneFingerprintX, "L-999", "L-1000"))
		writeProjectFile(t, dir, "evals/harness/candidates/.gitkeep", "")
		writeProjectFile(t, dir, "evals/harness/candidates/README", "candidates waiting for review\n")
	}
	if withManifest {
		writeProjectFile(t, dir, harneval.ManifestPath, `{"active_paths":["evals/harness/tasks/surface","evals/harness/tasks/agent"]}`)
		writeIntakeJSON(t, dir, "evals/harness/tasks/surface/GT-INC-023E9302.json", pruneTask("GT-INC-023E9302", "L-002", pruneFingerprintY))
		writeIntakeJSON(t, dir, "evals/harness/tasks/surface/GT-INC-8E80C7A1.json", pruneTask("GT-INC-8E80C7A1", "L-999", pruneFingerprintX))
	}
	return filepath.Join(dir, ".autopus", "learnings", "pipeline.jsonl")
}

func pruneLink(taskID, fingerprint string, refs ...string) intake.Link {
	return intake.Link{SchemaVersion: intake.LinkSchemaV1, TaskID: taskID, CandidateID: "GTC-" + fingerprint[:12],
		FingerprintVersion: 1, Fingerprint: fingerprint, Representative: refs[0], LearningRefs: refs,
		Expected: "e", Actual: "a", RedactedFields: []string{}}
}

func pruneTask(id, ref, fingerprint string) map[string]any {
	return map[string]any{
		"schema_version": harneval.TaskSchemaV1, "id": id, "kind": "surface", "category": "hooks_settings",
		"intent": "i", "outcome": "o", "variants": []any{},
		"assertions": []any{map[string]any{"kind": "file_exists", "platform": "codex", "path": ".codex/hooks.json"}},
		"provenance": map[string]any{"kind": "incident", "ref": ref, "fingerprint": fingerprint},
		"status":     map[string]any{"state": "active", "reason": ""},
	}
}

func writeIntakeJSON(t *testing.T, dir, rel string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	writeProjectFile(t, dir, rel, string(data)+"\n")
}

func writeProjectFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func remainingIDs(t *testing.T, dir string) []string {
	t.Helper()
	store, err := learn.NewStore(dir)
	require.NoError(t, err)
	entries, err := store.Read()
	require.NoError(t, err)
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func TestLearnPrune_S7_KeepsEveryEntryOfLinkedGroups(t *testing.T) {
	for _, withManifest := range []bool{true, false} {
		dir := setupLearnDir(t)
		chdir(t, dir)
		path := s7PruneProject(t, dir, true, withManifest)

		first, err := runLearn(t, "learn", "prune", "--days", "30")
		require.NoError(t, err)
		afterFirst := fileSHA256(t, path)
		second, err := runLearn(t, "learn", "prune", "--days", "30")
		require.NoError(t, err)

		assert.Equal(t, "Removed 1 entries older than 30 days.\nKept 3 entries linked to golden-task evals.\n", first, "manifest %v", withManifest)
		assert.Equal(t, []string{"L-001", "L-002", "L-004", "L-005", "L-1000"}, remainingIDs(t, dir))
		assert.Equal(t, "Removed 0 entries older than 30 days.\nKept 3 entries linked to golden-task evals.\n", second)
		assert.Equal(t, afterFirst, fileSHA256(t, path), "a second prune keeps the store bytes")
	}
}

func TestLearnPrune_S7_NoIntakeAreaOrManifest_KeepsTheAgeOnlyOutput(t *testing.T) {
	dir := setupLearnDir(t)
	chdir(t, dir)
	s7PruneProject(t, dir, false, false)

	out, err := runLearn(t, "learn", "prune", "--days", "30")

	require.NoError(t, err)
	assert.Equal(t, "Removed 4 entries older than 30 days.\n", out)
	assert.Equal(t, []string{"L-004", "L-005"}, remainingIDs(t, dir))
}

func TestLearnPrune_S7_UnreadableLinks_RefuseWithTheStoreUnchanged(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"broken candidate": func(t *testing.T, dir string) { writeProjectFile(t, dir, pruneOpenCandidate, "{") },
		"broken manifest":  func(t *testing.T, dir string) { writeProjectFile(t, dir, harneval.ManifestPath, "{\"active_paths\":") },
		"broken active incident task": func(t *testing.T, dir string) {
			writeProjectFile(t, dir, "evals/harness/tasks/surface/GT-INC-8E80C7A1.json", `{"provenance":`)
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			dir := setupLearnDir(t)
			chdir(t, dir)
			path := s7PruneProject(t, dir, true, true)
			seed(t, dir)
			assertPruneRefused(t, path)
		})
	}
}

// assertPruneRefused runs prune and requires eval_links_unreadable, no
// stdout, and an unchanged store.
func assertPruneRefused(t *testing.T, path string) {
	t.Helper()
	before := fileSHA256(t, path)

	out, err := runLearn(t, "learn", "prune", "--days", "30")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "eval_links_unreadable: "), err.Error())
	assert.Contains(t, err.Error(), "the learning store is unchanged")
	assert.Empty(t, out)
	assert.Equal(t, before, fileSHA256(t, path))
}
