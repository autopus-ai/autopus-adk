package execcheck

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
	"github.com/insajin/autopus-adk/pkg/qa/triage"
)

// A tiny wc-like product: it counts newline characters, as wc does, so a
// file whose last line has no newline counts one short. An intent that says
// "count the last line too" must catch that.
const mywc = `package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	total, failed := 0, false
	for _, name := range os.Args[2:] {
		data, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mywc: %s: no such file\n", name)
			failed = true
			continue
		}
		n := bytes.Count(data, []byte("\n"))
		total += n
		fmt.Printf("%d %s\n", n, name)
		os.WriteFile("last.txt", []byte(name), 0o644)
	}
	if len(os.Args) > 3 {
		fmt.Printf("%d total\n", total)
	}
	if failed {
		os.Exit(1)
	}
}
`

func newProduct(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module mywc\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(mywc), 0o644))
	return dir
}

func intp(n int) *int       { return &n }
func strp(s string) *string { return &s }

func build() []string { return []string{"go", "build", "-o", "$BIN/mywc", "."} }

func TestRun_ChecksWhatTheUserObserves(t *testing.T) {
	project := newProduct(t)
	ctx := context.Background()

	res, err := Run(ctx, project, testscenario.Exec{
		Build: build(), Argv: []string{"$BIN/mywc", "--lines", "a.txt", "b.txt"},
		Files: map[string]string{"a.txt": "1\n2\n3\n", "b.txt": "x\n"},
		Expect: testscenario.ExecExpect{
			ExitCode: intp(0), Stdout: strp("3 a.txt\n1 b.txt\n4 total\n"),
			Files: map[string]string{"last.txt": "b.txt"},
		},
	})
	require.NoError(t, err)
	assert.True(t, res.Passed(), "%+v", res.Mismatches)

	// The misread the intent was written to catch.
	res, err = Run(ctx, project, testscenario.Exec{
		Build: build(), Argv: []string{"$BIN/mywc", "--lines", "c.txt"},
		Files:  map[string]string{"c.txt": "one\ntwo\nthree"},
		Expect: testscenario.ExecExpect{Stdout: strp("3 c.txt\n")},
	})
	require.NoError(t, err)
	require.Len(t, res.Mismatches, 1)
	assert.Equal(t, Mismatch{"stdout", "3 c.txt\n", "2 c.txt\n"}, res.Mismatches[0])

	res, err = Run(ctx, project, testscenario.Exec{
		Build: build(), Argv: []string{"$BIN/mywc", "--lines", "a.txt", "missing.txt"},
		Files: map[string]string{"a.txt": "1\n"},
		Expect: testscenario.ExecExpect{
			ExitCode: intp(0), StderrContains: []string{"missing.txt"}, StdoutContains: []string{"1 total"},
			Files: map[string]string{"nope.txt": "x"},
		},
	})
	require.NoError(t, err)
	fields := []string{}
	for _, m := range res.Mismatches {
		fields = append(fields, m.Field)
	}
	assert.ElementsMatch(t, []string{"exit_code", "file nope.txt"}, fields)

	var report bytes.Buffer
	WriteReport(&report, "missing-file", "AC-7", res)
	assert.Contains(t, report.String(), "exec case missing-file (AC-7): exit_code did not hold\n  Expected: \"0\"\n  Received: \"1\"\n")
}

func TestRun_CannotRunIsAnErrorNotAVerdict(t *testing.T) {
	project := newProduct(t)
	ctx := context.Background()
	_, err := Run(ctx, project, testscenario.Exec{
		Build: []string{"go", "build", "-o", "$BIN/mywc", "./nope"}, Argv: []string{"$BIN/mywc"},
		Expect: testscenario.ExecExpect{ExitCode: intp(0)},
	})
	assert.ErrorContains(t, err, "build")

	_, err = Run(ctx, project, testscenario.Exec{Argv: []string{"/bin/rm", "-rf", "x"}, Expect: testscenario.ExecExpect{ExitCode: intp(0)}})
	assert.ErrorContains(t, err, "argv[0]", "an arbitrary system binary is refused")

	_, err = Run(ctx, project, testscenario.Exec{Argv: []string{"$BIN/mywc"}})
	assert.ErrorContains(t, err, "at least one observable outcome")

	_, err = Run(ctx, project, testscenario.Exec{
		Argv: []string{"$BIN/mywc"}, Files: map[string]string{"../escape": "x"}, Expect: testscenario.ExecExpect{ExitCode: intp(0)},
	})
	assert.ErrorContains(t, err, "relative path")
}

func TestValidateExec_ProgramMustBeTheProduct(t *testing.T) {
	ok := testscenario.ExecExpect{ExitCode: intp(0)}
	for argv0, want := range map[string]bool{
		"$BIN/tool": true, "$PROJECT/bin/tool": true, "./bin/tool": true, "bin/tool": true, "python3": true,
		"/usr/bin/curl": false, "curl": false, "../outside/tool": false, "$BIN/../x": false, "$BIN/": false,
	} {
		err := testscenario.ValidateExec(&testscenario.Exec{Argv: []string{argv0}, Expect: ok})
		assert.Equal(t, want, err == nil, "%s: %v", argv0, err)
	}
}

// The report must read as a product defect to the QA loop's triage, or a
// failed expectation would stop the loop as unknown instead of being fixed.
func TestWriteReport_TriagesAsProductDefect(t *testing.T) {
	var report bytes.Buffer
	WriteReport(&report, "last-line-no-newline", "AC-6", Result{
		ExitCode: 0, Stdout: "2 c.txt\n",
		Mismatches: []Mismatch{{Field: "stdout", Expected: "3 c.txt\n", Received: "2 c.txt\n"}},
	})
	v := triage.Classify(triage.Input{
		JourneyID: "compiled-test-scenarios-intent-wc-last-line-no-newline-custom-command",
		Adapter:   "custom-command", Status: "failed", FailureText: report.String(), ProjectDir: t.TempDir(),
	})
	assert.Equal(t, triage.ClassProductDefect, v.Class, "%+v", v)
}
