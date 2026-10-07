package cli

import (
	"context"
	"errors"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandTrackedArgs lists the tracked files under the metric store, read-only.
var bandTrackedArgs = []string{"ls-files", "-z", "--", ".autopus/metrics"}

// errBandStoreTracked fails a run whose metric store git tracks: those files
// came with the repository, not from this machine's runs, so band neither
// trusts nor rewrites them. It is a fail-closed store exception to the exit
// rule of REQ-14, like a store I/O failure.
var errBandStoreTracked = errors.New("react band: .autopus/metrics/ is tracked by git; untrack it (git rm -r --cached .autopus/metrics) before running band")

// storeTracked reports whether git tracks any file under the metric store of
// projectDir. A directory outside a repository, a missing git, or a failed
// or oversized listing counts as untracked: there is then no index that
// could have brought store files in.
func (c bandGHClient) storeTracked(ctx context.Context, projectDir string) bool {
	command := bandCommand{Name: "git", Args: bandTrackedArgs, Dir: projectDir}
	listing, err := c.output(ctx, command, bandTextOutputCap)
	return err == nil && listing != ""
}

// guardStore refuses a tracked store before band reads, locks, or writes it.
func (r bandRun) guardStore(ctx context.Context, report *bandReport) error {
	if !r.client.storeTracked(ctx, r.projectDir) {
		return nil
	}
	report.Reasons = append(report.Reasons, healthband.ReasonStoreTracked)
	return errBandStoreTracked
}
