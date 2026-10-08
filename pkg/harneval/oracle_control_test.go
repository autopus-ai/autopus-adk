package harneval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Positive-control fixture bytes of GT-AG-001: a second input the same
// artifact must accept, and the stdout it must print for it.
const (
	fixtureControlInput  = `{"rows":[1]}` + "\n"
	fixtureControlStdout = "one row\n"
)

// controlTask is blackBoxTask(id) whose definition also pins a positive
// control: the same command over control/rows.json must exit 0 and print
// control/stdout.txt.
func controlTask(id string) map[string]any {
	task := blackBoxTask(id)
	oracleOf(task)["positive_control"] = map[string]any{
		"inputs": []any{pinned("control/rows.json", fixtureControlInput)},
		"stdout": pinned("control/stdout.txt", fixtureControlStdout),
	}
	return task
}

func controlOf(task map[string]any) map[string]any {
	return oracleOf(task)["positive_control"].(map[string]any)
}

func (f *fixture) writeControl(task map[string]any) {
	f.t.Helper()
	f.writeBlackBox(task)
	f.write(oracleFixture("control/rows.json"), fixtureControlInput)
	f.write(oracleFixture("control/stdout.txt"), fixtureControlStdout)
}

// TestDecodeTask_PositiveControl_AddsItsTwoReservedAssertions: a definition
// with a positive control decodes, and the trusted assertion ids end with the
// control's exit and stdout ids the oracle harness reports for it.
func TestDecodeTask_PositiveControl_AddsItsTwoReservedAssertions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.standard()
	f.writeControl(controlTask("GT-AG-001"))

	set, err := LoadSet(f.root)

	require.NoError(t, err)
	task := findTask(set, "GT-AG-001")
	require.NotNil(t, task)
	control := task.BlackBoxOracle.PositiveControl
	require.NotNil(t, control)
	assert.Equal(t, oracleFixture("control/stdout.txt"), control.Stdout.Path)
	assert.Equal(t, map[string][]string{"GT-AG-001": {"exit", "stdout", "report",
		"positive_control.exit", "positive_control.stdout"}}, OracleAssertionIDs(set))
}

func findTask(set *Set, id string) *Task {
	for index := range set.Tasks {
		if set.Tasks[index].ID == id {
			return &set.Tasks[index]
		}
	}
	return nil
}

// TestDecodeTask_PositiveControl_RejectsWithExactDetail: the control is a
// second input set and a pinned stdout, nothing else, and the task's own
// assertions may not take the control's reserved ids.
func TestDecodeTask_PositiveControl_RejectsWithExactDetail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(task map[string]any)
		detail string
	}{
		{"unknown control field", func(task map[string]any) { controlOf(task)["exit_code"] = 0 }, DetailUnknownField},
		{"no control stdout", func(task map[string]any) { delete(controlOf(task), "stdout") }, DetailFieldInvalid},
		{"no control input", func(task map[string]any) { controlOf(task)["inputs"] = []any{} }, DetailFieldInvalid},
		{"control inputs absent", func(task map[string]any) { delete(controlOf(task), "inputs") }, DetailFieldInvalid},
		{"control input names repeat", func(task map[string]any) {
			controlOf(task)["inputs"] = []any{pinned("control/rows.json", "a"), pinned("other/rows.json", "b")}
		}, DetailFieldInvalid},
		{"control stdin names no control input", func(task map[string]any) { controlOf(task)["stdin"] = "rows2.json" }, DetailFieldInvalid},
		{"control stdout outside the oracle root", func(task map[string]any) {
			controlOf(task)["stdout"] = map[string]any{"path": "pkg/x/stdout.txt", "sha256": sha256Of("x")}
		}, DetailUncleanPath},
		{"assertion takes a reserved id", func(task map[string]any) { assertionOf(task, 1)["id"] = "positive_control.stdout" }, DetailFieldInvalid},
		{"assertion in the reserved namespace", func(task map[string]any) { assertionOf(task, 0)["id"] = "positive_control.x" }, DetailFieldInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			task := controlTask("GT-AG-001")
			tc.mutate(task)

			_, err := DecodeTask([]byte(mustJSON(t, task)))

			requireInvalid(t, err, tc.detail)
		})
	}
	white := blackBoxTask("GT-AG-001")
	assertionOf(white, 0)["id"] = "positive_control.exit"
	_, err := DecodeTask([]byte(mustJSON(t, white)))
	requireInvalid(t, err, DetailFieldInvalid)
}

// TestExpectationDigest_PositiveControl: adding a control or re-pinning its
// stdout or input is an expectation change; a task without one keeps its
// digest.
func TestExpectationDigest_PositiveControl(t *testing.T) {
	t.Parallel()
	assert.Equal(t, oracleBlackBoxDigest, ExpectationDigest(decodeFixtureTask(t, blackBoxTask("GT-AG-001"))))
	with := ExpectationDigest(decodeFixtureTask(t, controlTask("GT-AG-001")))
	assert.NotEqual(t, oracleBlackBoxDigest, with)

	repinned := controlTask("GT-AG-001")
	controlOf(repinned)["stdout"] = pinned("control/stdout.txt", fixtureControlStdout+"!")
	assert.NotEqual(t, with, ExpectationDigest(decodeFixtureTask(t, repinned)))
	input := controlTask("GT-AG-001")
	controlOf(input)["inputs"] = []any{pinned("control/rows.json", `{"rows":[2]}`+"\n")}
	assert.NotEqual(t, with, ExpectationDigest(decodeFixtureTask(t, input)))
}

// TestLoadSet_PositiveControlFixtures_PinBytes: the loader checks the
// control's input and stdout bytes like every other oracle fixture.
func TestLoadSet_PositiveControlFixtures_PinBytes(t *testing.T) {
	t.Parallel()
	for name, file := range map[string]string{"control input drifted": "control/rows.json", "control stdout drifted": "control/stdout.txt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.standard()
			f.writeControl(controlTask("GT-AG-001"))
			f.write(oracleFixture(file), "drifted\n")

			_, err := LoadSet(f.root)

			requireInvalid(t, err, DetailOracleDigestMismatch)
		})
	}
}
