package loop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Restored names the files RestoreLastSeed brought back and where from.
type Restored struct {
	Branch string
	Files  []string
}

// RestoreLastSeed brings back the QA files the latest seeded loop committed,
// for a rerun whose working tree has none. A loop commits its scenarios on its
// own branch and leaves the caller's branch untouched, so after a stop the
// scenarios are only on that branch. The files are read from the branch tip,
// which includes any test fixes the loop made, and written to the working
// tree only: nothing is staged, and a file that already exists is kept. The
// product fixes on that branch are not brought back.
func RestoreLastSeed(projectDir string, dirs []string) (Restored, error) {
	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return Restored{}, err
	}
	branch := LatestSeededBranch(projectDir)
	if branch == "" || len(dirs) == 0 {
		return Restored{}, nil
	}
	out, err := runGit(projectDir, nil, append([]string{"ls-tree", "-r", "-z", "--name-only", branch, "--"}, dirs...)...)
	if err != nil {
		return Restored{}, err
	}
	var missing []string
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(projectDir, filepath.FromSlash(rel))); os.IsNotExist(err) {
			missing = append(missing, rel)
		}
	}
	if len(missing) == 0 {
		return Restored{}, nil
	}
	if _, err := runGit(projectDir, nil, append([]string{"restore", "--source=" + branch, "--worktree", "--"}, missing...)...); err != nil {
		return Restored{}, err
	}
	return Restored{Branch: branch, Files: missing}, nil
}

// LatestSeededBranch reads the loop reports, newest first, and returns the
// first branch that carries a seed commit and still exists.
func LatestSeededBranch(projectDir string) string {
	paths, _ := filepath.Glob(filepath.Join(projectDir, filepath.FromSlash(ReportDirRel), "*", "report.json"))
	var reports []Report
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var rep Report
		if json.Unmarshal(data, &rep) == nil && rep.SeedCommit != "" && !rep.BranchDeleted &&
			strings.HasPrefix(rep.Branch, BranchPrefix) {
			reports = append(reports, rep)
		}
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].StartedAt.After(reports[j].StartedAt) })
	for _, rep := range reports {
		if _, err := runGit(projectDir, nil, "rev-parse", "--verify", "--quiet", "refs/heads/"+rep.Branch); err == nil {
			return rep.Branch
		}
	}
	return ""
}
