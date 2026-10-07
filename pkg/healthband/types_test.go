package healthband_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

func jsonKeys(t *testing.T, data []byte) map[string]json.RawMessage {
	t.Helper()
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &keys))
	return keys
}

// Data Contracts: an evaluation without enough baseline blocks carries n and
// x but omits mu, sd, sd_eff, z, and tier, and nothing is ever null.
func TestEvent_EvaluationOmitsAbsentStatisticsAndNeverWritesNull(t *testing.T) {
	t.Parallel()
	evaluation := healthband.EvaluateValues(valuesFromBlocks(repeat(0, 19), 1))
	evaluation.Series, evaluation.SampleKey = "canary.failure_rate:local", "c80"
	event := healthband.Event{
		Schema: healthband.SchemaBandEvaluation, Seq: 7, Kind: healthband.EventKindEvaluation,
		Evaluation: evaluation, Action: healthband.ActionLog,
	}

	data, err := json.Marshal(event)

	require.NoError(t, err)
	assert.NotContains(t, string(data), "null")
	keys := jsonKeys(t, data)
	for _, key := range []string{"schema", "seq", "kind", "series", "sample_key", "n", "x", "reasons", "constants", "action"} {
		assert.Contains(t, keys, key)
	}
	for _, key := range []string{"mu", "sd", "sd_eff", "z", "tier", "episode_id", "claims", "claim_id", "prompt_manifest"} {
		assert.NotContains(t, keys, key)
	}
	assert.JSONEq(t, `{"k":4,"w":30,"n_min":20,"floor":0.25,"eps":1e-9}`, string(keys["constants"]))
	assert.JSONEq(t, `["insufficient_samples"]`, string(keys["reasons"]))
	assert.JSONEq(t, `19`, string(keys["n"]))
}

// A zero n is a real value, not an absent one: it must still be written.
func TestEvent_ZeroBaselineCountIsWritten(t *testing.T) {
	t.Parallel()
	event := healthband.Event{Evaluation: healthband.EvaluateValues(repeat(1, 4))}

	data, err := json.Marshal(event)

	require.NoError(t, err)
	keys := jsonKeys(t, data)
	assert.JSONEq(t, `0`, string(keys["n"]))
	assert.JSONEq(t, `1`, string(keys["x"]))
}

// Data Contracts: action_result adds claim_id, diagnosis_status, bs_id,
// bs_status, and prompt_manifest, and claims carry no status in events.
func TestEvent_ActionResultFieldsAndClaimShape(t *testing.T) {
	t.Parallel()
	lease := time.Date(2026, 10, 6, 1, 15, 30, 0, time.UTC)
	tier := 3
	event := healthband.Event{
		Schema: healthband.SchemaBandEvaluation, Seq: 9, Kind: healthband.EventKindActionResult,
		Evaluation: healthband.Evaluation{Series: "ci.failure_rate:CI", SampleKey: "1042"},
		Action:     healthband.ActionDiagnose, EpisodeID: "e1042", MaxTier: &tier,
		Claims:  []healthband.Claim{{ID: "k1", Kind: healthband.ClaimKindDiagnose, Owner: "h:1:ab", LeaseUntil: lease}},
		ClaimID: "k1", DiagnosisStatus: "unavailable(provider_missing)", BSID: "BS-BAND-013", BSStatus: "written",
		PromptManifest: []promptlayer.ManifestEntry{{ID: "band.instructions.v1", Kind: promptlayer.KindStable, Hash: "abc"}},
	}

	data, err := json.Marshal(event)

	require.NoError(t, err)
	keys := jsonKeys(t, data)
	for _, key := range []string{"claim_id", "diagnosis_status", "bs_id", "bs_status", "prompt_manifest", "episode_id", "max_tier"} {
		assert.Contains(t, keys, key)
	}
	assert.NotContains(t, keys, "n")
	assert.JSONEq(t, `[{"id":"k1","kind":"diagnose","owner":"h:1:ab","lease_until":"2026-10-06T01:15:30Z"}]`, string(keys["claims"]))
	var back healthband.Event
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, event, back)
}

// Data Contracts: the checkpoint keeps last_seq and per-series last_key and
// episodes with claims; equal state always marshals to equal bytes.
func TestCheckpoint_RoundTripsAndMarshalsDeterministically(t *testing.T) {
	t.Parallel()
	interrupted := time.Date(2026, 10, 6, 2, 0, 0, 0, time.UTC)
	state := healthband.Checkpoint{
		Schema: healthband.SchemaBandState, LastSeq: 12,
		Series: map[string]healthband.SeriesState{
			"ci.failure_rate:CI": {LastKey: "1042", Episodes: []healthband.Episode{{
				ID: "e1042", Open: true, MaxTier: 3, BSID: "BS-BAND-013",
				Claims: []healthband.Claim{{
					ID: "k1", Kind: healthband.ClaimKindDiagnose, Status: healthband.ClaimInterrupted,
					Owner: "h:1:ab", LeaseUntil: interrupted.Add(-time.Minute), InterruptedAt: &interrupted,
				}},
			}}},
			"canary.failure_rate:local": {LastKey: "c80"},
		},
	}

	first, err := json.Marshal(state)
	require.NoError(t, err)
	second, err := json.Marshal(state)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.NotContains(t, string(first), "null")
	assert.Less(t, strings.Index(string(first), "canary.failure_rate:local"), strings.Index(string(first), "ci.failure_rate:CI"))
	var back healthband.Checkpoint
	require.NoError(t, json.Unmarshal(first, &back))
	assert.Equal(t, state, back)
}
