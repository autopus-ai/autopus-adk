package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Local Patch Flow step 1, inside a flag-on diagnose claim: the checks, the
// key lock, and the prep record after the store-locked re-read; step 2
// follows in react_band_localpatch_worktree.go. Every stop and failure
// leaves the diagnosis without a worktree, so it reports
// unavailable(worktree_unavailable) and 001 writes the evidence-only BS.

// Checkout preflight bounds (CD-3 M5).
const (
	lpMaxEntries   = 200_000
	lpMaxBlobBytes = 2 << 30
	lpDiskReserve  = 512 << 20
)

// localPatchTarget names the claims of one flag-on diagnosis.
type localPatchTarget struct {
	diagnose healthband.DueClaim
	claimID  string    // the local_patch claim id; "" for a diagnosis without one
	lease    time.Time // the local_patch claim's lease_until
}

// recordClaimID is the claim that the stage and result records name.
func (t localPatchTarget) recordClaimID() string {
	if t.claimID != "" {
		return t.claimID
	}
	return t.diagnose.ID
}

// localPatchSetup is what steps 1–2 left for the diagnosis and the patch stage.
type localPatchSetup struct {
	target   localPatchTarget
	key      string
	worktree string // absolute; set once worktree_done is recorded
	baseSHA  string
	prepped  bool   // the prep record was appended
	code     string // "" while ok, else the code the claim ends with
	stopped  bool   // key lock busy or a result found: this claim gets no record
	lock     *filelock.Lock
	kept     []localPatchKept
}

// ready reports a worktree at the base SHA for the confined diagnosis.
func (s *localPatchSetup) ready() bool { return s.worktree != "" && s.code == "" && !s.stopped }

// prepare runs steps 1–2 of a flag-on diagnosis within the setup deadline.
func (p *bandLocalPatcher) prepare(ctx context.Context, target localPatchTarget) *localPatchSetup {
	s := &localPatchSetup{target: target, key: bandLocalPatchKey(target.diagnose.Series, target.diagnose.EpisodeID, target.recordClaimID())}
	lease := target.diagnose.LeaseUntil
	if !p.covers(lease, p.groups.setup, false) {
		s.code = lpCodeLeaseExhausted
		return s
	}
	gctx, cancel := p.group(ctx, lease, p.groups.setup)
	defer cancel()
	code := p.firstChecks(gctx, s)
	if code == "" {
		code = p.lockKey(gctx, s)
		if s.stopped {
			return s
		}
	}
	if code == "" && s.lock != nil {
		code = p.repoChecks(gctx, s)
	}
	if p.appendPrep(ctx, s, code) {
		p.addWorktree(gctx, ctx, s)
	}
	return s
}

// firstChecks are the four codes that end step 1 before the key lock: the
// git version, <lp>, an existing artifact, and the retention count.
func (p *bandLocalPatcher) firstChecks(ctx context.Context, s *localPatchSetup) string {
	code, err := p.git.In(p.checkout).CheckVersion(ctx)
	switch {
	case err != nil:
		return healthband.GitVersionUnsupportedPrefix + "unknown"
	case code != "":
		return code
	case p.cache == nil || !p.cache.available():
		return lpCodeCacheUnavailable
	}
	exists, err := p.cache.artifactExists(s.key)
	if err != nil {
		return lpCodeCacheUnavailable
	}
	if exists || p.branchPresent(ctx, p.checkout, s.key) {
		return lpCodeArtifactExists
	}
	if count, err := p.cache.retained(); err != nil {
		return lpCodeCacheUnavailable
	} else if count >= lpRetentionCap {
		return lpCodeCapReached
	}
	return ""
}

// branchPresent reports refs/heads/autopus/band/<key> as a ref or as a
// symbolic ref; any answer but git's "absent" exit 1 counts as present.
func (p *bandLocalPatcher) branchPresent(ctx context.Context, dir, key string) bool {
	ref, run := healthband.BandBranchRef(key), p.git.In(dir)
	if _, err := run.Run(ctx, "rev-parse", "--verify", "--quiet", ref); healthband.GitExitCode(err) != 1 {
		return true
	}
	_, err := run.Run(ctx, "symbolic-ref", "-q", "--no-recurse", ref)
	return healthband.GitExitCode(err) != 1
}

// lockKey takes <lp>/<key>.lock with a zero wait; a lock that another
// process holds stops the claim.
func (p *bandLocalPatcher) lockKey(ctx context.Context, s *localPatchSetup) string {
	lock, err := filelock.Acquire(ctx, p.cache.path(s.key+".lock"), 0)
	switch {
	case errors.Is(err, filelock.ErrTimeout):
		s.stopped = true
		return ""
	case err != nil:
		return lpCodeCacheUnavailable
	}
	s.lock = lock
	return ""
}

