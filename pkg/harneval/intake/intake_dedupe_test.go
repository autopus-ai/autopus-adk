package intake

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func explicit(ids ...string) func(*Request) {
	return func(r *Request) { r.AllEligible, r.LearningIDs = false, ids }
}

func promotedLink(fingerprint, taskID string, refs ...string) Link {
	return Link{SchemaVersion: LinkSchemaV1, TaskID: taskID, CandidateID: candidateIDFor(fingerprint),
		FingerprintVersion: 1, Fingerprint: fingerprint, Representative: refs[0], LearningRefs: refs,
		Expected: "e", Actual: "a", RedactedFields: []string{}}
}

func rejection(fingerprint string, refs ...string) Rejection {
	return Rejection{SchemaVersion: RejectionSchemaV1, CandidateID: candidateIDFor(fingerprint),
		FingerprintVersion: 1, Fingerprint: fingerprint, LearningRefs: refs, Reason: "not a harness issue"}
}

func TestRun_S4Rerun_WritesNothingAndReportsExplicitDuplicates(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	writeFile(t, root, IntakeDir+"/.gitkeep", "")
	writeFile(t, root, IntakeDir+"/README", "notes")
	before := treeDigest(t, filepath.Join(root, IntakeDir))

	rerun := runIntake(t, root, s3Entries(), nil)
	single := runIntake(t, root, s3Entries(), explicit("L-1000"))

	assert.Equal(t, []Row{}, rerun.Rows)
	assert.Equal(t, []Row{{LearningID: "L-1000", Result: ResultDuplicateCandidate, Match: "GTC-8e80c7a18029", Fingerprint: fingerprintX}}, single.Rows)
	assert.Equal(t, 0, single.ExitCode())
	assert.Equal(t, before, treeDigest(t, filepath.Join(root, IntakeDir)))
}

func TestRun_S4PromotedFingerprint_ReportsAlreadyPromoted(t *testing.T) {
	t.Parallel()
	l1001 := entryY("L-1001")
	l1001.Expected, l1001.Actual = "y", "ya"
	byLink, byTask := t.TempDir(), t.TempDir()
	writeRecord(t, byLink, PromotedDir+"/GT-INC-023E9302.json", promotedLink(fingerprintY, "GT-INC-023E9302", "L-002"))
	task := validTaskDoc("GT-INC-023E9302")
	task["provenance"] = map[string]any{"kind": "incident", "ref": "L-002", "fingerprint": fingerprintY}
	writeRecord(t, byTask, SurfaceTaskDir+"/GT-INC-023E9302.json", task)
	writeFile(t, byTask, harneval.ManifestPath, `{"active_paths":["evals/harness/tasks/surface","evals/harness/tasks/agent"]}`)

	for _, root := range []string{byLink, byTask} {
		before := treeDigest(t, root)
		single := runIntake(t, root, []Entry{l1001}, explicit("L-1001"))
		all := runIntake(t, root, []Entry{l1001}, nil)

		assert.Equal(t, []Row{{LearningID: "L-1001", Result: ResultAlreadyPromoted, Match: "GT-INC-023E9302", Fingerprint: fingerprintY}}, single.Rows)
		assert.Equal(t, []Row{}, all.Rows)
		assert.Equal(t, before, treeDigest(t, root))
	}
}

func TestRun_RetiredOrMalformedIDIncidentTask_DoesNotBlockIntake(t *testing.T) {
	t.Parallel()
	retired := validTaskDoc("GT-INC-023E9302")
	retired["status"] = map[string]any{"state": "retired", "reason": "superseded"}
	badID := validTaskDoc("gt-inc-\x1b[0m")
	for _, task := range []map[string]any{retired, badID} {
		root := t.TempDir()
		task["provenance"] = map[string]any{"kind": "incident", "ref": "L-002", "fingerprint": fingerprintY}
		writeRecord(t, root, SurfaceTaskDir+"/GT-INC-023E9302.json", task)
		writeFile(t, root, harneval.ManifestPath, `{"active_paths":["evals/harness/tasks/surface"]}`)

		result := runIntake(t, root, s3Entries(), explicit("L-002"))

		assert.Equal(t, ResultCreated, result.Rows[0].Result)
	}
}

func TestRun_S4RejectedFingerprint_IsNeverRecreated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRecord(t, root, RejectedDir+"/GTC-8e80c7a18029.json", rejection(fingerprintX, "L-999", "L-1000"))

	all := runIntake(t, root, s3Entries(), nil)
	single := runIntake(t, root, s3Entries(), explicit("L-999"))

	assert.Equal(t, []Row{{LearningID: "L-002", Result: ResultCreated, CandidateID: "GTC-023e9302ff0b", Fingerprint: fingerprintY, LearningRefs: []string{"L-002"}}}, all.Rows)
	assert.Equal(t, []Row{{LearningID: "L-999", Result: ResultAlreadyRejected, Match: "GTC-8e80c7a18029", Fingerprint: fingerprintX}}, single.Rows)
	assert.NoFileExists(t, filepath.Join(root, IntakeDir, "GTC-8e80c7a18029.json"))
}

