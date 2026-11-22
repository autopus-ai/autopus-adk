package verifygate

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// suspect is one piece of evidence of a possible hang: a running test, or a
// silent package, with how long it went without progress.
type suspect struct {
	Label  string
	Idle   time.Duration
	Output string
}

// suspects joins the two analysers of a cut-off run: tests running for at
// least limit without progress (and with no unfinished subtest), and
// packages silent for at least limit. Neither is proof. At kill time the
// gate sees only a prefix of the event stream, so a slow test and a
// deadlocked one look the same, and a deadlock inside a busy package shows
// only at the test level. The evidence is ordered by idle time, longest
// first, so the likeliest culprit is named first.
func (p testProgress) suspects(now time.Time, limit time.Duration) []suspect {
	var out []suspect
	for _, r := range p.Running {
		if r.Idle >= limit {
			out = append(out, suspect{Label: fmt.Sprintf("%s (%s)", r.Name, r.Idle.Round(100*time.Millisecond)),
				Idle: r.Idle, Output: r.Output})
		}
	}
	for _, pkg := range p.stalled(now, limit) {
		out = append(out, suspect{Label: describeStalled([]*packageState{pkg}, now), Idle: now.Sub(pkg.Last),
			Output: pkg.output.String()})
	}
	slices.SortStableFunc(out, func(a, b suspect) int { return cmp.Compare(b.Idle, a.Idle) })
	return out
}

func describeSuspects(s []suspect) string {
	const shown = 5
	labels := make([]string, 0, shown+1)
	for i, x := range s {
		if i == shown {
			labels = append(labels, fmt.Sprintf("and %d more", len(s)-shown))
			break
		}
		labels = append(labels, x.Label)
	}
	return strings.Join(labels, ", ")
}

// onlyLatePackageFails reports whether the run's only failures are
// package-level fail events stamped after the step's deadline: go reporting
// the test binary the kill took down, not a test that failed.
func onlyLatePackageFails(out []byte, deadline time.Time) bool {
	late := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev goTestEventTimed
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch {
		case ev.Action == "build-fail", ev.Action == "fail" && ev.Test != "":
			return false
		case ev.Action == "fail" && ev.Package != "":
			if ev.Time.IsZero() || !ev.Time.After(deadline) {
				return false
			}
			late = true
		}
	}
	return late
}
