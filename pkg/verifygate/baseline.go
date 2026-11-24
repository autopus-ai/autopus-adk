package verifygate

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// maxBaselineTests caps the failed tests one stop checks against HEAD. More
// failures than this are not a few broken tests the tree already had.
const maxBaselineTests = 50

// minBaselineBudget is the least time worth giving the run on HEAD.
const minBaselineBudget = 30 * time.Second

// ChargedRun is the outcome of RunCharged. Failure is nil when every failed
// Go test also fails on HEAD. Preexisting lists those tests ("pkg.Test");
// Edited the failed tests this change edited, which are its failures
// whatever HEAD does; Unavailable says why the failures could not be
// checked against HEAD, so they stay charged to the change.
type ChargedRun struct {
	Failure     *Failure
	Skipped     []Skip
	Preexisting []string
	Edited      []string
	Unavailable string
}

// RunCharged is Run that charges a change only with the Go test failures it
// caused. When a go test -json step fails with failed tests, those tests
// are run again on a HEAD snapshot (the export the test-first check uses);
// the ones that fail there too were broken before the change (an
// environment without a tool or network, a flaky suite) and do not fail the
// run. Then the steps after the failing one run as usual. A failure that
// passes on HEAD, a new or edited test, a package-level failure HEAD does
// not share, or a baseline that could not be run (no HEAD, no time left, a
// HEAD that does not build) is the change's failure, as with Run.
func RunCharged(ctx context.Context, root string, changed []string, steps []Step, budget time.Duration) ChargedRun {
	started := time.Now()
	var res ChargedRun
	known := map[string]bool{}
	var edited map[string]bool
	cache := loadBaselineCache(ctx, root)
	rest, left := withKnownFailing(steps, cache.knownFailing()), budget
	for {
		failure, skipped := Run(ctx, rest, left)
		res.Skipped = append(res.Skipped, skipped...)
		if failure == nil || !failure.Step.GoTestJSON || failure.Hung || failure.TimedOut || len(failure.FailedTests) == 0 {
			res.Failure = failure
			return res
		}
		var fresh []string
		for _, key := range failure.FailedTests {
			if !known[key] {
				fresh = append(fresh, key)
			}
		}
		if edited == nil {
			edited = editedTestFuncs(ctx, root, changed)
		}
		for _, key := range fresh {
			if isEditedTest(root, key, edited) {
				res.Edited = append(res.Edited, key)
			}
		}
		if len(res.Edited) > 0 {
			res.Failure = failure
			return res
		}
		idx := slices.IndexFunc(rest, func(s Step) bool { return sameStep(s, failure.Step) })
		left = budget - time.Since(started)
		if len(fresh) > 0 {
			window := left
			if idx >= 0 && idx < len(rest)-1 {
				// The steps after the failing one still need time.
				window = left / 2
			}
			var why string
			switch {
			case len(fresh) > maxBaselineTests:
				why = fmt.Sprintf("%d failed tests is more than the %d checked", len(fresh), maxBaselineTests)
			case window < minBaselineBudget:
				why = "no time was left in the " + budget.String() + " budget to run them"
			}
			var head headResult
			if why == "" {
				head = cache.headFailures(ctx, root, fresh, window)
				if head.interrupted {
					return ChargedRun{Skipped: append(res.Skipped, interruptedSkips(rest)...)}
				}
				why = head.unavailable
				head.runs = movedRuns(head.runs, fileMoves(ctx, root))
			}
			if why == "" {
				var charged []string
				charged, why = head.judge(fresh, currentRuns(failure, root))
				if len(charged) > 0 && why == "" {
					res.Failure = failure
					res.Preexisting = append(res.Preexisting, without(fresh, charged)...)
					return res
				}
			}
			if why != "" {
				res.Failure, res.Unavailable = failure, why
				return res
			}
			for _, key := range fresh {
				known[key] = true
			}
			res.Preexisting = append(res.Preexisting, fresh...)
		}
		if failure.Partial && !failure.Step.CoveredLater {
			res.Skipped = append(res.Skipped, partialSkip(failure))
		}
		if idx < 0 || idx == len(rest)-1 {
			return res
		}
		rest, left = rest[idx+1:], budget-time.Since(started)
	}
}

// Note describes how the run's failures were charged, for the hook's
// message; "" when nothing was checked against HEAD.
func (r ChargedRun) Note() string {
	var parts []string
	if len(r.Edited) > 0 {
		parts = append(parts, fmt.Sprintf("the failing test(s) %s were edited by this change (the test or its package's test code), so they count as its failures "+
			"even if they failed before it", listKeys(r.Edited)))
	}
	if len(r.Preexisting) > 0 {
		parts = append(parts, fmt.Sprintf("%d failing test(s) also fail on HEAD (before this change) and were not charged to it: %s",
			len(r.Preexisting), listKeys(r.Preexisting)))
	}
	if r.Unavailable != "" {
		parts = append(parts, "the failing tests could not be checked against HEAD ("+r.Unavailable+
			"), so they are charged to this change")
	}
	if len(parts) == 0 {
		return ""
	}
	return "auto verify stop: " + strings.Join(parts, "; ")
}

func listKeys(keys []string) string {
	const shown = 10
	if len(keys) > shown {
		return strings.Join(keys[:shown], ", ") + fmt.Sprintf(" and %d more", len(keys)-shown)
	}
	return strings.Join(keys, ", ")
}

// currentRuns is the failing step's runs per key, paths normalized.
func currentRuns(f *Failure, root string) map[string][]testRun {
	out := make(map[string][]testRun, len(f.Runs))
	for key, runs := range f.Runs {
		out[key] = normalized(runs, root)
	}
	return out
}

// partialSkip is the budget skip of a step cut off with only failures HEAD
// shares. Cut early for the HEAD check, the rest of its tests are still owed
// (Baseline); cut at its window's end, it is the size of the suite.
func partialSkip(f *Failure) Skip {
	if f.EarlyCut {
		return Skip{Name: f.Step.Name, Budget: true, Baseline: true, Reason: fmt.Sprintf(
			"stopped early to check its %d failed test(s) against HEAD, where they fail too; the tests after them run at the next stop",
			len(f.FailedTests))}
	}
	return Skip{Name: f.Step.Name, Budget: true, Reason: fmt.Sprintf(
		"did not finish within the window it had; its only failures were %d test(s) that fail on HEAD too", len(f.FailedTests))}
}

func sameStep(a, b Step) bool {
	return a.Name == b.Name && a.Dir == b.Dir && slices.Equal(a.Argv, b.Argv)
}

func without(all, drop []string) []string {
	var out []string
	for _, k := range all {
		if !slices.Contains(drop, k) {
			out = append(out, k)
		}
	}
	return out
}
