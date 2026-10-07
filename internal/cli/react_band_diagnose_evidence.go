package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandRunEvidence is the evidence source of CI diagnose claims (FR-11): the
// failed-step log of each failed run attempt of the current block, fetched
// through the gh Invocation Table, and an existing .autopus/react/<run>.md
// report, read only as untrusted evidence (REQ-17). Both pass the Untrusted
// Input Contract; a failed fetch or read is skipped, never kept partly.
type bandRunEvidence struct {
	client     bandGHClient
	target     *bandGHTarget // nil when no repository was resolved: no gh call runs
	projectDir string
}

// Evidence fetches at most K logs, so the claim stays inside its 4×60 s log
// budget, and reads the react report of each of those runs.
func (e bandRunEvidence) Evidence(ctx context.Context, claim healthband.DueClaim) ([]healthband.RunLog, []healthband.ReactReport) {
	var logs []healthband.RunLog
	var reports []healthband.ReactReport
	for _, run := range claim.FailedRuns[:min(len(claim.FailedRuns), healthband.BlockSize)] {
		if e.target != nil {
			if evidence, err := e.client.failedRunEvidence(ctx, *e.target, e.projectDir, run.RunID, run.Attempt); err == nil {
				logs = append(logs, healthband.RunLog{RunID: run.RunID, Attempt: run.Attempt, Evidence: evidence})
			}
		}
		if evidence, ok := readBandReactReport(e.projectDir, run.RunID); ok {
			reports = append(reports, healthband.ReactReport{RunID: run.RunID, Evidence: evidence})
		}
	}
	return logs, reports
}

// readBandReactReport keeps the last 4 MiB of a react report that is a
// regular file in a real .autopus/react directory (neither component a
// symlink) and sanitizes it like a CI log, whose tail it ends with. The
// file is opened without following a link or blocking on a FIFO and is
// checked again after the open, so a planted or swapped link cannot pull a
// file from outside the project into a prompt or a BS.
func readBandReactReport(projectDir string, runID int64) (healthband.Evidence, bool) {
	autopus := filepath.Join(projectDir, ".autopus")
	dir := filepath.Join(autopus, "react")
	for _, component := range []string{autopus, dir} {
		if info, err := os.Lstat(component); err != nil || !info.IsDir() {
			return healthband.Evidence{}, false
		}
	}
	file, err := healthband.OpenRegular(filepath.Join(dir, strconv.FormatInt(runID, 10)+".md"))
	if err != nil {
		return healthband.Evidence{}, false
	}
	defer func() { _ = file.Close() }()
	tail := healthband.NewTailBuffer(healthband.CILogCaptureBytes)
	if _, err := io.Copy(tail, file); err != nil {
		return healthband.Evidence{}, false
	}
	captured, dropped := tail.Captured()
	return healthband.SanitizeCILog(captured, dropped, projectDir), true
}
