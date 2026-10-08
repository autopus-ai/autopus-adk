package learn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// synthToken builds a GitHub fine-grained token shape at run time so no
// secret-shaped literal is committed. The alphabet spells no detector keyword.
func synthToken() string {
	return "github" + "_pat_" + strings.Repeat("BCDF2468", 5)
}

func TestRedactEvidenceField_RejectsInFixedOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		field  EvidenceField
		value  string
		detail string
	}{
		{"newline", FieldExpected, "line\nbreak", DetailControlChar},
		{"tab", FieldActual, "a\tb", DetailControlChar},
		{"escape", FieldRepro, "auto \x1b[2J", DetailControlChar},
		{"delete", FieldExpected, "x\x7f", DetailControlChar},
		{"c1 control", FieldActual, "x\u0085y", DetailControlChar},
		{"invalid utf-8", FieldActual, "x\xffy", DetailControlChar},
		// Step 1 runs before step 2: an oversized value with a control
		// character reports the control character.
		{"control before raw limit", FieldExpected, "\n" + strings.Repeat("x", 5000), DetailControlChar},
		// Step 2 runs before redaction: these values would redact under the
		// cap, so only a raw-length check rejects them.
		{"expected raw over limit", FieldExpected, "password=" + strings.Repeat("x", 4088), DetailRawOverLimit},
		{"repro raw over limit", FieldRepro, "password=" + strings.Repeat("x", 2040), DetailRawOverLimit},
		// Step 4 applies the cap to the redacted value.
		{"redaction grows past cap", FieldActual, "token=a " + strings.Repeat("x ", 506), DetailOverCapAfterRedaction},
		{"actual over cap", FieldActual, strings.Repeat("x", 1025), DetailOverCapAfterRedaction},
		{"repro over cap", FieldRepro, strings.Repeat("x", 513), DetailOverCapAfterRedaction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed, err := RedactEvidenceField(tt.field, tt.value)

			var fieldErr *FieldError
			require.True(t, errors.As(err, &fieldErr), "want *FieldError, got %v", err)
			assert.Equal(t, tt.field, fieldErr.Field)
			assert.Equal(t, tt.detail, fieldErr.Detail)
			assert.Equal(t, ReasonFieldInvalid, fieldErr.Reason())
			assert.Equal(t, "learning_field_invalid: "+string(tt.field)+": "+tt.detail, err.Error())
			assert.Empty(t, got)
			assert.False(t, changed)
		})
	}
}

func TestRedactEvidenceField_AcceptsAndRedacts(t *testing.T) {
	t.Parallel()
	token := synthToken()
	tests := []struct {
		name    string
		field   EvidenceField
		value   string
		want    string
		changed bool
	}{
		{"empty", FieldExpected, "", "", false},
		{"plain", FieldExpected, "e", "e", false},
		{"exact cap", FieldActual, strings.Repeat("x", 1024), strings.Repeat("x", 1024), false},
		{"exact repro cap", FieldRepro, strings.Repeat("x", 512), strings.Repeat("x", 512), false},
		{"raw over cap redacts under it", FieldActual, "password=" + strings.Repeat("x", 1100), "[REDACTED_SECRET]", true},
		{"token", FieldActual, token + " leaked", "[REDACTED_SECRET] leaked", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed, err := RedactEvidenceField(tt.field, tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.changed, changed)
		})
	}
}

func TestEvidenceField_Cap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1024, FieldExpected.Cap())
	assert.Equal(t, 1024, FieldActual.Cap())
	assert.Equal(t, 512, FieldRepro.Cap())
	assert.Equal(t, 0, EvidenceField("pattern").Cap(), "unknown fields have no cap and fail closed")

	_, _, err := RedactEvidenceField(EvidenceField("pattern"), "x")
	var fieldErr *FieldError
	require.True(t, errors.As(err, &fieldErr))
	assert.Equal(t, DetailRawOverLimit, fieldErr.Detail)
}

func TestAppendAtomic_EvidenceFields_StoredOnlyWhenSet(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)

	require.NoError(t, RecordFixPattern(store, RecordOpts{
		Pattern: "with evidence", Expected: "e", Actual: "a", Repro: "auto init",
	}))
	require.NoError(t, RecordFixPattern(store, RecordOpts{Pattern: "without evidence"}))

	lines := readLines(t, path)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], `"expected":"e"`)
	assert.Contains(t, lines[0], `"actual":"a"`)
	assert.Contains(t, lines[0], `"repro":"auto init"`)
	for _, key := range []string{`"expected"`, `"actual"`, `"repro"`} {
		assert.NotContains(t, lines[1], key, "an entry without evidence serializes no %s key", key)
	}

	entries, err := store.Read()
	require.NoError(t, err)
	assert.Equal(t, "e", entries[0].Expected)
	assert.Equal(t, "a", entries[0].Actual)
	assert.Equal(t, "auto init", entries[0].Repro)
}

func TestAppendAtomic_InvalidEvidence_LeavesStoreByteIdentical(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	require.NoError(t, RecordFixPattern(store, RecordOpts{Pattern: "existing"}))
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	err = RecordFixPattern(store, RecordOpts{Pattern: "p", Expected: "two\nlines"})

	var fieldErr *FieldError
	require.True(t, errors.As(err, &fieldErr))
	assert.Equal(t, FieldExpected, fieldErr.Field)
	assert.Equal(t, DetailControlChar, fieldErr.Detail)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

// TestRewrite_KeepsEvidenceFields guards probe A2: a rewrite through Prune or
// UpdateReuseCount used to drop fields the struct did not declare.
func TestRewrite_KeepsEvidenceFields(t *testing.T) {
	t.Parallel()
	store, path := newTestStore(t)
	require.NoError(t, RecordFixPattern(store, RecordOpts{
		Pattern: "kept", Expected: "e", Actual: "a", Repro: "r",
	}))
	entries, err := store.Read()
	require.NoError(t, err)

	require.NoError(t, store.UpdateReuseCount(entries[0].ID))
	removed, err := Prune(store, 30)
	require.NoError(t, err)
	require.Equal(t, 0, removed)

	var got LearningEntry
	lines := readLines(t, path)
	require.Len(t, lines, 1)
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &got))
	assert.Equal(t, "e", got.Expected)
	assert.Equal(t, "a", got.Actual)
	assert.Equal(t, "r", got.Repro)
	assert.Equal(t, 1, got.ReuseCount)
}

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)
	return store, filepath.Join(dir, ".autopus", "learnings", "pipeline.jsonl")
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