// repoChecks run under the key lock: the configuration of the user's
// checkout (item 3, partial clones included), the base SHA (item 4), and
// the checkout preflight.
func (p *bandLocalPatcher) repoChecks(ctx context.Context, s *localPatchSetup) string {
	run := p.git.In(p.checkout)
	if code, err := run.CheckConfig(ctx); err != nil {
		return healthband.GitConfigUnsafePrefix + healthband.GitConfigUnreadable
	} else if code != "" {
		return code
	}
	base, code, err := run.ResolveBase(ctx, p.defaultBranch)
	if err != nil || code != "" {
		return healthband.GitBaseUnavailable
	}
	s.baseSHA = base
	out, err := run.Run(ctx, "ls-tree", "-r", "-l", "-z", base)
	if err != nil {
		return lpCodeWorktreeTooLarge
	}
	sizes, ok := lpTreeSizes(out)
	if !ok {
		return lpCodeWorktreeTooLarge
	}
	space, err := p.statfs(p.cache.dir)
	if err != nil || space.unit == 0 || space.avail < lpBlocksNeeded(sizes, space.unit) {
		return lpCodeDiskInsufficient
	}
	return ""
}

// lpTreeSizes reads git ls-tree -r -l -z: the blob size of every entry (a
// gitlink counts 0), refusing more than 200,000 entries or 2 GiB of blobs.
func lpTreeSizes(out []byte) ([]uint64, bool) {
	records := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(out) == 0 {
		records = nil
	}
	if len(records) > lpMaxEntries {
		return nil, false
	}
	sizes := make([]uint64, 0, len(records))
	var total uint64
	for _, record := range records {
		meta, _, found := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) != 4 {
			return nil, false
		}
		var size uint64
		if fields[3] != "-" {
			parsed, err := strconv.ParseUint(fields[3], 10, 64)
			if err != nil {
				return nil, false
			}
			size = parsed
		}
		if total += size; total > lpMaxBlobBytes {
			return nil, false
		}
		sizes = append(sizes, size)
	}
	return sizes, true
}

// lpBlocksNeeded is the free space a checkout needs in blocks of unit: every
// blob rounded up to whole blocks, plus 512 MiB.
func lpBlocksNeeded(sizes []uint64, unit uint64) uint64 {
	blocks := (lpDiskReserve + unit - 1) / unit
	for _, size := range sizes {
		blocks += (size + unit - 1) / unit
	}
	return blocks
}

// appendPrep writes the prep record after the store-locked re-read; it
// reports whether step 2 may start. A result found by the re-read stops the
// claim with no record, and a failed append ends it record_unavailable.
func (p *bandLocalPatcher) appendPrep(ctx context.Context, s *localPatchSetup, code string) bool {
	d := s.target.diagnose
	prep := localPatchPrep{
		Series: d.Series, EpisodeID: d.EpisodeID, DiagnoseClaimID: d.ID, LeaseUntil: d.LeaseUntil,
		ClaimID: s.target.claimID, Key: s.key, BaseSHA: s.baseSHA, Code: lpCodeOK,
	}
	if code != "" {
		prep.Code = code
	}
	appended, err := p.ledger.AppendPrep(ctx, prep)
	switch {
	case err != nil:
		s.code = lpCodeRecordUnavailable
	case !appended:
		s.stopped = true
		p.release(s)
	default:
		s.prepped, s.code = true, code
	}
	return s.prepped && code == ""
}

// cleanup applies the Cleanup Rules to the claim within the cleanup
// deadline, then removes an empty <lp>/<key>/ that the claim created.
func (p *bandLocalPatcher) cleanup(ctx context.Context, s *localPatchSetup, lease time.Time) []localPatchKept {
	cctx, cancel := p.group(ctx, lease, p.groups.cleanup)
	defer cancel()
	kept, err := p.cleaner.Cleanup(cctx, s.target.recordClaimID())
	if err != nil && p.warn != nil {
		fmt.Fprintf(p.warn, "react band: local patch cleanup of claim %s: %v\n", s.target.recordClaimID(), err)
	}
	_ = p.cache.removeEmpty(s.key)
	return kept
}

// release unlinks <lp>/<key>.lock while it is held, then unlocks it, so a
// waiter that opened the old file fails filelock's same-file check.
func (p *bandLocalPatcher) release(s *localPatchSetup) {
	if s.lock == nil {
		return
	}
	_ = p.cache.removeEmpty(s.key + ".lock")
	_ = s.lock.Unlock()
	s.lock = nil
}
