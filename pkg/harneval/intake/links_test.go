package intake

import (
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s7Manifest declares the surface and agent active paths; the agent directory
// is never created, which counts as empty.
const s7Manifest = `{"active_paths":["evals/harness/tasks/surface","evals/harness/tasks/agent"]}`

// s7Area builds the S7 intake area below a new root: an open candidate whose
// group is L-001 and L-004, the promoted links of Y (L-002) and of X (L-999 and
// the non-representative L-1000), a rejection naming L-003, and the .gitkeep and
// README a maintainer may keep there. withManifest adds the manifest and its
// active tasks: Y's incident task, a lenient-only incident task naming L-777,
// a retired incident tombstone naming L-888, and a manual task.
func s7Area(t *testing.T, withManifest bool) string {
	t.Helper()
	root := t.TempDir()
	first := Entry{ID: "L-001", Type: "gate_fail", Pattern: "codex hook timeout", Expected: "e", Actual: "a"}
	again := first
	again.ID = "L-004"
	created := runIntake(t, root, []Entry{again, first}, nil)
	require.Equal(t, []string{"L-001", "L-004"}, created.Rows[0].LearningRefs)
	writeRecord(t, root, PromotedDir+"/GT-INC-023E9302.json", promotedLink(fingerprintY, "GT-INC-023E9302", "L-002"))
	writeRecord(t, root, PromotedDir+"/GT-INC-8E80C7A1.json", promotedLink(fingerprintX, "GT-INC-8E80C7A1", "L-999", "L-1000"))
	writeRecord(t, root, RejectedDir+"/GTC-"+fingerprintYPrime[:12]+".json", rejection(fingerprintYPrime, "L-003"))
	writeFile(t, root, IntakeDir+"/.gitkeep", "")
	writeFile(t, root, IntakeDir+"/README", "notes\n")
	writeFile(t, root, PromotedDir+"/README.md", "notes\n")
	if !withManifest {
		return root
	}
	writeFile(t, root, harneval.ManifestPath, s7Manifest)
	incident := validTaskDoc("GT-INC-023E9302")
	incident["provenance"] = map[string]any{"kind": "incident", "ref": "L-002", "fingerprint": fingerprintY}
	writeRecord(t, root, SurfaceTaskDir+"/GT-INC-023E9302.json", incident)
	writeFile(t, root, SurfaceTaskDir+"/nested/GT-INC-77777777.json",
		`{"id":"GT-INC-77777777","extra":true,"provenance":{"kind":"incident","ref":"L-777"}}`)
	retired := validTaskDoc("GT-INC-88888888")
	retired["provenance"] = map[string]any{"kind": "incident", "ref": "L-888", "fingerprint": strings.Repeat("8", 64)}
	retired["status"] = map[string]any{"state": "retired", "reason": "superseded"}
	writeRecord(t, root, SurfaceTaskDir+"/GT-INC-88888888.json", retired)
	writeRecord(t, root, SurfaceTaskDir+"/GT-MANUAL-1.json", validTaskDoc("GT-MANUAL-1"))
	return root
}

func TestProtectedLearningIDs_S7WithManifest_KeepsGroupLinkAndIncidentTaskRefs(t *testing.T) {
	t.Parallel()
	root := s7Area(t, true)
	before := treeDigest(t, root)

	ids, err := ProtectedLearningIDs(root)

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{
		"L-001": true, "L-004": true, "L-002": true, "L-999": true, "L-1000": true, "L-777": true, "L-888": true,
	}, ids, "rejections and manual provenance protect nothing")
	assert.Equal(t, before, treeDigest(t, root), "the scan writes nothing")
}

func TestProtectedLearningIDs_S7WithoutManifest_ReadsTheIntakeAreaAlone(t *testing.T) {
	t.Parallel()
	root := s7Area(t, false)

	ids, err := ProtectedLearningIDs(root)

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"L-001": true, "L-004": true, "L-002": true, "L-999": true, "L-1000": true}, ids)
}

func TestProtectedLearningIDs_NoIntakeAreaOrManifest_ProtectsNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, SurfaceTaskDir+"/GT-INC-023E9302.json", "{ tasks without a manifest are never read")

	ids, err := ProtectedLearningIDs(root)

	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Equal(t, []string{"tasks"}, dirNames(t, root, harneval.EvalRoot), "nothing is created")
}

func TestProtectedLearningIDs_MalformedRejection_IsNeverRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, RejectedDir+"/GTC-8e80c7a18029.json", "{")

	ids, err := ProtectedLearningIDs(root)

	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestProtectedLearningIDs_S7UnreadableFile_FailsClosed(t *testing.T) {
	t.Parallel()
	candidate := IntakeDir + "/GTC-" + Fingerprint(Entry{ID: "L-001", Type: "gate_fail", Pattern: "codex hook timeout"})[:12] + ".json"
	cases := map[string]func(t *testing.T, root string){
		"broken candidate JSON": func(t *testing.T, root string) { writeFile(t, root, candidate, "{") },
		"candidate unknown field": func(t *testing.T, root string) {
			writeFile(t, root, candidate, strings.Replace(readFile(t, root, candidate), "{", `{"extra":1,`, 1))
		},
		"candidate fingerprint_version 0": func(t *testing.T, root string) {
			writeFile(t, root, candidate, strings.ReplaceAll(readFile(t, root, candidate), `"fingerprint_version": 1`, `"fingerprint_version": 0`))
		},
		"candidate name is a directory": func(t *testing.T, root string) {
			writeFile(t, root, IntakeDir+"/GTC-000000000000.json/inner", "x")
		},
		"broken promoted link": func(t *testing.T, root string) {
			writeFile(t, root, PromotedDir+"/GT-INC-023E9302.json", `{"schema_version":"harness_incident_link.v1",`)
		},
		"broken manifest":      func(t *testing.T, root string) { writeFile(t, root, harneval.ManifestPath, "{") },
		"broken incident task": func(t *testing.T, root string) { writeFile(t, root, SurfaceTaskDir+"/GT-INC-023E9302.json", "{") },
		"provenance of the wrong type": func(t *testing.T, root string) {
			writeFile(t, root, SurfaceTaskDir+"/GT-INC-023E9302.json", `{"provenance":"incident"}`)
		},
		"empty promoted link": func(t *testing.T, root string) { writeFile(t, root, PromotedDir+"/GT-INC-8E80C7A1.json", "") },
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := s7Area(t, true)
			seed(t, root)

			ids, err := ProtectedLearningIDs(root)

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, ReasonEvalLinksUnreadable, runErr.Reason)
			assert.True(t, strings.HasPrefix(err.Error(), "eval_links_unreadable: "), err.Error())
			assert.Nil(t, ids)
		})
	}
}

func TestProtectedLearningIDs_PromotedDirectoryIsAFile_FailsClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, root, PromotedDir, "not a directory")

	_, err := ProtectedLearningIDs(root)

	assert.ErrorIs(t, err, errPathUnsafe)
	var runErr *RunError
	require.ErrorAs(t, err, &runErr)
	assert.Equal(t, ReasonEvalLinksUnreadable, runErr.Reason)
}

func TestProtectedLearningIDs_MissingRoot_FailsClosed(t *testing.T) {
	t.Parallel()

	_, err := ProtectedLearningIDs(t.TempDir() + "/absent")

	var runErr *RunError
	require.ErrorAs(t, err, &runErr)
	assert.Equal(t, ReasonEvalLinksUnreadable, runErr.Reason)
}
