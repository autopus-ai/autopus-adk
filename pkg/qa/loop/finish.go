package loop

import (
	"fmt"
	"path/filepath"
	"strings"
)

// madeCommit reports whether any iteration committed a fix.
func (r *runner) madeCommit() bool {
	if r.report.SeedCommit != "" {
		return true
	}
	for _, it := range r.report.Iterations {
		if it.Commit != "" {
			return true
		}
	}
	return false
}

// finish restores the original ref, writes both reports, and turns a stop
// other than passed into an error that names the branch and the report.
func (r *runner) finish() (Report, error) {
	args := []string{"switch", "-q", r.report.OriginalRef}
	if r.detached {
		args = []string{"switch", "-q", "--detach", r.report.OriginalRef}
	}
	_, switchErr := r.git.run(args...)
	r.report.Restored = switchErr == nil
	if r.report.Restored && !r.madeCommit() {
		_, deleteErr := r.git.run("branch", "-D", r.report.Branch)
		r.report.BranchDeleted = deleteErr == nil
	}
	r.report.FinishedAt = r.deps.Now().UTC()
	r.report.Quarantined = sortedKeys(r.flaky)
	r.report.FinalStatus = finalStatus(r.report.StopReason)
	dir := filepath.Join(r.projectDir, filepath.FromSlash(ReportDirRel), r.report.RunID)
	r.report.ReportPath = filepath.Join(dir, "report.json")
	r.report.ReportMDPath = filepath.Join(dir, "report.md")
	writeErr := writeReports(dir, r.report)

	code := r.report.StopCode
	var problems []string
	if r.report.FinalStatus != "passed" {
		problems = append(problems, fmt.Sprintf("%s: %s", r.report.StopReason, r.report.StopDetail))
	}
	if switchErr != nil {
		code = firstNonEmpty(code, CodeGitFailed)
		problems = append(problems, "could not restore "+r.report.OriginalRef+": "+switchErr.Error())
	}
	if writeErr != nil {
		code = firstNonEmpty(code, CodeReportFailed)
		problems = append(problems, "could not write the report: "+writeErr.Error())
	}
	if len(problems) == 0 {
		return r.report, nil
	}
	return r.report, &Error{Code: code, Message: fmt.Sprintf("%s (loop branch %s, report %s)", strings.Join(problems, "; "), r.report.Branch, r.report.ReportPath)}
}
