package harneval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The expectation and set digest values below were computed outside Go by the
// scratch oracle_expect.py (hashlib over hand-written canonical JSON).
const (
	oracleSurfaceDigest  = "59ba8cba7c76c869aa11acfe0c0c8540d70f15d6cf6a436a69e3c3d8a9a81905"
	oracleAgentDigest    = "0ef15594db1cf6b56beb4680f3b1be6f082cd566c112b3e70d023a76e2266188"
	oracleVariantDigest  = "60442050e32f4a2eff23c07de60e019d9a63efede95ad65002f3a63277d400bd"
	oracleSetDigest      = "b6e83ca287658ef51dd27772f0bb887a1fc39ee993a3b9fa0d64252a4514b2b7"
	oracleAgentSetDigest = "95681c12046c448b2746b8c1e036dff6e8e22bc9abeb2b67971f8f154a463818"
	oracleEmptySetDigest = "2c9c1b0c26ecc4e9697ba29a841da17d9e1c9277598cb5d35fcf2ca8dd800ccf"
)

func decodeFixtureTask(t *testing.T, task map[string]any) Task {
	t.Helper()
	decoded, err := DecodeTask([]byte(mustJSON(t, task)))
	require.NoError(t, err)
	return decoded
}

func TestExpectationDigest_MatchesIndependentOracle(t *testing.T) {
	t.Parallel()
	assert.Equal(t, oracleSurfaceDigest, ExpectationDigest(decodeFixtureTask(t, surfaceTask("GT-FIX-A"))))
	assert.Equal(t, oracleAgentDigest, ExpectationDigest(decodeFixtureTask(t, agentTask("GT-AG-001"))))

	variant := surfaceTask("GT-FIX-B")
	variant["variants"] = []any{map[string]any{"name": "arch-off", "overrides": map[string]any{OverridePreCommitArch: false}}}
	variant["assertions"] = []any{map[string]any{"kind": AssertJSONPathPresent, "platform": "claude-code",
		"path": ".claude/settings.json", "json_path": "hooks.PreToolUse[*].matcher", "value_contains": "Bash"}}
	assert.Equal(t, oracleVariantDigest, ExpectationDigest(decodeFixtureTask(t, variant)))
}

// TestExpectationDigest_IgnoresWordingButNotBehavior is the S4 digest core.
func TestExpectationDigest_IgnoresWordingButNotBehavior(t *testing.T) {
	t.Parallel()
	reworded := surfaceTask("GT-FIX-A")
	reworded["intent"], reworded["outcome"] = "another intent", "another outcome"
	reworded["category"], reworded["provenance"] = "hooks_settings", map[string]any{"kind": "incident", "ref": "x"}
	reworded["variants"] = nil
	assert.Equal(t, oracleSurfaceDigest, ExpectationDigest(decodeFixtureTask(t, reworded)))

	needle := surfaceTask("GT-FIX-A")
	needle["assertions"] = []any{map[string]any{"kind": AssertContains, "platform": "claude-code", "path": "x.md", "needle": "a"}}
	other := surfaceTask("GT-FIX-A")
	other["assertions"] = []any{map[string]any{"kind": AssertContains, "platform": "claude-code", "path": "x.md", "needle": "b"}}
	assert.NotEqual(t, ExpectationDigest(decodeFixtureTask(t, needle)), ExpectationDigest(decodeFixtureTask(t, other)))

	fewer := agentTask("GT-AG-001")
	fewer["expected_tests"] = []any{"TestOther"}
	assert.NotEqual(t, oracleAgentDigest, ExpectationDigest(decodeFixtureTask(t, fewer)))
	resealed := agentTask("GT-AG-001")
	resealed["corpus_ref"].(map[string]any)["file_sha256"] = sha256Of(fixtureCorpus + " ")
	assert.NotEqual(t, oracleAgentDigest, ExpectationDigest(decodeFixtureTask(t, resealed)))
}

func TestSetDigests_MatchIndependentOracle(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	set, err := LoadSet(f.root)
	require.NoError(t, err)

	assert.Equal(t, oracleSetDigest, SetDigest(set))
	assert.Equal(t, oracleAgentSetDigest, AgentSetDigest(set))
	assert.Equal(t, oracleEmptySetDigest, SetDigest(&Set{Manifest: Manifest{SetVersion: "1"}}))
}
