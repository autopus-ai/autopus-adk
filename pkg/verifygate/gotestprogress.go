package verifygate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// runningTest is a test that had started and not ended when a step was
// killed. Idle is how long it, and every subtest under it, went without an
// event that starts, resumes, pauses or ends a test; 0 when unknown.
type runningTest struct {
	Name   string
	Idle   time.Duration
	Output string
}

// testProgress is what a cut-off `go test -json` run shows about itself.
// Packages maps each package seen to its finished test count; Done lists
// the packages that ended (passed, failed, or had no tests).
type testProgress struct {
	Finished int
	Running  []runningTest
	Packages map[string]int
	Done     []string
}

type testState struct {
	started, running, done bool
	activity               time.Time
	output                 strings.Builder
}

// goTestProgress reads `go test -json` output at kill time: how many tests
// finished, per package too, and which were still running. Only a test that
// is running (not paused waiting for a parallel slot) and has no unfinished
// subtest is listed: a parent waiting on its subtests, running or queued,
// is not stuck itself.
func goTestProgress(out []byte, now time.Time) testProgress {
	prog := testProgress{Packages: map[string]int{}}
	tests := map[string]*testState{}
	var order []string
	get := func(key string) *testState {
		s := tests[key]
		if s == nil {
			s = &testState{}
			tests[key] = s
			order = append(order, key)
		}
		return s
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev struct {
			Time                          time.Time
			Action, Package, Test, Output string
		}
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil || ev.Package == "" {
			continue
		}
		if _, ok := prog.Packages[ev.Package]; !ok {
			prog.Packages[ev.Package] = 0
		}
		if ev.Test == "" {
			if slices.Contains([]string{"pass", "fail", "skip"}, ev.Action) && !slices.Contains(prog.Done, ev.Package) {
				prog.Done = append(prog.Done, ev.Package)
			}
			continue
		}
		key := ev.Package + "." + ev.Test
		s := get(key)
		switch ev.Action {
		case "output":
			s.output.WriteString(ev.Output)
			continue
		case "run", "cont":
			s.started, s.running = true, true
		case "pause":
			s.started, s.running = true, false
		case "pass", "fail", "skip":
			s.running, s.done = false, true
			prog.Finished++
			prog.Packages[ev.Package]++
		default:
			continue
		}
		// An event in a subtest is activity of every test above it.
		for k := key; ; {
			if a := get(k); ev.Time.After(a.activity) {
				a.activity = ev.Time
			}
			i := strings.LastIndex(k, "/")
			if i < 0 {
				break
			}
			k = k[:i]
		}
	}
	for _, key := range order {
		s := tests[key]
		if !s.running || slices.ContainsFunc(order, func(k string) bool {
			return strings.HasPrefix(k, key+"/") && tests[k].started && !tests[k].done
		}) {
			continue
		}
		rt := runningTest{Name: key, Output: s.output.String()}
		if !s.activity.IsZero() {
			rt.Idle = now.Sub(s.activity)
		}
		prog.Running = append(prog.Running, rt)
	}
	return prog
}

// unchecked returns the packages, of those seen and those the step was
// given, that ended no test and did not end themselves: they were still
// building, or waiting to start, when time ran out.
func (p testProgress) unchecked(given []string) []string {
	var out []string
	pkgs := append([]string{}, given...)
	for pkg := range p.Packages {
		if !slices.Contains(pkgs, pkg) {
			pkgs = append(pkgs, pkg)
		}
	}
	slices.Sort(pkgs)
	for _, pkg := range pkgs {
		if p.Packages[pkg] == 0 && !slices.Contains(p.Done, pkg) {
			out = append(out, pkg)
		}
	}
	return out
}

// hung returns the running tests idle for at least limit.
func (p testProgress) hung(limit time.Duration) []runningTest {
	var out []runningTest
	for _, r := range p.Running {
		if r.Idle >= limit {
			out = append(out, r)
		}
	}
	return out
}

// describeRunning names tests with how long each had gone without progress.
func describeRunning(tests []runningTest) string {
	const shown = 5
	parts := make([]string, 0, shown+1)
	for i, r := range tests {
		if i == shown {
			parts = append(parts, fmt.Sprintf("and %d more", len(tests)-shown))
			break
		}
		if r.Idle > 0 {
			parts = append(parts, fmt.Sprintf("%s (%s)", r.Name, r.Idle.Round(100*time.Millisecond)))
		} else {
			parts = append(parts, r.Name)
		}
	}
	return strings.Join(parts, ", ")
}
