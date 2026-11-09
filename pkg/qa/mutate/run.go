package mutate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/execcheck"
	"github.com/insajin/autopus-adk/pkg/qa/testpath"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// Verdicts of one mutant.
const (
	Killed   = "killed"   // a case failed: the scenarios notice this fault
	Survived = "survived" // every case passed: the fault goes unnoticed
	Invalid  = "invalid"  // the product no longer builds or the cases cannot run
)

// DefaultMax is how many mutants one run tries when the caller sets none.
const DefaultMax = 20

// Options configures a run.
type Options struct {
	ProjectDir string
	Max        int
	// Progress, when set, receives one line per mutant as it finishes.
	Progress func(string)
}

// Outcome is one mutant and what the cases made of it.
type Outcome struct {
	Mutant
	Verdict string `json:"verdict"`
	// KilledBy is the first case that failed on the mutant.
	KilledBy string `json:"killed_by,omitempty"`
}

// Report is the whole run.
type Report struct {
	Cases    []string  `json:"cases"`
	Excluded []string  `json:"excluded_cases,omitempty"`
	Total    int       `json:"candidates"`
	Outcomes []Outcome `json:"outcomes"`
	Killed   int       `json:"killed"`
	Survived int       `json:"survived"`
	Invalid  int       `json:"invalid"`
	Score    float64   `json:"score"`
}

type namedCase struct {
	id   string
	exec testscenario.Exec
}

// Run plants each sampled mutant in a scratch copy of the project, one at a
// time, and runs the exec cases that pass on the unmodified product. The
// project itself is never written.
func Run(ctx context.Context, opts Options) (Report, error) {
	project, err := filepath.Abs(opts.ProjectDir)
	if err != nil {
		return Report{}, err
	}
	cases, err := loadCases(project)
	if err != nil {
		return Report{}, err
	}
	if len(cases) == 0 {
		return Report{}, fmt.Errorf("no exec cases under %s", testscenario.DirRel)
	}
	files, err := productFiles(ctx, project)
	if err != nil {
		return Report{}, err
	}
	work, err := os.MkdirTemp("", "autopus-qa-mutate-")
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := copyFiles(project, work, files); err != nil {
		return Report{}, err
	}
	var report Report
	var live []namedCase
	for _, c := range cases {
		if res, err := execcheck.Run(ctx, work, c.exec); err == nil && res.Passed() {
			live = append(live, c)
			report.Cases = append(report.Cases, c.id)
		} else {
			report.Excluded = append(report.Excluded, c.id)
		}
	}
	if len(live) == 0 {
		return report, fmt.Errorf("no exec case passes on the unmodified product; fix the product or the cases first")
	}
	var all []Mutant
	for _, f := range files {
		if testpath.IsTestPath(f) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(f)))
		if err == nil {
			all = append(all, Candidates(f, string(data))...)
		}
	}
	report.Total = len(all)
	max := opts.Max
	if max == 0 {
		max = DefaultMax
	}
	for _, m := range Sample(all, max) {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		out, err := try(ctx, work, m, live)
		if err != nil {
			return report, err
		}
		report.add(out)
		if opts.Progress != nil {
			opts.Progress(fmt.Sprintf("%s %s:%d %s", out.Verdict, m.File, m.Line, m.Operator))
		}
	}
	return report, nil
}

// try plants m, runs the cases until one fails, and restores the file. A
// restore that fails is an error: every later mutant would run on top of it.
func try(ctx context.Context, work string, m Mutant, cases []namedCase) (Outcome, error) {
	path := filepath.Join(work, filepath.FromSlash(m.File))
	original, err := os.ReadFile(path)
	if err != nil {
		return Outcome{}, err
	}
	lines := strings.Split(string(original), "\n")
	lines[m.Line-1] = m.Mutated
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return Outcome{}, err
	}
	out := judge(ctx, work, m, cases)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		return Outcome{}, fmt.Errorf("restore %s after a mutant: %w", m.File, err)
	}
	return out, nil
}

func judge(ctx context.Context, work string, m Mutant, cases []namedCase) Outcome {
	for _, c := range cases {
		res, err := execcheck.Run(ctx, work, c.exec)
		switch {
		case err != nil && strings.Contains(err.Error(), "did not finish within"):
			// A fault that makes the product hang is one a user notices.
			return Outcome{Mutant: m, Verdict: Killed, KilledBy: c.id + " (hang)"}
		case err != nil || buildFailed(res):
			return Outcome{Mutant: m, Verdict: Invalid}
		case !res.Passed():
			return Outcome{Mutant: m, Verdict: Killed, KilledBy: c.id}
		}
	}
	return Outcome{Mutant: m, Verdict: Survived}
}

// buildFailed reports the product verdict execcheck gives a compile error:
// the fault broke the build, which says nothing about the scenarios.
func buildFailed(res execcheck.Result) bool {
	return len(res.Mismatches) == 1 && res.Mismatches[0].Field == "build"
}

func (r *Report) add(o Outcome) {
	r.Outcomes = append(r.Outcomes, o)
	switch o.Verdict {
	case Killed:
		r.Killed++
	case Survived:
		r.Survived++
	default:
		r.Invalid++
	}
	if judged := r.Killed + r.Survived; judged > 0 {
		r.Score = float64(r.Killed) / float64(judged)
	}
}

func loadCases(project string) ([]namedCase, error) {
	paths, _ := filepath.Glob(filepath.Join(project, testscenario.DirRel, "*.yaml"))
	sort.Strings(paths)
	var out []namedCase
	for _, p := range paths {
		doc, err := testscenario.LoadFile(p)
		if err != nil {
			return nil, err
		}
		for _, c := range doc.Cases {
			if c.Automation.Type == testscenario.AutomationExec && c.Automation.Exec != nil {
				out = append(out, namedCase{id: c.ID, exec: *c.Automation.Exec})
			}
		}
	}
	return out, nil
}

// productFiles lists the project's tracked and untracked, non-ignored files
// outside the harness directories, relative to the project.
func productFiles(ctx context.Context, project string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = project
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mutate needs a git work tree to know the product files: %w", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" || strings.HasPrefix(f, ".autopus/") || strings.HasPrefix(f, ".git/") {
			continue
		}
		if _, err := os.Lstat(filepath.Join(project, filepath.FromSlash(f))); err == nil {
			files = append(files, f)
		}
	}
	return files, nil
}

func copyFiles(from, to string, files []string) error {
	for _, f := range files {
		src := filepath.Join(from, filepath.FromSlash(f))
		info, err := os.Lstat(src)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
