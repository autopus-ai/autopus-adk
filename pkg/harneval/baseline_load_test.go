package harneval

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDigest = strings.Repeat("ab", 32)

func baselineDoc(rows ...map[string]any) map[string]any {
	list := make([]any, 0, len(rows))
	for _, row := range rows {
		list = append(list, row)
	}
	return map[string]any{
		"schema_version": BaselineSchemaV1, "set_version": "1", "set_digest": testDigest, "rows": list,
	}
}

func baselineRow(id, kind, state, result string) map[string]any {
	row := map[string]any{"id": id, "kind": kind, "state": state, "result": result, "expectation_digest": testDigest}
	if state == StateRetired {
		row["retired_reason"] = "superseded"
	}
	return row
}

func TestLoadBaseline_MissingOrWithoutActiveSurfaceRows_IsBaselineMissing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	_, err := LoadBaseline(f.root)
	require.ErrorIs(t, err, ErrBaselineMissing)

	f.writeJSON(BaselinePath, baselineDoc(
		baselineRow("GT-AG-001", KindAgent, StateActive, ResultNotRun),
		baselineRow("GT-FIX-A", KindSurface, StateRetired, ResultPass),
	))
	_, err = LoadBaseline(f.root)
	require.ErrorIs(t, err, ErrBaselineMissing)
}

func TestLoadBaseline_ValidRows_Decode(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	accepted := baselineRow("GT-FIX-B", KindSurface, StateActive, ResultFail)
	accepted["accepted_regression_reason"] = "hook intentionally removed"
	f.writeJSON(BaselinePath, baselineDoc(
		baselineRow("GT-AG-001", KindAgent, StateActive, ResultNotRun),
		baselineRow("GT-FIX-A", KindSurface, StateActive, ResultPass),
		accepted,
	))

	baseline, err := LoadBaseline(f.root)

	require.NoError(t, err)
	require.Len(t, baseline.Rows, 3)
	assert.Equal(t, "hook intentionally removed", baseline.Rows[2].AcceptedRegressionReason)
}

func TestDecodeBaseline_RowContract_RejectsWithExactDetail(t *testing.T) {
	t.Parallel()
	active := func(id string) map[string]any { return baselineRow(id, KindSurface, StateActive, ResultPass) }
	cases := []struct {
		name   string
		doc    map[string]any
		detail string
	}{
		{"unsorted", baselineDoc(active("GT-FIX-B"), active("GT-FIX-A")), DetailFieldInvalid},
		{"duplicate", baselineDoc(active("GT-FIX-A"), active("GT-FIX-A")), DetailDuplicateTaskID},
		{"agent with result", baselineDoc(baselineRow("GT-AG-001", KindAgent, StateActive, ResultPass)), DetailFieldInvalid},
		{"surface not_run", baselineDoc(baselineRow("GT-FIX-A", KindSurface, StateActive, ResultNotRun)), DetailFieldInvalid},
		{"retired without reason", func() map[string]any {
			row := active("GT-FIX-A")
			row["state"] = StateRetired
			return baselineDoc(row)
		}(), DetailFieldInvalid},
		{"accepted on a pass", func() map[string]any {
			row := active("GT-FIX-A")
			row["accepted_regression_reason"] = "why"
			return baselineDoc(row)
		}(), DetailFieldInvalid},
		{"bad digest", func() map[string]any {
			row := active("GT-FIX-A")
			row["expectation_digest"] = "abc"
			return baselineDoc(row)
		}(), DetailFieldInvalid},
		{"bad set digest", func() map[string]any {
			doc := baselineDoc(active("GT-FIX-A"))
			doc["set_digest"] = ""
			return doc
		}(), DetailFieldInvalid},
		{"unknown field", func() map[string]any {
			doc := baselineDoc(active("GT-FIX-A"))
			doc["generated_at"] = "2026-10-07T00:00:00Z"
			return doc
		}(), DetailUnknownField},
	}
	for _, tc := range cases {
		_, err := DecodeBaseline([]byte(mustJSON(t, tc.doc)))
		requireInvalid(t, err, tc.detail)
	}
}
