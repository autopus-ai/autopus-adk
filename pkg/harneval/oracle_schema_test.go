package harneval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Black-box fixture bytes of GT-AG-001 and the expectation digest of
// blackBoxTask("GT-AG-001"), computed outside Go with hashlib over the
// hand-written canonical JSON {kind, variants, assertions, corpus_ref,
// expected_tests, oracle_mode, black_box_oracle}.
const (
	fixtureOracleInput   = `{"rows":[1,2]}` + "\n"
	fixtureOracleStdout  = "two rows\n"
	fixtureOracleReport  = `{"rows":2}` + "\n"
	oracleBlackBoxDigest = "78ec14697a24ef8e4a1fdb8faa7d62ddb2d6e03361131bee03057d85a49c840b"
)

func oracleFixture(name string) string { return OracleFixtureRoot + "/GT-AG-001/" + name }

func pinned(name, body string) map[string]any {
	return map[string]any{"path": oracleFixture(name), "sha256": sha256Of(body)}
}

// blackBoxTask is agentTask(id) with a black-box oracle over the three fixtures.
func blackBoxTask(id string) map[string]any {
	task := agentTask(id)
	task["oracle_mode"] = OracleModeBlackBox
	task["black_box_oracle"] = map[string]any{
		"build":   "./cmd/tool",
		"command": []any{"{artifact}", "count", "--input", "{input}/rows.json"},
		"inputs":  []any{pinned("rows.json", fixtureOracleInput)},
		"assertions": []any{
			map[string]any{"id": "exit", "kind": BlackBoxExitCode, "exit_code": 0},
			map[string]any{"id": "stdout", "kind": BlackBoxStdout, "expected": pinned("stdout.txt", fixtureOracleStdout)},
			map[string]any{"id": "report", "kind": BlackBoxFile, "path": "out/report.json", "expected": pinned("report.json", fixtureOracleReport)},
		},
	}
	return task
}

func oracleOf(task map[string]any) map[string]any { return task["black_box_oracle"].(map[string]any) }

func assertionOf(task map[string]any, index int) map[string]any {
	return oracleOf(task)["assertions"].([]any)[index].(map[string]any)
}

// writeBlackBox writes the black-box task and its three fixtures into a
// standard fixture tree.
func (f *fixture) writeBlackBox(task map[string]any) {
	f.t.Helper()
	f.writeJSON(agentPath(task["id"].(string)), task)
	f.write(oracleFixture("rows.json"), fixtureOracleInput)
	f.write(oracleFixture("stdout.txt"), fixtureOracleStdout)
	f.write(oracleFixture("report.json"), fixtureOracleReport)
}

func TestDecodeTask_BlackBoxOracle_DecodesTheRunnerFormat(t *testing.T) {
	t.Parallel()
	task := decodeFixtureTask(t, blackBoxTask("GT-AG-001"))

	require.NotNil(t, task.BlackBoxOracle)
	assert.Equal(t, OracleModeBlackBox, task.OracleMode)
	assert.Equal(t, []string{"{artifact}", "count", "--input", "{input}/rows.json"}, task.BlackBoxOracle.Command)
	assert.Equal(t, 0, *task.BlackBoxOracle.Assertions[0].ExitCode)
	assert.Equal(t, "out/report.json", *task.BlackBoxOracle.Assertions[2].Path)
	assert.Nil(t, task.BlackBoxOracle.Stdin)

	white := agentTask("GT-AG-002")
	white["oracle_mode"] = OracleModeWhiteBox
	assert.Nil(t, decodeFixtureTask(t, white).BlackBoxOracle)
}

