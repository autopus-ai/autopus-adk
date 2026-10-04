package loop

import (
	"path"
	"strings"
)

// Seeding lets `auto qa go` hand the loop QA files it just generated:
// scenarios, test scenarios, and compiled specs. They are committed as the
// first commit on the loop branch, so the branch carries the intent and the
// fixes it justified together and the caller's own branch is never written.

// seedRepoPaths maps project-relative seed paths onto repository paths.
func (r *runner) seedRepoPaths() []string {
	out := make([]string, 0, len(r.opts.SeedPaths))
	for _, p := range r.opts.SeedPaths {
		p = strings.Trim(path.Clean("/"+strings.TrimSpace(p)), "/")
		if p == "" || p == "." {
			continue
		}
		out = append(out, r.prefix+p)
	}
	return out
}

// outsideSeeds drops `git status --porcelain` entries that fall under a seed
// path: those changes are the seed itself, not unrelated work in progress.
func outsideSeeds(entries, seeds []string) []string {
	var out []string
	for _, entry := range entries {
		if !underAny(porcelainPath(entry), seeds) {
			out = append(out, entry)
		}
	}
	return out
}

// porcelainPath extracts the path of a trimmed `XY path` porcelain entry; for
// a rename it is the destination.
func porcelainPath(entry string) string {
	fields := strings.SplitN(entry, " ", 2)
	if len(fields) < 2 {
		return entry
	}
	p := strings.TrimSpace(fields[1])
	if _, dest, ok := strings.Cut(p, " -> "); ok {
		p = dest
	}
	return strings.Trim(p, `"`)
}

func underAny(p string, roots []string) bool {
	for _, root := range roots {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

// commitSeed stages the seed paths on the loop branch and commits them when
// anything changed. A failed commit unstages them again, so nothing is left
// in the index when the caller's branch is restored.
func (r *runner) commitSeed() error {
	seeds := r.seedRepoPaths()
	if len(seeds) == 0 {
		return nil
	}
	existing := []string{}
	for _, p := range seeds {
		if out, err := r.git.run("ls-files", "--others", "--cached", "--modified", "--", p); err == nil && strings.TrimSpace(out) != "" {
			existing = append(existing, p)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	if _, err := r.git.run(append([]string{"add", "-A", "--"}, existing...)...); err != nil {
		return err
	}
	if _, err := r.git.run("diff", "--cached", "--quiet"); err == nil {
		return nil // nothing new to commit
	}
	sha, err := r.git.commit(existing, r.seedMessage())
	if err != nil {
		_, _ = r.git.run(append([]string{"reset", "-q", "--"}, existing...)...)
		return err
	}
	r.report.SeedCommit = sha
	return nil
}

func (r *runner) seedMessage() string {
	subject := strings.TrimSpace(r.opts.SeedMessage)
	if subject == "" {
		subject = "test(qa): QA 시나리오를 추가한다"
	}
	return subject + "\n\n`auto qa go`가 생성·승격·컴파일한 QA 시나리오다. 이후 커밋은 이 시나리오의\n실패를 분류해 고친 결과다.\n\nConstraint: 기대값은 SPEC 수락 기준이나 확인된 녹화에서 온다\nConfidence: medium\nRelated: " + r.report.RunID + "\n\n🐙 Autopus <noreply@autopus.co>\n"
}

// seedRoots is seedRepoPaths before a runner exists, for the dirty check.
func seedRoots(seeds []string, prefix string) []string {
	r := &runner{opts: Options{SeedPaths: seeds}, prefix: prefix}
	return r.seedRepoPaths()
}

// abandonBranch returns to the original ref and deletes the loop branch when
// the run cannot start; nothing was committed on it that the caller needs.
func (r *runner) abandonBranch() {
	args := []string{"switch", "-q", r.report.OriginalRef}
	if r.detached {
		args = []string{"switch", "-q", "--detach", r.report.OriginalRef}
	}
	if _, err := r.git.run(args...); err == nil {
		_, _ = r.git.run("branch", "-D", r.report.Branch)
	}
}
