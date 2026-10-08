package cli

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Steps of ci.yaml that check the Windows write refusals of
// pkg/harneval/intake (SPEC-HARNEVAL-002 REQ-HC-11, T13).
const (
	intakeVetStepName     = "Vet Windows intake tests"
	intakeWindowsStepName = "Test Windows intake write refusals"
	intakeWindowsFilter   = "PlatformUnsupported"
)

var intakeWindowsFloor = regexp.MustCompile(`(?m)^minimum=([0-9]+)$`)

// windowsIntakeTests returns the Test functions of pkg/harneval/intake that a
// Windows build compiles and the step's filter selects, the set `go test
// -list` reports on the windows-runtime runner.
func windowsIntakeTests(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(moduleRootForTest(t), "pkg", "harneval", "intake")
	windows := build.Default
	windows.GOOS, windows.GOARCH, windows.CgoEnabled = "windows", "amd64", false
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	files := token.NewFileSet()
	var names []string
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		match, err := windows.MatchFile(dir, entry.Name())
		require.NoError(t, err)
		if !match {
			continue
		}
		file, err := parser.ParseFile(files, filepath.Join(dir, entry.Name()), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && strings.Contains(fn.Name.Name, intakeWindowsFilter) {
				names = append(names, fn.Name.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// TestEvalHarnessIntakeWorkflow_S10_WindowsRefusalsAreVettedAndRun is the
// static contract of T13: the ubuntu test job vets the //go:build windows
// tests, and the windows-runtime job lists them, holds a floor no higher than
// the tests a Windows build has, runs them verbose, and compares the PASS set
// with the list. Neither step adds an action or sits in omp-native-smoke.
func TestEvalHarnessIntakeWorkflow_S10_WindowsRefusalsAreVettedAndRun(t *testing.T) {
	t.Parallel()
	raw, workflow := readHarnessWorkflow(t)

	test := workflow.Jobs["test"]
	assert.Equal(t, "ubuntu-latest", test.RunsOn)
	vet := harnessWorkflowStepNamed(t, test, intakeVetStepName)
	assert.Equal(t, "GOOS=windows GOARCH=amd64 go vet ./pkg/harneval/intake/", strings.TrimSpace(vet.Run))
	assert.Empty(t, vet.Uses)

	windows := workflow.Jobs["windows-runtime"]
	assert.Equal(t, "windows-latest", windows.RunsOn)
	step := harnessWorkflowStepNamed(t, windows, intakeWindowsStepName)
	assert.Equal(t, "bash", step.Shell)
	assert.Empty(t, step.Uses)
	for _, fragment := range []string{
		"set -euo pipefail",
		"filter='" + intakeWindowsFilter + "'",
		`go test -list "$filter" ./pkg/harneval/intake`,
		`if [[ "${#listed_tests[@]}" -lt "$minimum" ]]; then`,
		`go test -count=1 -timeout=5m -v ./pkg/harneval/intake -run "$filter" 2>&1 | tee "$log"`,
		`sed -n 's/^--- PASS: \([^ ]*\).*/\1/p' "$log"`,
		`if [[ "$observed" != "$listed" ]]; then`,
	} {
		assert.Contains(t, step.Run, fragment)
	}
	floor := intakeWindowsFloor.FindStringSubmatch(step.Run)
	require.Len(t, floor, 2, "the step declares no minimum")
	minimum, err := strconv.Atoi(floor[1])
	require.NoError(t, err)
	tests := windowsIntakeTests(t)
	assert.GreaterOrEqual(t, minimum, 1)
	assert.LessOrEqual(t, minimum, len(tests), "the floor exceeds the Windows tests %v", tests)

	assert.NotContains(t, raw, "version: latest")
	native := strings.Index(raw, "\n  omp-native-smoke:\n")
	next := strings.Index(raw, "\n  macos-runtime:\n")
	require.True(t, native >= 0 && next > native, "cannot isolate omp-native-smoke")
	assert.NotContains(t, raw[native:next], "pkg/harneval/intake")
}

// intakeFakeGo stands in for go in the windows-runtime step: -list prints the
// FAKE_LISTED names and the package line, and the verbose run prints
// FAKE_RUN_FILE and exits FAKE_RUN_EXIT.
const intakeFakeGo = `#!/bin/sh
echo "go $*" >> "$FAKE_LOG"
case " $* " in
  *" -list "*)
    for name in $FAKE_LISTED; do echo "$name"; done
    echo "ok  	github.com/insajin/autopus-adk/pkg/harneval/intake	0.1s"
    ;;
  *)
    cat "$FAKE_RUN_FILE"
    exit "$FAKE_RUN_EXIT"
    ;;
esac
`

// runIntakeWindowsStep runs script the way `shell: bash` does, with the fake
// go and the system directories alone on PATH.
func runIntakeWindowsStep(t *testing.T, script string, listed []string, runOutput string, runExit int) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte(intakeFakeGo), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step.sh"), []byte(script), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.out"), []byte(runOutput), 0o644))
	log := filepath.Join(dir, "calls.log")
	cmd := exec.Command("/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", filepath.Join(dir, "step.sh"))
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir, "FAKE_LOG=" + log,
		"FAKE_LISTED=" + strings.Join(listed, " "), "FAKE_RUN_FILE=" + filepath.Join(dir, "run.out"),
		"FAKE_RUN_EXIT=" + strconv.Itoa(runExit)}
	output, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else {
		require.NoError(t, err, string(output))
	}
	calls, _ := os.ReadFile(log)
	return code, string(output), string(calls)
}