// TestDecodeTask_BlackBoxOracle_RejectsWithExactDetail: every rule the trusted
// runner applies to a definition rejects the task document with its detail.
func TestDecodeTask_BlackBoxOracle_RejectsWithExactDetail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(task map[string]any)
		detail string
	}{
		{"unknown mode", func(task map[string]any) { task["oracle_mode"] = "grey_box" }, DetailFieldInvalid},
		{"black_box without definition", func(task map[string]any) { delete(task, "black_box_oracle") }, DetailFieldInvalid},
		{"definition without mode", func(task map[string]any) { delete(task, "oracle_mode") }, DetailFieldInvalid},
		{"white_box with definition", func(task map[string]any) { task["oracle_mode"] = OracleModeWhiteBox }, DetailFieldInvalid},
		{"surface task with mode", func(task map[string]any) {
			task["kind"] = KindSurface
			delete(task, "corpus_ref")
			delete(task, "expected_tests")
			delete(task, "black_box_oracle")
			task["oracle_mode"] = OracleModeWhiteBox
		}, DetailFieldInvalid},
		{"unknown definition field", func(task map[string]any) { oracleOf(task)["env"] = []any{} }, DetailUnknownField},
		{"build not a ./ package", func(task map[string]any) { oracleOf(task)["build"] = "cmd/tool" }, DetailFieldInvalid},
		{"build leaves the tree", func(task map[string]any) { oracleOf(task)["build"] = "./cmd/../../x" }, DetailFieldInvalid},
		{"command without artifact", func(task map[string]any) { oracleOf(task)["command"] = []any{"go", "run"} }, DetailFieldInvalid},
		{"artifact named twice", func(task map[string]any) {
			oracleOf(task)["command"] = []any{"{artifact}", "--self={artifact}"}
		}, DetailFieldInvalid},
		{"inputs absent", func(task map[string]any) { delete(oracleOf(task), "inputs") }, DetailFieldInvalid},
		{"input outside the oracle root", func(task map[string]any) {
			oracleOf(task)["inputs"] = []any{map[string]any{"path": "pkg/x/rows.json", "sha256": sha256Of("x")}}
		}, DetailUncleanPath},
		{"input digest upper case", func(task map[string]any) {
			oracleOf(task)["inputs"] = []any{map[string]any{"path": oracleFixture("rows.json"), "sha256": strings.Repeat("A", 64)}}
		}, DetailFieldInvalid},
		{"input file names repeat", func(task map[string]any) {
			oracleOf(task)["inputs"] = []any{pinned("rows.json", "a"), map[string]any{"path": OracleFixtureRoot + "/other/rows.json", "sha256": sha256Of("b")}}
		}, DetailFieldInvalid},
		{"stdin names no input", func(task map[string]any) { oracleOf(task)["stdin"] = "missing.json" }, DetailFieldInvalid},
		{"no assertion", func(task map[string]any) { oracleOf(task)["assertions"] = []any{} }, DetailFieldInvalid},
		{"too many assertions", func(task map[string]any) {
			items := []any{}
			for index := 0; index <= maxBlackBoxAssertions; index++ {
				items = append(items, map[string]any{"id": "exit" + strings.Repeat("x", index), "kind": BlackBoxExitCode, "exit_code": 0})
			}
			oracleOf(task)["assertions"] = items
		}, DetailFieldInvalid},
		{"assertion id malformed", func(task map[string]any) { assertionOf(task, 0)["id"] = "-exit" }, DetailFieldInvalid},
		{"assertion id repeats", func(task map[string]any) { assertionOf(task, 1)["id"] = "exit" }, DetailFieldInvalid},
		{"exit_code with expected output", func(task map[string]any) {
			assertionOf(task, 0)["expected"] = pinned("stdout.txt", fixtureOracleStdout)
		}, DetailFieldInvalid},
		{"exit_code not an integer", func(task map[string]any) { assertionOf(task, 0)["exit_code"] = true }, DetailFieldInvalid},
		{"stdout without expected output", func(task map[string]any) { delete(assertionOf(task, 1), "expected") }, DetailFieldInvalid},
		{"file path climbs out", func(task map[string]any) { assertionOf(task, 2)["path"] = "out/../../report.json" }, DetailFieldInvalid},
		{"unknown assertion kind", func(task map[string]any) { assertionOf(task, 1)["kind"] = "stderr" }, DetailFieldInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := blackBoxTask("GT-AG-001")
			tc.mutate(task)

			_, err := DecodeTask([]byte(mustJSON(t, task)))

			requireInvalid(t, err, tc.detail)
		})
	}
}

