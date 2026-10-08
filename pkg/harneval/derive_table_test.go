package harneval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deriveTable is testdata/derive-table.json, the REQ-HR-08 table over
// hand-made inputs that the trusted runner's golden_blackbox.derive judges
// in scripts/benchmarks/harness/test_golden_derive_table.py.
type deriveTable struct {
	About        string                      `json:"about"`
	TaskID       string                      `json:"task_id"`
	AssertionIDs []string                    `json:"assertion_ids"`
	Terminations map[string]AgentTermination `json:"terminations"`
	Results      map[string]string           `json:"results"`
	Cases        []struct {
		Name             string    `json:"name"`
		StageReached     string    `json:"stage_reached"`
		Signal           *string   `json:"signal"`
		AgentTermination string    `json:"agent_termination"`
		OracleResult     *string   `json:"oracle_result"`
		AssertionIDs     *[]string `json:"assertion_ids"`
		Want             *[6]any   `json:"want"`
	} `json:"cases"`
}

// TestDeriveTrial_SharedTable: the signer's table gives every hand-made case
// exactly what the trusted runner's table gives it, a judgement or no row.
func TestDeriveTrial_SharedTable(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "derive-table.json"))
	require.NoError(t, err)
	var table deriveTable
	require.NoError(t, strictDecode(data, &table))
	require.GreaterOrEqual(t, len(table.Cases), 40)
	for _, tc := range table.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			term, known := table.Terminations[tc.AgentTermination]
			require.True(t, known, tc.AgentTermination)
			record := Record{TaskID: table.TaskID, StageReached: tc.StageReached, AgentTermination: &term}
			if tc.Signal != nil {
				record.Signal = *tc.Signal
			}
			var result []byte
			if tc.OracleResult != nil {
				body, named := table.Results[*tc.OracleResult]
				require.True(t, named, *tc.OracleResult)
				result = []byte(body)
			}
			ids := table.AssertionIDs
			if tc.AssertionIDs != nil {
				ids = *tc.AssertionIDs
			}

			got, matched := deriveTrial(record, result, ids)

			if tc.Want == nil {
				assert.False(t, matched, "no row matches, got %+v", got)
				return
			}
			require.True(t, matched, "a row matches")
			want := *tc.Want
			assert.Equal(t, []any{want[0], want[1], want[2], want[3], want[4], want[5]},
				[]any{got.outcome, got.signal, got.oracle.Ran, got.oracle.BuildFailed,
					float64(got.oracle.ExpectedPassed), float64(got.oracle.ExpectedFailed)})
		})
	}
}
