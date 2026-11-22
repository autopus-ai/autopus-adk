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

// packageState is one package of a cut-off run. Last is the time of its
// latest test event (run, cont, pause, pass, fail, skip), or of its start
// event when it has none; Open lists its tests that started and did not end.
type packageState struct {
	Name     string
	Started  bool
	Done     bool
	Finished int
	Last     time.Time
	Open     []string
	output   strings.Builder
}

// testProgress is what a cut-off `go test -json` run shows about itself.
type testProgress struct {
	Finished int
	Running  []runningTest
	Packages map[string]*packageState
}

type testState struct {
	started, running, done bool
	pkg                    string
	activity               time.Time
	output                 strings.Builder
}

type goTestEventTimed struct {
	Time                          time.Time
	Action, Package, Test, Output string
}

// goTestProgress reads `go test -json` output at kill time: how many tests
// finished, per package too, and which were still running. Only a test that
// is running (not paused waiting for a parallel slot) and has no unfinished
// subtest is listed as running.
func goTestProgress(out []byte, now time.Time) testProgress {
	prog := testProgress{Packages: map[string]*packageState{}}
	tests := map[string]*testState{}
	var order []string
	get := func(pkg, key string) *testState {
		s := tests[key]
		if s == nil {
			s = &testState{pkg: pkg}
			tests[key] = s
			order = append(order, key)
		}
		return s
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev goTestEventTimed
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil || ev.Package == "" {
			continue
		}
		pkg := prog.Packages[ev.Package]
		if pkg == nil {
			pkg = &packageState{Name: ev.Package}
			prog.Packages[ev.Package] = pkg
		}
		if ev.Test == "" {
			switch ev.Action {
			case "start":
				pkg.Started = true
				if ev.Time.After(pkg.Last) {
					pkg.Last = ev.Time
				}
			case "pass", "fail", "skip":
				pkg.Done = true
			}
			continue
		}
		key := ev.Package + "." + ev.Test
		s := get(ev.Package, key)
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
			pkg.Finished++
		default:
			continue
		}
		pkg.Started = true
		if ev.Time.After(pkg.Last) {
			pkg.Last = ev.Time
		}
		// An event in a subtest is activity of every test above it.
		for k := key; ; {
			if a := get(ev.Package, k); ev.Time.After(a.activity) {
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
		if p := prog.Packages[s.pkg]; p != nil && s.started && !s.done {
			p.Open = append(p.Open, key)
			p.output.WriteString(s.output.String())
		}
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

// stalled returns the packages that started and have not ended, whose last
// event is at least limit old: a test or TestMain stuck on something (a
// deadlock, a wait that never ends) stops the whole package's events. A
// package whose tests keep starting and ending is never stalled, however
// long it runs.
func (p testProgress) stalled(now time.Time, limit time.Duration) []*packageState {
	var out []*packageState
	for _, pkg := range p.Packages {
		if pkg.Started && !pkg.Done && !pkg.Last.IsZero() && now.Sub(pkg.Last) >= limit {
			out = append(out, pkg)
		}
	}
	slices.SortFunc(out, func(a, b *packageState) int { return strings.Compare(a.Name, b.Name) })
	return out
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
	for _, name := range pkgs {
		if pkg := p.Packages[name]; pkg == nil || (pkg.Finished == 0 && !pkg.Done) {
			out = append(out, name)
		}
	}
	return out
}

// describeStalled names each stalled package, how long it went silent, and
// the tests still open in it.
func describeStalled(pkgs []*packageState, now time.Time) string {
	parts := make([]string, 0, len(pkgs))
	for _, pkg := range pkgs {
		part := fmt.Sprintf("package %s (no test event for %s", pkg.Name, now.Sub(pkg.Last).Round(100*time.Millisecond))
		switch {
		case len(pkg.Open) > 5:
			part += "; still open: " + strings.Join(pkg.Open[:5], ", ") + fmt.Sprintf(" and %d more", len(pkg.Open)-5)
		case len(pkg.Open) > 0:
			part += "; still open: " + strings.Join(pkg.Open, ", ")
		case pkg.Finished > 0:
			part += "; no test open: the package is waiting after its tests (TestMain teardown)"
		default:
			part += "; no test started: TestMain or package init is waiting"
		}
		parts = append(parts, part+")")
	}
	return strings.Join(parts, ", ")
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
