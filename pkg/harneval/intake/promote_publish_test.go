package intake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// promotedTask is the S6 task file written by hand from the wire contract:
// harness_golden_task.v1 field order, two-space indent, final LF, the draft
// text from the representative L-002, and the person's category and assertion.
const promotedTask = `{
  "schema_version": "harness_golden_task.v1",
  "id": "GT-INC-023E9302",
  "kind": "surface",
  "category": "hooks_settings",
  "intent": "hook missing in codex",
  "outcome": "y",
  "variants": [],
  "assertions": [
    {
      "kind": "file_exists",
      "platform": "codex",
      "path": ".codex/hooks.json"
    }
  ],
  "provenance": {
    "kind": "incident",
    "ref": "L-002",
    "fingerprint": "023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86"
  },
  "status": {
    "state": "active",
    "reason": ""
  }
}
`

// promotedLinkRecord is the S6 harness_incident_link.v1 record: every
// learning ref of the group and the representative's evidence copies.
const promotedLinkRecord = `{
  "schema_version": "harness_incident_link.v1",
  "task_id": "GT-INC-023E9302",
  "candidate_id": "GTC-023e9302ff0b",
  "fingerprint_version": 1,
  "fingerprint": "023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86",
  "representative": "L-002",
  "learning_refs": [
    "L-002",
    "L-1000"
  ],
  "expected": "y",
  "actual": "ya",
  "repro": "auto init",
  "redacted": false,
  "redacted_fields": []
}
`

func TestPromote_S6CompleteDraft_PublishesTaskAndLinkThenRemovesCandidate(t *testing.T) {
	t.Parallel()
	// Given the S6 golden set, no baseline, and a completed draft.
	root := newCompletedProject(t)

	// When the candidate is promoted.
	result, err := runPromote(root, nil)

	// Then the task and the link hold the canonical bytes, the candidate and
	// every temp file are gone, and the task currently passes.
	require.NoError(t, err)
	assert.Equal(t, PromoteResult{
		SchemaVersion: PromoteResultSchemaV1, Result: PromoteResultPromoted,
		CandidateID: promoteCandidateID, TaskID: promoteTaskID,
		TaskPath: promoteTaskAt, LinkPath: promoteLinkAt, CurrentOutcome: OutcomePass,
	}, result)
	assert.Equal(t, promotedTask, readFile(t, root, promoteTaskAt))
	assert.Equal(t, promotedLinkRecord, readFile(t, root, promoteLinkAt))
	assert.NoFileExists(t, root+"/"+promoteCandidateAt)
	assert.Empty(t, tempNames(t, root))
	set, err := harneval.LoadSet(root)
	require.NoError(t, err)
	assert.Equal(t, 1, set.ActiveCount(harneval.KindAgent))
	assert.Equal(t, 2, set.ActiveCount(harneval.KindSurface))
}

