package harneval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s1Defect seeds exactly one S1 defect into a standard fixture.
type s1Defect struct {
	name   string
	mutate func(f *fixture)
	detail string
}

// s1DefectFixtures are the S1 fixtures in acceptance order; the loader and the
// run seam must both reject each with exactly its detail code.
var s1DefectFixtures = []s1Defect{
	{"unknown field", func(f *fixture) {
		task := surfaceTask("GT-FIX-A")
		task["extra"] = 1
		f.writeJSON(surfacePath("GT-FIX-A"), task)
	}, DetailUnknownField},
	{"trailing data", func(f *fixture) {
		f.write(surfacePath("GT-FIX-A"), mustJSON(f.t, surfaceTask("GT-FIX-A"))+"{}\n")
	}, DetailTrailingData},
	{"zero assertions", func(f *fixture) {
		task := surfaceTask("GT-FIX-A")
		task["assertions"] = []any{}
		f.writeJSON(surfacePath("GT-FIX-A"), task)
	}, DetailNoAssertions},
	{"unknown assertion kind", func(f *fixture) {
		task := surfaceTask("GT-FIX-A")
		task["assertions"] = []any{map[string]any{"kind": "file_matches", "platform": "codex", "path": "AGENTS.md"}}
		f.writeJSON(surfacePath("GT-FIX-A"), task)
	}, DetailUnknownAssertionKind},
	{"assertion without platform", func(f *fixture) {
		task := surfaceTask("GT-FIX-A")
		task["assertions"] = []any{map[string]any{"kind": AssertFileExists, "path": "AGENTS.md"}}
		f.writeJSON(surfacePath("GT-FIX-A"), task)
	}, DetailAssertionFieldInvalid},
	{"duplicate task id", func(f *fixture) {
		f.writeJSON("evals/harness/tasks/surface/zz-copy.json", surfaceTask("GT-FIX-A"))
	}, DetailDuplicateTaskID},
	{"threshold_bp 5", func(f *fixture) {
		manifest := validManifest()
		manifest["live"].(map[string]any)["threshold_bp"] = 5
		f.writeJSON(ManifestPath, manifest)
	}, DetailPolicyOutOfRange},
	{"unclean active path", func(f *fixture) {
		manifest := validManifest()
		manifest["active_paths"] = []any{"evals/harness/tasks/../candidates"}
		f.writeJSON(ManifestPath, manifest)
	}, DetailUncleanPath},
	{"symlinked task file", func(f *fixture) {
		f.write("elsewhere/GT-FIX-D.json", mustJSON(f.t, surfaceTask("GT-FIX-D")))
		require.NoError(f.t, os.Symlink(filepath.Join(f.root, "elsewhere", "GT-FIX-D.json"),
			filepath.Join(f.root, filepath.FromSlash(surfacePath("GT-FIX-D")))))
	}, DetailSymlinkNotAllowed},
	{"agent task without expected_tests", func(f *fixture) {
		task := agentTask("GT-AG-001")
		delete(task, "expected_tests")
		f.writeJSON(agentPath("GT-AG-001"), task)
	}, DetailExpectedTestsMissing},
}

// TestLoadSet_S1Fixtures_RejectWithExactDetail is the S1 oracle at the loader
// seam: each fixture carries one defect and yields exactly its detail code.
func TestLoadSet_S1Fixtures_RejectWithExactDetail(t *testing.T) {
	t.Parallel()
	for _, tc := range s1DefectFixtures {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given a valid set with exactly one seeded defect
			f := newFixture(t)
			f.standard()
			tc.mutate(f)
			// When the set is loaded
			_, err := LoadSet(f.root)
			// Then it is rejected with exactly that detail
			requireInvalid(t, err, tc.detail)
		})
	}
}

func TestLoadSet_ValidFixture_LoadsSortedTasksAndPins(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()

	set, err := LoadSet(f.root)

	require.NoError(t, err)
	ids := make([]string, 0, len(set.Tasks))
	for _, task := range set.Tasks {
		ids = append(ids, task.ID)
	}
	assert.Equal(t, []string{"GT-AG-001", "GT-FIX-A", "GT-FIX-B", "GT-FIX-C"}, ids)
	assert.Equal(t, 3, set.ActiveCount(KindSurface))
	assert.Equal(t, 1, set.ActiveCount(KindAgent))
	assert.Equal(t, "evals/harness/tasks/surface/GT-FIX-A.json", set.Tasks[1].Path)
	assert.Nil(t, set.CodexCatalog, "an empty pin selects no catalog")
}

func TestLoadSet_ReservedAndOutsideActivePaths_AreRejected(t *testing.T) {
	t.Parallel()
	for path, detail := range map[string]string{
		"evals/harness/candidates":          DetailReservedActivePath,
		"evals/harness/candidates/promoted": DetailReservedActivePath,
		"evals/other":                       DetailUncleanPath,
		"evals/harness":                     DetailUncleanPath,
		"/evals/harness/tasks":              DetailUncleanPath,
		"evals/harness/tasks/":              DetailUncleanPath,
		"":                                  DetailUncleanPath,
	} {
		f := newFixture(t)
		f.standard()
		manifest := validManifest()
		manifest["active_paths"] = []any{path}
		f.writeJSON(ManifestPath, manifest)
		_, err := LoadSet(f.root)
		requireInvalid(t, err, detail)
	}
}

// TestLoadSet_QuarantineIsNeverRead proves the runner reads only active paths:
// a malformed candidate under evals/harness/candidates leaves the load intact.
func TestLoadSet_QuarantineIsNeverRead(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.write("evals/harness/candidates/GTC-0001.json", "{not json")
	f.write("evals/harness/tasks/surface/README.md", "not a task")

	set, err := LoadSet(f.root)

	require.NoError(t, err)
	assert.Len(t, set.Tasks, 4)
}

func TestLoadSet_SymlinkedActiveDirectory_IsRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	real := filepath.Join(f.root, "evals", "harness", "tasks", "agent")
	moved := filepath.Join(f.root, "agent-real")
	require.NoError(t, os.Rename(real, moved))
	require.NoError(t, os.Symlink(moved, real))

	_, err := LoadSet(f.root)

	requireInvalid(t, err, DetailSymlinkNotAllowed)
}

func TestLoadSet_CorpusTampered_IsCorpusDigestMismatch(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.write("bench/corpus_a.json", fixtureCorpus+" ")

	_, err := LoadSet(f.root)

	requireInvalid(t, err, DetailCorpusDigestMismatch)
}

func TestLoadSet_MissingManifestOrActiveDirectory_IsReadFailed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := LoadSet(f.root)
	requireInvalid(t, err, DetailReadFailed)

	f.standard()
	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "evals", "harness", "tasks", "agent")))
	_, err = LoadSet(f.root)
	requireInvalid(t, err, DetailReadFailed)
}

func TestLoadSet_CodexCatalogPin_IsReadAndValidated(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	catalog := `{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"max"}]}]}`
	f.write("evals/harness/fixtures/codex-models.json", catalog)
	manifest := validManifest()
	manifest["pins"].(map[string]any)["codex_model_catalog"] = "evals/harness/fixtures/codex-models.json"
	f.writeJSON(ManifestPath, manifest)

	set, err := LoadSet(f.root)
	require.NoError(t, err)
	assert.Equal(t, catalog, string(set.CodexCatalog))

	f.write("evals/harness/fixtures/codex-models.json", `{"models":"nope"}`)
	_, err = LoadSet(f.root)
	requireInvalid(t, err, DetailFieldInvalid)
}
