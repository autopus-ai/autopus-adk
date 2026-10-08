package intake

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// csi is a raw 8-bit C1 control (CSI). It is not valid UTF-8: a terminal
// reads it as a control, and JSON encoding rewrites it as a three-byte U+FFFD.
const csi = "\x9b"

// TestRun_RawC1Bytes_AreRefusedLikeTheLearnStore pins review C3: intake
// checks learning text with the learn store's own validator, so an
// undecodable C1 byte is a control character in a stored field, a flag value,
// and a pattern, and a value at the cap cannot grow past it once encoded.
func TestRun_RawC1Bytes_AreRefusedLikeTheLearnStore(t *testing.T) {
	t.Parallel()
	skippedX := Row{LearningID: "L-1000", Result: ResultSkipped, Fingerprint: fingerprintX, Reason: ReasonLearningFieldInvalid}
	for name, value := range map[string]string{
		"C1 byte in a stored actual":    "seen " + csi + "2J",
		"1024 undecodable actual bytes": strings.Repeat(csi, 1024),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, entries := t.TempDir(), s3Entries()
			entries[0].Actual = value // L-1000, a member of group X

			result := runIntake(t, root, entries, nil)

			require.Len(t, result.Rows, 3)
			assert.Equal(t, skippedX, result.Rows[2])
		})
	}

	t.Run("C1 byte in a flag value", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()

		_, err := Run(Request{Root: root, Entries: s3Entries(), LearningIDs: []string{"L-010"},
			Expected: "e", Actual: "a" + csi, Redactor: noRedaction})

		var runErr *RunError
		require.ErrorAs(t, err, &runErr)
		assert.Equal(t, ReasonLearningFieldInvalid, runErr.Reason)
		assert.Equal(t, "actual: "+DetailControlChar, runErr.Detail)
		assert.NoDirExists(t, filepath.Join(root, "evals"))
	})

	t.Run("C1 byte in a pattern", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		entry := Entry{ID: "L-001", Type: "gate_fail", Pattern: "hook " + csi + "31m red", Expected: "e", Actual: "a"}

		result := runIntake(t, root, []Entry{entry}, nil)

		require.Len(t, result.Rows, 1)
		assert.Equal(t, ResultSkipped, result.Rows[0].Result)
		assert.Equal(t, ReasonCandidateTextInvalid, result.Rows[0].Reason)
		assert.NoDirExists(t, filepath.Join(root, "evals"))
	})
}

func TestReject_RawC1ByteInReason_IsRefused(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runIntake(t, root, s3Entries(), nil)
	before := treeDigest(t, root)

	_, err := rejectX(root, "not a harness issue "+csi)

	var runErr *RunError
	require.ErrorAs(t, err, &runErr)
	assert.Equal(t, ReasonLearningFieldInvalid, runErr.Reason)
	assert.Equal(t, "reason: "+DetailControlChar, runErr.Detail)
	assert.Equal(t, before, treeDigest(t, root))
}