func TestPromote_S6TargetAlreadyTaken_RefusedBeforePublishing(t *testing.T) {
	t.Parallel()
	incident := func(id, fingerprint string) map[string]any {
		task := validTaskDoc(id)
		task["provenance"] = map[string]any{"kind": "incident", "ref": "L-002", "fingerprint": fingerprint}
		return task
	}
	cases := []struct {
		name, reason, detail string
		seed                 func(t *testing.T, root string)
	}{
		{"same id in another kind directory", ReasonTaskIDExists, promoteAgentDir + "/" + promoteTaskID + ".json", func(t *testing.T, root string) {
			writeRecord(t, root, promoteAgentDir+"/"+promoteTaskID+".json", validTaskDoc(promoteTaskID))
		}},
		{"other bytes at the target", ReasonTaskIDExists, promoteTaskAt, func(t *testing.T, root string) {
			writeRecord(t, root, promoteTaskAt, validTaskDoc(promoteTaskID))
		}},
		{"active task of the same fingerprint", ResultAlreadyPromoted, "GT-INC-COPY", func(t *testing.T, root string) {
			writeRecord(t, root, SurfaceTaskDir+"/GT-INC-COPY.json", incident("GT-INC-COPY", fingerprintY))
		}},
		{"link of the same fingerprint", ResultAlreadyPromoted, "GT-INC-OLD", func(t *testing.T, root string) {
			writeRecord(t, root, PromotedDir+"/GT-INC-OLD.json", promotedLink(fingerprintY, "GT-INC-OLD", "L-002"))
		}},
		{"link of the task id for another fingerprint", ReasonTaskIDExists, promoteLinkAt, func(t *testing.T, root string) {
			writeRecord(t, root, promoteLinkAt, promotedLink(fingerprintX, promoteTaskID, "L-999"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			tc.seed(t, root)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, tc.reason, tc.detail)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_UnreadableLinkRecord_IsEvalLinksUnreadable(t *testing.T) {
	t.Parallel()
	root := newCompletedProject(t)
	writeFile(t, root, PromotedDir+"/GT-INC-OLD.json", "{")
	before := treeDigest(t, root)

	_, err := runPromote(root, nil)

	requirePromoteRefusal(t, err, ReasonEvalLinksUnreadable, "")
	assert.Equal(t, before, treeDigest(t, root))
}

// duplicateAfterPublish is the S6 seam: right after step 10 it puts a task
// of the promoted id into the agent kind directory.
func duplicateAfterPublish(t *testing.T, root string, stop error) func(int) error {
	return func(step int) error {
		if step == stepTaskPublished {
			writeRecord(t, root, promoteAgentDir+"/"+promoteTaskID+".json", validTaskDoc(promoteTaskID))
			return stop
		}
		return nil
	}
}

func TestPromote_S6SetInvalidAfterPublish_RollsBackOnlyTheTask(t *testing.T) {
	t.Parallel()
	// Given a completed draft and a seam that breaks the set right after the
	// task is published.
	root := newCompletedProject(t)
	initial := treeDigest(t, root)
	candidate := readFile(t, root, promoteCandidateAt)
	for round := range 2 {
		// When the candidate is promoted.
		_, err := runPromote(root, func(r *PromoteRequest) { r.interrupt = duplicateAfterPublish(t, root, nil) })

		// Then the task is rolled back with the loader detail, no link is
		// written, and the candidate is untouched; once the seam's file is
		// gone the tree is the initial one, so a rerun gives the same result.
		requirePromoteRefusal(t, err, ReasonPromoteRolledBack, harneval.DetailDuplicateTaskID)
		assert.NoFileExists(t, root+"/"+promoteTaskAt, "round %d", round)
		assert.NoFileExists(t, root+"/"+promoteLinkAt, "round %d", round)
		assert.Equal(t, candidate, readFile(t, root, promoteCandidateAt))
		removeRel(t, root, promoteAgentDir+"/"+promoteTaskID+".json")
		assert.Equal(t, initial, treeDigest(t, root))
	}
}

func TestPromote_CrashBeforePostCheck_RerunRollsBackTheBrokenSet(t *testing.T) {
	t.Parallel()
	// Given a promotion that stopped between steps 10 and 11 after its task
	// had broken the set.
	root := newCompletedProject(t)
	candidate := readFile(t, root, promoteCandidateAt)
	_, err := runPromote(root, func(r *PromoteRequest) { r.interrupt = duplicateAfterPublish(t, root, errPromoteCrash) })
	require.ErrorIs(t, err, errPromoteCrash)
	require.FileExists(t, root+"/"+promoteTaskAt)

	// When promote runs again.
	_, err = runPromote(root, nil)

	// Then it resumes at the post-check and rolls the task back.
	requirePromoteRefusal(t, err, ReasonPromoteRolledBack, harneval.DetailDuplicateTaskID)
	assert.NoFileExists(t, root+"/"+promoteTaskAt)
	assert.NoFileExists(t, root+"/"+promoteLinkAt)
	assert.Equal(t, candidate, readFile(t, root, promoteCandidateAt))
}

func TestPromote_LinkAppearingAfterCheckNine_KeepsTheCandidate(t *testing.T) {
	t.Parallel()
	// Given a link record of the task id that appears between the checks and
	// step 12.
	root := newCompletedProject(t)
	candidate := readFile(t, root, promoteCandidateAt)
	foreign := func(step int) error {
		if step == stepPostChecked {
			writeRecord(t, root, promoteLinkAt, promotedLink(fingerprintX, promoteTaskID, "L-999"))
		}
		return nil
	}

	// When the candidate is promoted.
	_, err := runPromote(root, func(r *PromoteRequest) { r.interrupt = foreign })

	// Then the foreign record is kept and so is the candidate, which goes
	// only after its own link record is durable.
	requirePromoteRefusal(t, err, ReasonTaskIDExists, promoteLinkAt)
	assert.Equal(t, candidate, readFile(t, root, promoteCandidateAt))
	assert.Empty(t, tempNames(t, root))
}

func TestPromote_NullRedactedFields_AreWrittenAsAnEmptyList(t *testing.T) {
	t.Parallel()
	root := newCompletedProject(t)
	editCandidate(t, root, func(c *Candidate) { c.RedactedFields = nil })

	_, err := runPromote(root, nil)

	require.NoError(t, err)
	assert.Equal(t, promotedLinkRecord, readFile(t, root, promoteLinkAt))
}
