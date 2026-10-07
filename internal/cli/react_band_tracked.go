package cli

import (
	"context"
	"errors"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// bandTrackedArgs lists the tracked files under the metric store, read-only.
// core.fsmonitor=false keeps git from starting the fsmonitor command that a
// repository's .git/config may name (security N1), and the icase pathspec
// also matches a case variant such as .autopus/METRICS, which is the store
// itself on a case-insensitive file system.
var bandTrackedArgs = []string{"-c", "core.fsmonitor=false", "ls-files", "-z", "--", ":(icase).autopus/metrics"}

// errBandStoreTracked fails a run whose metric store git tracks: those files
// came with the repository, not from this machine's runs, so band neither
// trusts nor rewrites them. It is a fail-closed store exception to the exit
// rule of REQ-14, like a store I/O failure.
var errBandStoreTracked = errors.New("react band: .autopus/metrics/ is tracked by git; untrack it (git rm -r --cached .autopus/metrics) before running band")

// storeTracked reports whether git tracks any file under the metric store of
// projectDir. Any listed byte counts, whatever git's exit status, and so does
// a listing above the output bound. Only a git that lists nothing counts as
// untracked: a missing git, a directory outside any repository, or a
// repository git refuses to read has no index that could have brought store
// files in.
func (c bandGHClient) storeTracked(ctx context.Context, projectDir string) bool {
	command := bandCommand{Name: "git", Args: bandTrackedArgs, Dir: projectDir}
	out := healthband.NewHeadBuffer(bandTextOutputCap)
	_ = c.capture(ctx, c.callTimeout, command, out)
	listing, dropped := out.Captured()
	return dropped || listing != ""
}

// guardStore refuses a tracked store before band reads, locks, or writes it.
func (r bandRun) guardStore(ctx context.Context, report *bandReport) error {
	if !r.client.storeTracked(ctx, r.projectDir) {
		return nil
	}
	report.Reasons = append(report.Reasons, healthband.ReasonStoreTracked)
	return errBandStoreTracked
}
