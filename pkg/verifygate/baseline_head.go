package verifygate

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// testKey splits a failed-test key "pkg.TestX/sub" into the package and the
// top-level function. The function is the first ".Test", ".Example",
// ".Fuzz" or ".Benchmark" element, so a dotted package path (gopkg.in/x.v3)
// stays whole. A bare package (a build failure, TestMain) does not match.
var testKey = regexp.MustCompile(`^(.+?)\.((?:Test|Example|Fuzz|Benchmark)[^/]*)(?:/.*)?$`)

func splitTestKey(key string) (pkg, top string, ok bool) {
	m := testKey.FindStringSubmatch(key)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// headCount is how often HEAD runs each failed test. A failure is only
// pre-existing when HEAD fails it every time: a test that is flaky on HEAD
// would otherwise vouch for a deterministic break.
const headCount = 2

// headResult is what the failed tests did on HEAD. runs holds each key's
// runs there, paths normalized; brokenPkgs the packages that failed there
// with no test failed (they do not build, or TestMain failed), so their
// tests could not be compared; partial marks a run cut off before every
// test ran.
type headResult struct {
	runs        map[string][]testRun
	brokenPkgs  map[string]bool
	partial     bool
	unavailable string
	interrupted bool
	// decided marks keys answered from the cache: complete, whatever
	// partial says about the run that filled the rest.
	decided map[string]bool
}

// headFailures runs the top-level tests of the failed keys, and the
// packages of the package-level ones, on a HEAD snapshot of root.
func headFailures(ctx context.Context, root string, keys []string, budget time.Duration) headResult {
	module := goModule(root)
	if module == "" {
		return headResult{unavailable: "go.mod declares no module"}
	}
	var names, dirs []string
	for _, key := range keys {
		pkg, top, ok := splitTestKey(key)
		if !ok {
			pkg = key
		}
		dir := packageDir(module, pkg)
		if dir == "" {
			return headResult{unavailable: pkg + " is not a package of " + module}
		}
		if ok && !slices.Contains(names, top) {
			names = append(names, top)
		}
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	slices.Sort(names)
	slices.Sort(dirs)
	// Package-level failures alone still build the package and run its
	// TestMain, which is what they are compared on.
	pattern := "^$"
	if len(names) > 0 {
		pattern = "^(" + strings.Join(names, "|") + ")$"
	}
	if len(pattern) > maxRunPattern {
		return headResult{unavailable: "the failed test names do not fit one -run pattern"}
	}
	scratch, err := os.MkdirTemp("", "autopus-baseline-")
	if err != nil {
		return headResult{unavailable: err.Error()}
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := exportHead(ctx, root, scratch); err != nil {
		return headResult{unavailable: "exporting HEAD: " + err.Error()}
	}
	step := Step{Name: "go test (on HEAD)", Argv: append([]string{"go", "test", "-json", "-count", strconv.Itoa(headCount), "-run", pattern}, dirs...),
		Dir: scratch, GoTestJSON: true}
	failure, skipped := Run(ctx, []Step{step}, budget)
	switch {
	case slices.ContainsFunc(skipped, func(s Skip) bool { return s.Interrupted }):
		return headResult{interrupted: true}
	case len(skipped) > 0:
		return headResult{unavailable: skipped[0].Reason}
	case failure == nil:
		return headResult{runs: map[string][]testRun{}}
	case failure.TimedOut:
		return headResult{unavailable: "the run on HEAD " + failure.Reason}
	case len(failure.FailedTests) == 0:
		return headResult{unavailable: "go test on HEAD failed without a test result: " + lastLine(failure.Output)}
	}
	res := headResult{runs: map[string][]testRun{}, brokenPkgs: map[string]bool{}, partial: failure.Partial}
	for key, runs := range failure.Runs {
		res.runs[key] = normalized(runs, scratch)
	}
	for _, key := range failure.FailedTests {
		if _, _, ok := splitTestKey(key); !ok {
			res.brokenPkgs[key] = true
		}
	}
	return res
}

// judge returns the keys that are the change's failures: HEAD did not fail
// them every time with the same failure lines (current holds this tree's
// runs, paths normalized). A package-level failure needs one matching
// package failure on HEAD. A key HEAD could not answer for (its package is
// broken there, or the run was cut off before it) makes the comparison
// unavailable.
func (h headResult) judge(keys []string, current map[string][]testRun) (charged []string, unavailable string) {
	for _, key := range keys {
		_, _, isTest := splitTestKey(key)
		want := 1
		if isTest {
			want = headCount
		}
		if sameFailure(current[key], h.runs[key], want) {
			continue
		}
		if h.decided[key] {
			charged = append(charged, key)
			continue
		}
		if pkg, _, ok := splitTestKey(key); ok && h.brokenPkgs[pkg] {
			return nil, pkg + " does not build or run its tests on HEAD"
		}
		if h.partial && len(h.runs[key]) < want {
			return nil, "the run on HEAD was cut off before " + key + " finished"
		}
		charged = append(charged, key)
	}
	return charged, ""
}

// goModule is the module go.mod at root declares; "" when it declares none.
func goModule(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"`")
		}
	}
	return ""
}

// packageDir turns an import path under module into its ./dir pattern.
func packageDir(module, pkg string) string {
	switch {
	case pkg == module:
		return "."
	case strings.HasPrefix(pkg, module+"/"):
		return "./" + strings.TrimPrefix(pkg, module+"/")
	}
	return ""
}
