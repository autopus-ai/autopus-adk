package verifygate

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// RedVerdict is the outcome of the test-first check.
type RedVerdict int

const (
	// RedNotApplicable means no product source changed, or only sources the
	// check cannot pair with tests (Rust keeps its unit tests inline).
	RedNotApplicable RedVerdict = iota
	// RedProven means the changed tests fail against the code before the
	// change, so they test it.
	RedProven
	// RedNoTest means product source changed with no test change at all.
	RedNoTest
	// RedPassesBefore means the changed tests already pass against the code
	// before the change, so they do not test it.
	RedPassesBefore
	// RedUnavailable means the old tree could not be built or the tests could
	// not run there: the machine, not the change.
	RedUnavailable
)

// RedResult carries the verdict and what the message needs to explain it.
type RedResult struct {
	Verdict RedVerdict
	Sources []string
	Tests   []string
	Detail  string
}

// redLanguages are the source kinds whose tests live in their own files.
var redLanguages = append([]string{".go", ".py"}, nodeExts...)

// IsTest reports whether a changed path is a test or a test fixture.
func IsTest(file string) bool {
	base := path.Base(file)
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.Contains("/"+file, "/testdata/"),
		strings.Contains("/"+file, "/__tests__/"),
		strings.Contains(base, ".test."), strings.Contains(base, ".spec."),
		path.Ext(base) == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")):
		return true
	}
	return false
}

// CheckRed proves test-first work without asking the agent for it: the
// changed tests are laid over HEAD's sources in a scratch copy and run there.
// They must fail before the change; the normal gate already showed they pass
// after it.
func CheckRed(ctx context.Context, root string, changed []string, budget time.Duration) RedResult {
	var res RedResult
	for _, f := range changed {
		switch {
		case IsTest(f):
			res.Tests = append(res.Tests, f)
		case hasExt([]string{f}, redLanguages...):
			res.Sources = append(res.Sources, f)
		}
	}
	if len(res.Sources) == 0 {
		return res
	}
	if len(res.Tests) == 0 {
		res.Verdict = RedNoTest
		return res
	}
	scratch, err := os.MkdirTemp("", "autopus-red-")
	if err != nil {
		return RedResult{Verdict: RedUnavailable, Sources: res.Sources, Tests: res.Tests, Detail: err.Error()}
	}
	defer os.RemoveAll(scratch)
	if err := exportHead(ctx, root, scratch); err != nil {
		return RedResult{Verdict: RedUnavailable, Sources: res.Sources, Tests: res.Tests, Detail: "exporting HEAD: " + err.Error()}
	}
	for _, f := range res.Tests {
		if err := copyFile(filepath.Join(root, filepath.FromSlash(f)), filepath.Join(scratch, filepath.FromSlash(f))); err != nil {
			return RedResult{Verdict: RedUnavailable, Sources: res.Sources, Tests: res.Tests, Detail: err.Error()}
		}
	}
	linkNodeModules(root, scratch)
	steps := redSteps(scratch, res.Tests)
	if len(steps) == 0 {
		res.Verdict = RedNotApplicable
		return res
	}
	failure, skipped := Run(ctx, steps, budget)
	switch {
	case failure != nil:
		res.Verdict = RedProven
	case len(skipped) > 0:
		res.Verdict, res.Detail = RedUnavailable, skipped[0].Name+": "+skipped[0].Reason
	default:
		res.Verdict = RedPassesBefore
	}
	return res
}

// redSteps runs only the tests that changed: the Go packages that hold them,
// the node test script, or pytest on the changed files.
func redSteps(scratch string, tests []string) []Step {
	var steps []Step
	if pkgs := goPackages(scratch, tests); len(pkgs) > 0 {
		steps = append(steps, Step{Name: "go test (before the change)", Argv: append([]string{"go", "test"}, pkgs...), Dir: scratch})
	}
	if hasExt(tests, nodeExts...) {
		for _, step := range nodeSteps(scratch) {
			if strings.HasSuffix(step.Name, "test") {
				step.Name += " (before the change)"
				steps = append(steps, step)
			}
		}
	}
	var py []string
	for _, f := range tests {
		if path.Ext(f) == ".py" {
			py = append(py, filepath.FromSlash(f))
		}
	}
	if len(py) > 0 {
		steps = append(steps, Step{Name: "pytest (before the change)", Argv: append([]string{"python3", "-m", "pytest", "-q"}, py...), Dir: scratch})
	}
	return steps
}

// exportHead writes HEAD's tracked files into dir without touching the
// repository: no worktree entry, index, or ref is created.
func exportHead(ctx context.Context, root, dir string) error {
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar", "HEAD")
	archive.Dir = root
	extract := exec.CommandContext(ctx, "tar", "-x", "-C", dir)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	extract.Stdin = pipe
	if err := extract.Start(); err != nil {
		return err
	}
	if err := archive.Run(); err != nil {
		_ = extract.Wait()
		return err
	}
	return extract.Wait()
}

// linkNodeModules points the scratch copy at the installed dependencies, so
// node tests run there without a second install.
func linkNodeModules(root, scratch string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" || (strings.HasPrefix(name, ".") && p != root) {
			return filepath.SkipDir
		}
		if name != "node_modules" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if target := filepath.Join(scratch, rel); fileExists(filepath.Dir(target)) {
			_ = os.Symlink(p, target)
		}
		return filepath.SkipDir
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
