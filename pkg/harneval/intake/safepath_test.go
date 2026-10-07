package intake

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidCandidateID_OnlyLowercaseTwelveHex_Accepted(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"GTC-023e9302ff0b":         true,
		"GTC-8e80c7a18029":         true,
		"GTC-023E9302FF0B":         false,
		"GTC-023e9302ff0b/../x":    false,
		"../x":                     false,
		"GTC-023e9302ff0":          false,
		"GTC-023e9302ff0bb":        false,
		"gtc-023e9302ff0b":         false,
		"GTC-023e9302ff0b\n":       false,
		"GTC-023e9302ff0b.json":    false,
		"":                         false,
		"GTC-023e9302ff0b/../../x": false,
	}
	for id, want := range cases {
		assert.Equal(t, want, ValidCandidateID(id), "id %q", id)
	}
}

func TestValidTaskID_AgreesWithHarnevalDecoder(t *testing.T) {
	t.Parallel()
	// The intake copy of the SPEC-HARNEVAL-001 id grammar must accept exactly
	// the ids the 001 strict decoder accepts.
	ids := []string{
		"GT-INC-023E9302", "GT-INC-8E80C7A1", "GT-A12", "GT-AB", "GT-1AB", "GT-inc-023e9302",
		"GT-INC-023E9302/../x", "GT-" + strings.Repeat("A", 41), "GT-" + strings.Repeat("A", 42),
		"GT-INC_1", "XT-INC-1", "GT-INC-023E9302\n",
	}
	for _, id := range ids {
		data, err := json.Marshal(validTaskDoc(id))
		require.NoError(t, err)
		_, decodeErr := harneval.DecodeTask(data)
		assert.Equal(t, decodeErr == nil, ValidTaskID(id), "id %q", id)
	}
}

func TestArea_DirExists_MissingIsFalseAndFileComponentIsUnsafe(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "evals/harness/tasks", "not a directory")
	a := openTestArea(t, root)

	exists, err := a.dirExists(IntakeDir)
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = a.dirExists(SurfaceTaskDir)
	assert.ErrorIs(t, err, errPathUnsafe)

	exists, err = a.dirExists("evals/harness")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestArea_ReadFile_ReturnsBytesOnlyForRegularFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "evals/harness/candidates/GTC-023e9302ff0b.json", "{}\n")
	writeFile(t, root, "evals/harness/candidates/promoted/x", "inner")
	a := openTestArea(t, root)

	data, err := a.readFile("evals/harness/candidates/GTC-023e9302ff0b.json")
	require.NoError(t, err)
	assert.Equal(t, "{}\n", string(data))

	_, err = a.readFile("evals/harness/candidates/promoted")
	assert.ErrorIs(t, err, errPathUnsafe, "a directory leaf is not a regular file")

	_, err = a.readFile("evals/harness/candidates/missing.json")
	assert.ErrorIs(t, err, fs.ErrNotExist)

	for _, rel := range []string{"../outside.json", "/abs.json", "evals/./harness", `evals\harness`, ""} {
		_, err = a.readFile(rel)
		assert.ErrorIs(t, err, errPathUnsafe, "path %q", rel)
	}
}

func TestArea_ListDir_SortsNamesAndTreatsMissingAsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "evals/harness/candidates/b.json", "b")
	writeFile(t, root, "evals/harness/candidates/a.json", "a")
	writeFile(t, root, "evals/harness/candidates/rejected/c.json", "c")
	a := openTestArea(t, root)

	entries, err := a.listDir(IntakeDir)
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"a.json", "b.json", "rejected"}, names)

	entries, err = a.listDir(PromotedDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestArea_WalkJSON_VisitsNestedJSONInLexicalOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, "evals/harness/tasks/surface/b.json", "b")
	writeFile(t, root, "evals/harness/tasks/surface/a/z.json", "z")
	writeFile(t, root, "evals/harness/tasks/surface/README.md", "ignored")
	writeFile(t, root, "evals/harness/tasks/surface/.GT-X.json.tmp-0011223344556677", "ignored")
	a := openTestArea(t, root)

	var visited []string
	err := a.walkJSON(SurfaceTaskDir, func(rel string, data []byte) error {
		visited = append(visited, rel+"="+string(data))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"evals/harness/tasks/surface/a/z.json=z",
		"evals/harness/tasks/surface/b.json=b",
	}, visited)
}

func TestArea_ActivePaths_ReadsManifestLeniently(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		manifest string
		want     []string
		unsafe   bool
		fails    bool
	}{
		{name: "no manifest"},
		{name: "extra fields ignored", manifest: `{"schema_version":"x","active_paths":["evals/harness/tasks/surface","evals/harness/tasks/agent"],"floors":{}}`,
			want: []string{"evals/harness/tasks/surface", "evals/harness/tasks/agent"}},
		{name: "malformed", manifest: `{"active_paths":[`, fails: true},
		{name: "quarantine path", manifest: `{"active_paths":["evals/harness/candidates/promoted"]}`, unsafe: true},
		{name: "escape", manifest: `{"active_paths":["evals/harness/../../x"]}`, unsafe: true},
		{name: "outside eval root", manifest: `{"active_paths":["pkg/tasks"]}`, unsafe: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if tc.manifest != "" {
				writeFile(t, root, harneval.ManifestPath, tc.manifest)
			}
			paths, err := openTestArea(t, root).activePaths()
			switch {
			case tc.unsafe:
				assert.ErrorIs(t, err, errPathUnsafe)
			case tc.fails:
				require.Error(t, err)
				assert.NotErrorIs(t, err, errPathUnsafe)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.want, paths)
			}
		})
	}
}