func TestRun_S4ReusedID_WithNewFingerprint_IsANewIncident(t *testing.T) {
	t.Parallel()
	// Given L-001 (fingerprint X) was rejected and pruned, and a new incident
	// received the same id with fingerprint Z.
	root := t.TempDir()
	writeRecord(t, root, RejectedDir+"/GTC-8e80c7a18029.json", rejection(fingerprintX, "L-001"))
	reused := Entry{ID: "L-001", Type: "gate_fail", Pattern: "codex hook timeout", Expected: "e", Actual: "a"}
	again := entryX("L-002")
	again.Expected, again.Actual = "x", "a"
	fingerprintZ := Fingerprint(reused)

	all := runIntake(t, root, []Entry{reused, again}, nil)
	single := runIntake(t, root, []Entry{reused, again}, explicit("L-002"))

	assert.Equal(t, []Row{{LearningID: "L-001", Result: ResultCreated, CandidateID: "GTC-" + fingerprintZ[:12], Fingerprint: fingerprintZ, LearningRefs: []string{"L-001"}}}, all.Rows)
	assert.Equal(t, ResultAlreadyRejected, single.Rows[0].Result)
}

func TestRun_S4CandidateIDCollision_SkipsGroupAndKeepsFixture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	path := IntakeDir + "/GTC-8e80c7a18029.json"
	other := strings.Repeat("ab", 32)
	writeFile(t, root, path, strings.ReplaceAll(readFile(t, root, path), fingerprintX, other))
	before := treeDigest(t, filepath.Join(root, IntakeDir))

	result := runIntake(t, root, s3Entries(), nil)

	assert.Equal(t, []Row{
		{LearningID: "L-999", Result: ResultSkipped, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, Reason: ReasonCandidateIDCollision},
		{LearningID: "L-1000", Result: ResultSkipped, CandidateID: "GTC-8e80c7a18029", Fingerprint: fingerprintX, Reason: ReasonCandidateIDCollision},
	}, result.Rows)
	assert.Equal(t, 2, result.ExitCode())
	assert.Equal(t, before, treeDigest(t, filepath.Join(root, IntakeDir)))
}

func TestRun_UnreadableRecord_FailsRunWithoutWriting(t *testing.T) {
	t.Parallel()
	cases := map[string]func(root string){
		"malformed candidate": func(root string) { writeFile(t, root, IntakeDir+"/GTC-000000000000.json", "{") },
		"candidate unknown field": func(root string) {
			writeFile(t, root, IntakeDir+"/GTC-000000000000.json", `{"schema_version":"harness_golden_candidate.v1","extra":1}`)
		},
		"link wrong schema": func(root string) {
			link := promotedLink(fingerprintY, "GT-INC-023E9302", "L-002")
			link.SchemaVersion = "harness_incident_link.v0"
			writeRecord(t, root, PromotedDir+"/GT-INC-023E9302.json", link)
		},
		"rejection trailing data": func(root string) {
			writeFile(t, root, RejectedDir+"/GTC-8e80c7a18029.json", `{"schema_version":"harness_candidate_rejection.v1"}{}`)
		},
		"rejection bad fingerprint": func(root string) {
			record := rejection(fingerprintX, "L-999")
			record.Fingerprint = "X"
			writeRecord(t, root, RejectedDir+"/GTC-8e80c7a18029.json", record)
		},
		"malformed manifest": func(root string) { writeFile(t, root, harneval.ManifestPath, "{") },
		"malformed active task": func(root string) {
			writeFile(t, root, harneval.ManifestPath, `{"active_paths":["evals/harness/tasks/surface"]}`)
			writeFile(t, root, SurfaceTaskDir+"/GT-INC-023E9302.json", `{"provenance":"x"}`)
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			seed(root)
			before := treeDigest(t, root)

			_, err := Run(Request{Root: root, Entries: s3Entries(), AllEligible: true, Redactor: noRedaction})

			var runErr *RunError
			require.ErrorAs(t, err, &runErr)
			assert.Equal(t, ReasonEvalLinksUnreadable, runErr.Reason)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestRun_MissingProjectRoot_FailsWithoutCreatingIt(t *testing.T) {
	t.Parallel()
	absent := filepath.Join(t.TempDir(), "absent")

	_, err := Run(Request{Root: absent, Entries: s3Entries(), AllEligible: true, Redactor: noRedaction})

	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, absent)
}
