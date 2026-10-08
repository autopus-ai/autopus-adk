package harneval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeOracleResult_WireContract: harness_oracle_result.v1 is decoded
// strictly. Only a compared output carries assertions, and an output is
// not_checked exactly when the artifact timed out, so no result can claim a
// comparison it did not make.
func TestDecodeOracleResult_WireContract(t *testing.T) {
	t.Parallel()
	compared := `{"schema_version":"harness_oracle_result.v1","task_id":"GT-AG-001","output_check":"ok",` +
		`"assertions":[{"id":"stdout","passed":true},{"id":"exit_status","passed":false}],"artifact_exit":3,"timed_out":false}`

	result, err := DecodeOracleResult([]byte(compared))

	require.NoError(t, err)
	exit := 3
	assert.Equal(t, OracleResult{
		SchemaVersion: OracleResultSchemaV1, TaskID: "GT-AG-001", OutputCheck: OutputCheckOK,
		Assertions: []OracleAssertion{{ID: "stdout", Passed: true}, {ID: "exit_status"}}, ArtifactExit: &exit,
	}, result)
	timedOut, err := DecodeOracleResult([]byte(`{"task_id":"GT-AG-001","output_check":"not_checked","assertions":[],"artifact_exit":null,"timed_out":true}`))
	require.NoError(t, err)
	assert.Equal(t, OracleResult{TaskID: "GT-AG-001", OutputCheck: OutputCheckNotChecked, Assertions: []OracleAssertion{}, TimedOut: true}, timedOut)

	tests := []struct{ name, detail, fragment, body string }{
		{"unknown field", DetailUnknownField, `unknown field "verdict"`,
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false,"verdict":"pass"}`},
		{"trailing data", DetailTrailingData, "data follows",
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}{}`},
		{"foreign schema", DetailFieldInvalid, "harness_oracle_result.v2",
			`{"schema_version":"harness_oracle_result.v2","task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"task id not a GT id", DetailFieldInvalid, `task_id "a06"`,
			`{"task_id":"a06","output_check":"ok","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"unknown output check", DetailFieldInvalid, `output_check "skipped"`,
			`{"task_id":"GT-AG-001","output_check":"skipped","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"timed out yet checked", DetailFieldInvalid, "not_checked exactly when",
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"stdout","passed":true}],"artifact_exit":null,"timed_out":true}`},
		{"not checked without a timeout", DetailFieldInvalid, "not_checked exactly when",
			`{"task_id":"GT-AG-001","output_check":"not_checked","assertions":[],"artifact_exit":0,"timed_out":false}`},
		{"rejected output with assertions", DetailFieldInvalid, "output_check link_rejected compared",
			`{"task_id":"GT-AG-001","output_check":"link_rejected","assertions":[{"id":"stdout","passed":true}],"artifact_exit":0,"timed_out":false}`},
		{"blank assertion id", DetailFieldInvalid, `assertion id ""`,
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"","passed":true}],"artifact_exit":0,"timed_out":false}`},
		{"repeated assertion id", DetailFieldInvalid, `assertion id "stdout"`,
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[{"id":"stdout","passed":true},{"id":"stdout","passed":true}],"artifact_exit":0,"timed_out":false}`},
		{"exit status a string", DetailFieldInvalid, "artifact_exit",
			`{"task_id":"GT-AG-001","output_check":"ok","assertions":[],"artifact_exit":"0","timed_out":false}`},
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