// TestExpectationDigest_BlackBoxOracle: a white-box task keeps its
// SPEC-HARNEVAL-001 digest; a black-box task digests its mode and
// definition, so a re-pinned expected output or a mode change moves it.
func TestExpectationDigest_BlackBoxOracle(t *testing.T) {
	t.Parallel()
	white := agentTask("GT-AG-001")
	white["oracle_mode"] = OracleModeWhiteBox
	assert.Equal(t, oracleAgentDigest, ExpectationDigest(decodeFixtureTask(t, white)))
	assert.Equal(t, oracleBlackBoxDigest, ExpectationDigest(decodeFixtureTask(t, blackBoxTask("GT-AG-001"))))

	repinned := blackBoxTask("GT-AG-001")
	assertionOf(repinned, 1)["expected"] = pinned("stdout.txt", fixtureOracleStdout+"!")
	assert.NotEqual(t, oracleBlackBoxDigest, ExpectationDigest(decodeFixtureTask(t, repinned)))
	reworded := blackBoxTask("GT-AG-001")
	reworded["intent"] = "another intent"
	assert.Equal(t, oracleBlackBoxDigest, ExpectationDigest(decodeFixtureTask(t, reworded)))
}

// TestLoadSet_BlackBoxFixtures_PinBytesAndSize: the loader reads every pinned
// fixture like a corpus file and refuses drift, links, and oversized outputs.
func TestLoadSet_BlackBoxFixtures_PinBytesAndSize(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeBlackBox(blackBoxTask("GT-AG-001"))
	set, err := LoadSet(f.root)
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"GT-AG-001": {"exit", "stdout", "report"}}, OracleAssertionIDs(set))

	for name, tc := range map[string]struct {
		mutate func(f *fixture)
		detail string
	}{
		"expected output drifted": {func(f *fixture) { f.write(oracleFixture("stdout.txt"), "three rows\n") }, DetailOracleDigestMismatch},
		"input missing": {func(f *fixture) {
			require.NoError(f.t, os.Remove(filepath.Join(f.root, filepath.FromSlash(oracleFixture("rows.json")))))
		}, DetailReadFailed},
		"expected output is a symlink": {func(f *fixture) {
			full := filepath.Join(f.root, filepath.FromSlash(oracleFixture("report.json")))
			require.NoError(f.t, os.Remove(full))
			require.NoError(f.t, os.Symlink(filepath.Join(f.root, filepath.FromSlash(oracleFixture("rows.json"))), full))
		}, DetailSymlinkNotAllowed},
		"expected output over 1 MiB": {func(f *fixture) {
			big := strings.Repeat("x", maxOracleOutputBytes+1)
			task := blackBoxTask("GT-AG-001")
			assertionOf(task, 1)["expected"] = pinned("stdout.txt", big)
			f.writeJSON(agentPath("GT-AG-001"), task)
			f.write(oracleFixture("stdout.txt"), big)
		}, DetailOracleDigestMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.standard()
			f.writeBlackBox(blackBoxTask("GT-AG-001"))
			tc.mutate(f)

			_, err := LoadSet(f.root)

			requireInvalid(t, err, tc.detail)
		})
	}
}

// TestOracleAssertionIDs_TakesOnlyActiveBlackBoxTasks: white-box and retired
// tasks give the signer no trusted assertion ids, so it schedules neither.
func TestOracleAssertionIDs_TakesOnlyActiveBlackBoxTasks(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeBlackBox(blackBoxTask("GT-AG-001"))
	retired := blackBoxTask("GT-AG-002")
	retired["status"] = map[string]any{"state": StateRetired, "reason": "superseded"}
	f.writeJSON(agentPath("GT-AG-002"), retired)
	f.writeJSON(agentPath("GT-AG-003"), agentTask("GT-AG-003"))
	set, err := LoadSet(f.root)
	require.NoError(t, err)

	assert.Equal(t, map[string][]string{"GT-AG-001": {"exit", "stdout", "report"}}, OracleAssertionIDs(set))
}
