package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// bandTrackedArgs lists, read-only, the tracked paths that may lie in the
// metric store: an entry at the store path and everything below it, in the
// spellings a case-insensitive file system resolves to .autopus/metrics.
// icase covers letter case, and the trailing globs cover a final s spelled ſ
// (U+017F), which APFS folds to s. storeTracked then keeps only the paths
// that are the store on disk. core.fsmonitor=false keeps git from starting
// the fsmonitor command that a repository's .git/config may name (security
// N1).
var bandTrackedArgs = []string{
	"-c", "core.fsmonitor=false", "ls-files", "-z", "--",
	":(icase,glob).autopu*/metric*", ":(icase,glob).autopu*/metric*/**",
}

// bandPathspecSwitches are the global pathspec settings git takes from the
// environment. The tracked-store check runs without them: an inherited
// GIT_LITERAL_PATHSPECS=1 makes git read the magic pathspecs as file names,
// and a conflicting pair makes it refuse them, so either would list nothing.
var bandPathspecSwitches = []string{"GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "GIT_ICASE_PATHSPECS"}

// errBandStoreTracked fails a run whose metric store git tracks: those files
// came with the repository, not from this machine's runs, so band neither
// trusts nor rewrites them. It is a fail-closed store exception to the exit
// rule of REQ-14, like a store I/O failure.
var errBandStoreTracked = errors.New("react band: .autopus/metrics/ is tracked by git; untrack it (git rm -r --cached .autopus/metrics) before running band")

// storeTracked reports whether git tracks any file in the metric store of
// projectDir. A listed path counts when it is in the store by identity, or
// when its identity cannot be settled, whatever git's exit status, and a
// listing above the output bound counts as a whole. Only a git that lists
// nothing of the store counts as untracked: a missing git, a directory
// outside any repository, or a repository git refuses to read has no index
// that could have brought store files in.
func (c bandGHClient) storeTracked(ctx context.Context, projectDir string) bool {
	env := orchestra.EnvironWithout(c.environ(), bandPathspecSwitches)
	command := bandCommand{Name: "git", Args: bandTrackedArgs, Dir: projectDir, Env: env}
	out := healthband.NewHeadBuffer(bandTextOutputCap)
	_ = c.capture(ctx, c.callTimeout, command, out)
	listing, dropped := out.Captured()
	return dropped || listsBandStore(projectDir, listing)
}

// listsBandStore reports whether a NUL-terminated ls-files listing, relative
// to projectDir, names a path in its metric store. A path does when its entry
// at the store's depth (its first two components) and the store directory
// stat to the same file. Identity that cannot be settled counts as the store,
// fail-closed: a stat of either path that fails, a record without two
// components, and a last record git did not finish.
func listsBandStore(projectDir, listing string) bool {
	if listing == "" {
		return false
	}
	records, complete := strings.CutSuffix(listing, "\x00")
	store, storeErr := os.Stat(healthband.NewStore(projectDir).Dir())
	if !complete || storeErr != nil {
		return true
	}
	for _, record := range strings.Split(records, "\x00") {
		parts := strings.SplitN(record, "/", 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return true
		}
		entry, entryErr := os.Stat(filepath.Join(projectDir, parts[0], parts[1]))
		if entryErr != nil || os.SameFile(entry, store) {
			return true
		}
	}
	return false
}

// guardStore refuses a tracked store before band reads, locks, or writes it.
func (r bandRun) guardStore(ctx context.Context, report *bandReport) error {
	if !r.client.storeTracked(ctx, r.projectDir) {
		return nil
	}
	report.Reasons = append(report.Reasons, healthband.ReasonStoreTracked)
	return errBandStoreTracked
}
