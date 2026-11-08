// Package execcheck runs one exec test case: it builds the product if asked,
// runs it in a fresh directory holding only the case's files, and compares
// what a user would observe with the case's expectations.
package execcheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// DefaultTimeout bounds a case that declares none, for build and run each.
const DefaultTimeout = time.Minute

// Mismatch is one expectation that did not hold.
type Mismatch struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Received string `json:"received"`
}

// Result is what one run observed.
type Result struct {
	ExitCode   int        `json:"exit_code"`
	Stdout     string     `json:"stdout"`
	Stderr     string     `json:"stderr"`
	Mismatches []Mismatch `json:"mismatches"`
}

// Passed reports whether every expectation held.
func (r Result) Passed() bool { return len(r.Mismatches) == 0 }

// Run executes x for a project. An error means the case could not run at
// all (bad case, failed build, timeout), which is not a product verdict.
func Run(ctx context.Context, projectDir string, x testscenario.Exec) (Result, error) {
	if err := testscenario.ValidateExec(&x); err != nil {
		return Result{}, err
	}
	project, err := filepath.Abs(projectDir)
	if err != nil {
		return Result{}, err
	}
	timeout := DefaultTimeout
	if x.Timeout != "" {
		timeout, _ = time.ParseDuration(x.Timeout)
	}
	bin, err := os.MkdirTemp("", "autopus-qa-exec-bin-")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(bin) }()
	work, err := os.MkdirTemp("", "autopus-qa-exec-work-")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	expand := strings.NewReplacer(testscenario.PlaceholderBin, bin, testscenario.PlaceholderProject, project)

	if len(x.Build) > 0 {
		res, err := run(ctx, timeout, project, expandAll(expand, x.Build), "")
		if err != nil {
			return Result{}, fmt.Errorf("build %s: %w", strings.Join(x.Build, " "), err)
		}
		if res.ExitCode != 0 {
			return Result{}, fmt.Errorf("build %s exited %d: %s", strings.Join(x.Build, " "), res.ExitCode, tail(res.Stderr))
		}
	}
	for name, body := range x.Files {
		path := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Result{}, err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return Result{}, err
		}
	}
	argv := expandAll(expand, x.Argv)
	// A project path stays a project path even though the product runs in
	// the work directory.
	if !filepath.IsAbs(argv[0]) && strings.Contains(argv[0], "/") {
		argv[0] = filepath.Join(project, argv[0])
	}
	res, err := run(ctx, timeout, work, argv, x.Stdin)
	if err != nil {
		return Result{}, err
	}
	res.Mismatches = compare(x.Expect, res, work)
	return res, nil
}

func run(ctx context.Context, timeout time.Duration, dir string, argv []string, stdin string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return Result{}, fmt.Errorf("%s did not finish within %s", argv[0], timeout)
	}
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), Mismatches: []Mismatch{}}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return Result{}, fmt.Errorf("start %s: %w", argv[0], err)
	}
	return res, nil
}

func compare(want testscenario.ExecExpect, got Result, work string) []Mismatch {
	out := []Mismatch{}
	if want.ExitCode != nil && *want.ExitCode != got.ExitCode {
		out = append(out, Mismatch{"exit_code", fmt.Sprint(*want.ExitCode), fmt.Sprint(got.ExitCode)})
	}
	if want.Stdout != nil && *want.Stdout != got.Stdout {
		out = append(out, Mismatch{"stdout", *want.Stdout, got.Stdout})
	}
	for _, s := range want.StdoutContains {
		if !strings.Contains(got.Stdout, s) {
			out = append(out, Mismatch{"stdout_contains", s, got.Stdout})
		}
	}
	for _, s := range want.StdoutNotContains {
		if strings.Contains(got.Stdout, s) {
			out = append(out, Mismatch{"stdout_not_contains", "no " + strconv.Quote(s), got.Stdout})
		}
	}
	if len(want.StdoutLines) > 0 {
		if lines := nonEmptyLines(got.Stdout); !linesMatch(want.StdoutLines, lines) {
			out = append(out, Mismatch{"stdout_lines", strings.Join(want.StdoutLines, "\n"), got.Stdout})
		}
	}
	for _, s := range want.StderrContains {
		if !strings.Contains(got.Stderr, s) {
			out = append(out, Mismatch{"stderr_contains", s, got.Stderr})
		}
	}
	for name, body := range want.Files {
		data, err := os.ReadFile(filepath.Join(work, name))
		received := string(data)
		if err != nil {
			received = "(missing: " + err.Error() + ")"
		}
		if err != nil || received != body {
			out = append(out, Mismatch{"file " + name, body, received})
		}
	}
	return out
}

// WriteReport prints a failed result in the Expected:/Received: shape the
// QA triage reads as a product defect.
func WriteReport(w io.Writer, caseID, ac string, r Result) {
	for _, m := range r.Mismatches {
		fmt.Fprintf(w, "exec case %s (%s): %s did not hold\n", caseID, ac, m.Field)
		fmt.Fprintf(w, "  Expected: %q\n", m.Expected)
		fmt.Fprintf(w, "  Received: %q\n", m.Received)
	}
	if !r.Passed() && r.Stderr != "" {
		fmt.Fprintf(w, "stderr:\n%s\n", tail(r.Stderr))
	}
}

// linesMatch reports whether there is one line per entry and each line
// contains its entry.
func linesMatch(want, lines []string) bool {
	if len(want) != len(lines) {
		return false
	}
	for i, w := range want {
		if !strings.Contains(lines[i], w) {
			return false
		}
	}
	return true
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func expandAll(r *strings.Replacer, in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = r.Replace(s)
	}
	return out
}

func tail(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}
