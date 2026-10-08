package harneval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v1 opens every oracle result body below with its schema_version.
const v1 = `{"schema_version":"harness_oracle_result.v1",`

// TestDecodeOracleResult_WireContract: harness_oracle_result.v1 is decoded
// strictly. Only a compared output carries assertions, and an output is
// not_checked exactly when the artifact timed out, so no result can claim a
// comparison it did not make. Every key is present once and only
// artifact_exit may be null, as the trusted runner decodes it.
func TestDecodeOracleResult_WireContract(t *testing.T) {
	t.Parallel()
	compared := v1 + `"task_id":"GT-AG-001","output_check":"ok",` +
		`"assertions":[{"id":"stdout","passed":true},{"id":"exit_status","passed":false}],"artifact_exit":3,"timed_out":false}`

	result, err := DecodeOracleResult([]byte(compared))

	require.NoError(t, err)
	exit := 3
	assert.Equal(t, OracleResult{
		SchemaVersion: OracleResultSchemaV1, TaskID: "GT-AG-001", OutputCheck: OutputCheckOK,
		Assertions: []OracleAssertion{{ID: "stdout", Passed: true}, {ID: "exit_status"}}, ArtifactExit: &exit,
	}, result)
	timedOut, err := DecodeOracleResult([]byte(v1 + `"task_id":"GT-AG-001","output_check":"not_checked","assertions":[],"artifact_exit":null,"timed_out":true}`))
	require.NoError(t, err)
	assert.Equal(t, OracleResult{SchemaVersion: OracleResultSchemaV1, TaskID: "GT-AG-001", OutputCheck: OutputCheckNotChecked,
		Assertions: []OracleAssertion{}, TimedOut: true}, timedOut)

	tests := []struct{ name, detail, fragment, body string }{
		{"unknown field", DetailUnknownField, `unknown field "verdict"`,
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false,"verdict":"pass"}`},
		{"trailing data", DetailTrailingData, "data follows",
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}{}`},
		{"foreign schema", DetailFieldInvalid, "harness_oracle_result.v2",
			`{"schema_version":"harness_oracle_result.v2","task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"schema_version absent", DetailFieldInvalid, "schema_version is absent or null",
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"a repeated key", DetailFieldInvalid, `repeats the key "timed_out"`,
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":true,"timed_out":false}`},
		{"timed_out null", DetailFieldInvalid, "timed_out is absent or null",
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":null}`},
		{"passed absent", DetailFieldInvalid, "passed is absent or null",
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"stdout"}],"artifact_exit":0,"timed_out":false}`},
		{"task id not a GT id", DetailFieldInvalid, `task_id "a06"`,
			v1 + `"task_id":"a06","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"unknown output check", DetailFieldInvalid, `output_check "skipped"`,
			v1 + `"task_id":"GT-AG-001","output_check":"skipped","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"timed out yet checked", DetailFieldInvalid, "not_checked exactly when",
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"stdout","passed":true}],"artifact_exit":null,"timed_out":true}`},
		{"not checked without a timeout", DetailFieldInvalid, "not_checked exactly when",
			v1 + `"task_id":"GT-AG-001","output_check":"not_checked","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"rejected output with assertions", DetailFieldInvalid, "output_check link_rejected compared",
			v1 + `"task_id":"GT-AG-001","output_check":"link_rejected","assertions":[{"id":"stdout","passed":true}],"artifact_exit":0,"timed_out":false}`},
		{"blank assertion id", DetailFieldInvalid, `assertion id ""`,
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"","passed":true}],"artifact_exit":0,"timed_out":false}`},
		{"repeated assertion id", DetailFieldInvalid, `assertion id "stdout"`,
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"stdout","passed":true},{"id":"stdout","passed":true}],"artifact_exit":0,"timed_out":false}`},
		{"exit status a string", DetailFieldInvalid, "artifact_exit",
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":"0","timed_out":false}`},
		{"exit status above 255", DetailFieldInvalid, "artifact_exit 256 is outside 0..255",
			v1 + `"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":256,"timed_out":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeOracleResult([]byte(tt.body))

			requireInvalid(t, err, tt.detail)
			assert.Contains(t, err.Error(), tt.fragment)
		})
	}
}

// TestRejectRepeatedKeys_AtEveryDepth: a key repeated in a nested object or
// an array element is refused; equal keys in sibling objects are not repeats.
func TestRejectRepeatedKeys_AtEveryDepth(t *testing.T) {
	t.Parallel()
	require.NoError(t, rejectRepeatedKeys([]byte(`{"a":{"b":1},"c":[{"b":1},{"b":2}],"d":[[1],{"e":null}]}`)))
	for _, body := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{"c":[{"b":1,"b":1}]}`, `[{"x":{"y":[{"z":1,"z":1}]}}]`} {
		assert.Error(t, rejectRepeatedKeys([]byte(body)), body)
	}
}
