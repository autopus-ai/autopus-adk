package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	qarelease "github.com/insajin/autopus-adk/pkg/qa/release"
)

func rejectionReasons(report Report) []string {
	out := make([]string, 0, len(report.Ingestion.Rejections))
	for _, rejection := range report.Ingestion.Rejections {
		out = append(out, rejection.Reason)
	}
	return out
}

func readJSONInto(t *testing.T, path string, target any) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, target))
}

// A release index that cannot be parsed must be reported as unreadable, not
// silently dropped: the gate matrix would otherwise vanish without explanation.
func TestBuildRejectsUnreadableReleaseIndex(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	path := filepath.Join(fx.root, ".autopus", "qa", "releases", fixtureReleaseID, ReleaseIndexFile)
	writeFile(t, path, "{not json")

	report, err := Build(fx.options())
	require.NoError(t, err)
	assert.Contains(t, rejectionReasons(report), "unreadable_release_index")
	assert.Nil(t, report.Release, "a refused release index must contribute no gate matrix")
}

// A release index from another schema generation cannot be projected safely, so
// its version must be named in the rejection rather than guessed at.
func TestBuildRejectsReleaseIndexWithUnsupportedSchema(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	path := writeReleaseIndex(t, fx.root)
	var index qarelease.Index
	readJSONInto(t, path, &index)
	index.SchemaVersion = "qamesh.release-index.v999"
	writeJSON(t, path, index)

	report, err := Build(fx.options())
	require.NoError(t, err)
	assert.Contains(t, rejectionReasons(report), "unsupported_schema_version:qamesh.release-index.v999")
	assert.Nil(t, report.Release, "a refused release index must contribute no gate matrix")
}

// Blocked redaction means the release index itself may carry unredacted text;
// it must be refused whole instead of contributing lane rows.
func TestBuildRejectsReleaseIndexWithBlockedRedaction(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	path := writeReleaseIndex(t, fx.root)
	var index qarelease.Index
	readJSONInto(t, path, &index)
	index.RedactionStatus = qarelease.RedactionBlocked
	writeJSON(t, path, index)

	report, err := Build(fx.options())
	require.NoError(t, err)
	assert.Contains(t, rejectionReasons(report), "redaction_blocked")
	assert.Nil(t, report.Release, "a refused release index must contribute no gate matrix")
}

// An explicitly passed release index path must be honoured over the newest one
// discovered under the releases directory.
func TestBuildHonoursExplicitReleaseIndexPath(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	explicit := filepath.Join(t.TempDir(), ReleaseIndexFile)
	writeFile(t, explicit, "{}")

	opts := fx.options()
	opts.ReleaseIndexPath = explicit
	report, err := Build(opts)
	require.NoError(t, err)
	// An empty object has no schema version, so the refusal proves the explicit
	// path was read instead of the valid discovered one.
	assert.Contains(t, rejectionReasons(report), "unsupported_schema_version:")
}

// A manifest ref listed twice must be ingested once: a duplicated journey would
// double every summary counter.
func TestBuildIngestsDuplicateManifestRefOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	in := ingestion{projectRoot: fx.root}
	ref := manifestRef("cli-fast")
	loadManifests(&in, []string{ref, ref, "./" + ref})

	assert.Len(t, in.manifests, 1)
	assert.Empty(t, in.rejections)
}

// Refs are rendered into the report, so each refusal class must be named
// distinctly; a generic failure would hide a traversal attempt.
func TestConfinedRefClassifiesRefusals(t *testing.T) {
	t.Parallel()
	// The project root is compared against resolved paths, so the temp dir must
	// be symlink-resolved the same way the loader resolves it.
	root, err := realPath(t.TempDir())
	require.NoError(t, err)
	cases := []struct {
		name   string
		ref    string
		reason string
	}{
		{"empty", "   ", "invalid_ref:empty"},
		{"control character", "runs/\nmanifest.json", "invalid_ref:control_character"},
		{"dot", ".", "unsafe_ref:path_traversal"},
		{"traversal", "../outside/manifest.json", "unsafe_ref:path_traversal"},
		{"absolute outside project", filepath.Join(t.TempDir(), "manifest.json"), "unsafe_ref:absolute_outside_project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, reason := confinedRef(root, tc.ref)
			assert.Empty(t, rel)
			assert.Equal(t, tc.reason, reason)
		})
	}

	// An absolute ref inside the project is accepted but rewritten relative so
	// no local user path is ever rendered.
	inside := filepath.Join(root, "runs", "a", "manifest.json")
	writeFile(t, inside, "{}")
	rel, reason := confinedRef(root, inside)
	assert.Empty(t, reason)
	assert.Equal(t, "runs/a/manifest.json", rel)
}

// A ref that escapes the project root has no relative form to show, so the
// display path must fall back to the raw ref instead of an empty string.
func TestIndexRefDisplayFallsBackForUnconfinedRefs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	assert.Empty(t, indexRefDisplay(root, "  "))
	assert.Equal(t, "../elsewhere/manifest.json", indexRefDisplay(root, "../elsewhere/manifest.json"))
	assert.Equal(t, "runs/a/manifest.json", indexRefDisplay(root, "runs/a/manifest.json"))
}

// An index larger than the read cap is not a summary; reading it would let a
// hostile evidence directory drive report memory use.
func TestBuildRefusesOversizedRunIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, RunIndexFile)
	require.NoError(t, os.WriteFile(path, make([]byte, 0), 0o644))
	require.NoError(t, os.Truncate(path, maxIndexBytes+1))

	opts := Options{ProjectDir: dir, RunIndexPath: path, Now: fixedNow}
	_, err := Build(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

// A multi-line validation error is rendered in a single rejection row, so only
// its first line may be carried.
func TestFirstLineTrimsMultilineReasons(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "first", firstLine("first\nsecond"))
	assert.Equal(t, "first", firstLine("first\r\nsecond"))
	assert.Equal(t, "only", firstLine("only"))
}