// TestEvalHarnessIntakeWorkflow_S10_WindowsStepScript_FailsUnlessEveryListedTestPasses
// runs the committed windows-runtime step against a fake go. The gate passes
// only when at least the declared floor is listed and every listed test
// reports a top-level PASS; a shrunken list never reaches the run.
func TestEvalHarnessIntakeWorkflow_S10_WindowsStepScript_FailsUnlessEveryListedTestPasses(t *testing.T) {
	t.Parallel()
	_, workflow := readHarnessWorkflow(t)
	script := harnessWorkflowStepNamed(t, workflow.Jobs["windows-runtime"], intakeWindowsStepName).Run
	floor := intakeWindowsFloor.FindStringSubmatch(script)
	require.Len(t, floor, 2)
	minimum, err := strconv.Atoi(floor[1])
	require.NoError(t, err)
	var names []string
	var passed strings.Builder
	for index := range minimum {
		name := fmt.Sprintf("TestRefusal%d_OnWindows_PlatformUnsupported", index)
		names = append(names, name)
		fmt.Fprintf(&passed, "=== RUN   %s\n--- PASS: %s (0.00s)\n", name, name)
	}
	last := names[len(names)-1]
	allButLast := strings.Replace(passed.String(), "--- PASS: "+last, "--- SKIP: "+last, 1)
	cases := []struct {
		name, runOutput, message string
		listed                   []string
		runExit, code            int
		runs                     bool
	}{
		{name: "every listed test passes", listed: names, runOutput: passed.String() + "    --- PASS: " + last + "/sub (0.00s)\nPASS\n", runs: true},
		{name: "the list falls below the floor", listed: names[:minimum-1], code: 1, message: "below the declared"},
		{name: "a listed test skips", listed: names, runOutput: allButLast, code: 1, message: "reported PASS", runs: true},
		{name: "a listed test fails", listed: names, runOutput: strings.Replace(allButLast, "--- SKIP", "--- FAIL", 1) + "FAIL\n",
			runExit: 1, code: 1, runs: true},
		{name: "no test runs", listed: names, runOutput: "testing: warning: no tests to run\nPASS\n", code: 1, message: "reported PASS", runs: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code, output, calls := runIntakeWindowsStep(t, script, tc.listed, tc.runOutput, tc.runExit)

			assert.Equal(t, tc.code, code, output)
			assert.Contains(t, output, tc.message)
			assert.Contains(t, calls, "go test -list "+intakeWindowsFilter+" ./pkg/harneval/intake\n")
			run := "go test -count=1 -timeout=5m -v ./pkg/harneval/intake -run " + intakeWindowsFilter + "\n"
			assert.Equal(t, tc.runs, strings.Contains(calls, run), calls)
		})
	}
}
