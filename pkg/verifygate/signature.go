package verifygate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// testRun is one ended run of a test, or of a package's own events. Lines
// is its failure signature: the `file.go:N: message` lines it printed and
// its first `panic:` line, with the tree's path replaced by $ROOT.
type testRun struct {
	Failed bool
	Lines  []string
	// open marks a failure line that indented continuation lines extend;
	// skipping marks a continuation under a label that is left out.
	open, skipping bool
}

// continuationLabel is a testify field label: "Error:", "Error Trace:",
// "Messages:". Its unlabeled continuation lines belong to it.
var continuationLabel = regexp.MustCompile(`^([A-Z][A-Za-z ]*):(\s|$)`)

// continueLine appends an indented line that follows a failure line to it:
// testify prints only "file_test.go:10: " there and the values below it
// ("Error: Not equal:", "expected: 3", "actual  : -1"). The Error Trace and
// Test fields are left out (a path, the test's own name); a framing line
// ("=== RUN", "--- FAIL") or an unindented line ends the failure line.
func (r *testRun) continueLine(text string) {
	content := strings.TrimSpace(text)
	if !r.open || content == "" {
		return
	}
	if text == strings.TrimLeft(text, " \t") || strings.HasPrefix(content, "--- ") || strings.HasPrefix(content, "=== ") {
		r.open = false
		return
	}
	if m := continuationLabel.FindStringSubmatch(content); m != nil {
		r.skipping = m[1] == "Error Trace" || m[1] == "Test"
	}
	if r.skipping {
		return
	}
	r.Lines[len(r.Lines)-1] += " | " + strings.Join(strings.Fields(content), " ")
}

// failureLine matches what t.Error, t.Fatal and the compiler print:
// "file_test.go:12: message" or "./a.go:3:2: undefined: x".
var failureLine = regexp.MustCompile(`^\s*([\w./$-]+\.go:\d+(?::\d+)?: .*)$`)

// testRuns reads go test -json output into the ended runs of each test key
// ("pkg.Test", "pkg.Test/sub") and of each package (the bare import path).
// A test run with -count=2 ends twice, so it has two runs.
func testRuns(out []byte) map[string][]testRun {
	runs := map[string][]testRun{}
	pending := map[string]*testRun{}
	panicked := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev goTestEvent
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			continue
		}
		key := ev.Package
		if key == "" {
			key, _, _ = strings.Cut(ev.ImportPath, " ")
		}
		if key == "" {
			continue
		}
		if ev.Test != "" {
			key += "." + ev.Test
		}
		run := pending[key]
		if run == nil {
			run = &testRun{}
			pending[key] = run
		}
		switch ev.Action {
		case "output", "build-output":
			text := strings.TrimRight(ev.Output, "\n")
			if m := failureLine.FindStringSubmatch(text); m != nil {
				run.Lines = append(run.Lines, m[1])
				run.open, run.skipping = true, false
			} else if t := strings.TrimSpace(text); strings.HasPrefix(t, "panic: ") && !panicked[key] {
				panicked[key] = true
				run.Lines = append(run.Lines, t)
				run.open = false
			} else {
				run.continueLine(text)
			}
		case "fail", "build-fail":
			run.Failed = true
			fallthrough
		case "pass", "skip":
			runs[key] = append(runs[key], *run)
			delete(pending, key)
			delete(panicked, key)
		}
	}
	return runs
}

// normalized replaces the tree's own path (and its symlink-resolved form,
// /var against /private/var on macOS) with $ROOT, so a message that names
// a file in the work tree reads the same from the HEAD snapshot.
func normalized(runs []testRun, dir string) []testRun {
	prefixes := []string{dir}
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		// The longer path first, so /private/var is not left as /private$ROOT.
		prefixes = []string{real, dir}
	}
	out := make([]testRun, 0, len(runs))
	for _, r := range runs {
		lines := make([]string, 0, len(r.Lines))
		for _, l := range r.Lines {
			for _, p := range prefixes {
				l = strings.ReplaceAll(l, p, "$ROOT")
			}
			lines = append(lines, normalizeTemp(l))
		}
		out = append(out, testRun{Failed: r.Failed, Lines: lines})
	}
	return out
}

// sameFailure reports whether HEAD's runs show the current failure: every
// HEAD run failed, there are at least want of them (a test that passes once
// on HEAD is flaky there, and a flaky failure cannot vouch for a
// deterministic one), and each printed every failure line the current
// failed runs printed. A different assertion, a different table row or a
// different environment error is a different failure.
func sameFailure(current, head []testRun, want int) bool {
	if len(head) < want {
		return false
	}
	// A failure that printed nothing (t.FailNow, t.Fail) is only the same
	// as HEAD's when HEAD's printed nothing either: an empty signature is
	// contained in every other.
	silent := true
	for _, c := range current {
		if c.Failed && len(c.Lines) > 0 {
			silent = false
		}
	}
	if silent {
		for _, h := range head {
			if len(h.Lines) > 0 {
				return false
			}
		}
	}
	for _, h := range head {
		if !h.Failed {
			return false
		}
		have := map[string]bool{}
		for _, l := range h.Lines {
			have[l] = true
		}
		for _, c := range current {
			if !c.Failed {
				continue
			}
			for _, l := range c.Lines {
				if !have[l] {
					return false
				}
			}
		}
	}
	return true
}
