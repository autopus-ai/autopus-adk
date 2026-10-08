package cli

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

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

// lpDiskSpace is the free space of the file system that holds <lp>: Bavail
// blocks of the unit in which the file system counts Bavail.
type lpDiskSpace struct {
	avail, unit uint64
}

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
	dir      *healthband.LocalPatchDir  // the opened <lp>; nil when unavailable
	paths    healthband.LocalPatchPaths // derived from <lp> and key once dir is open
	worktree string                     // absolute; set once worktree_done is recorded
	baseSHA  string
	prepped  bool   // the prep record was appended
	code     string // "" while ok, else the code the claim ends with
	stopped  bool   // key lock busy or a result found: this claim gets no record
	lock     *healthband.LocalPatchKeyLock
	records  []healthband.LocalPatchRecord // prep and stage records appended, for the Cleanup Rules
	kept     []healthband.LocalPatchKept
}

// ready reports a worktree at the base SHA for the confined diagnosis.
func (s *localPatchSetup) ready() bool { return s.worktree != "" && s.code == "" && !s.stopped }

// prepare runs steps 1–2 of a flag-on diagnosis within the setup deadline.
func (p *bandLocalPatcher) prepare(ctx context.Context, target localPatchTarget) *localPatchSetup {
	d := target.diagnose
	s := &localPatchSetup{target: target, key: healthband.LocalPatchKey(d.Series, d.EpisodeID, target.recordClaimID())}
	if !p.covers(d.LeaseUntil, p.groups.setup, false) {
		s.code = healthband.LocalPatchCodeLeaseExhausted
		return s
	}
	gctx, cancel := p.group(ctx, d.LeaseUntil, p.groups.setup)
	defer cancel()
	code := p.firstChecks(gctx, s)
	if code == "" {
		if code = p.lockKey(gctx, s); s.stopped {
			p.release(s)
			return s
		}
	}
	if code == "" {
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
	run := p.git.In(p.checkout)
	code, err := run.CheckVersion(ctx)
	switch {
	case err != nil:
		return healthband.GitVersionUnsupportedPrefix + "unknown"
	case code != "":
		return code
	case p.location == nil:
		return healthband.LocalPatchCodeCacheUnavailable
	}
	dir, code, err := p.location.Open(ctx, run)
	if err != nil || code != "" {
		return healthband.LocalPatchCodeCacheUnavailable
	}
	s.dir, s.paths = dir, dir.Paths(s.key)
	if code, err := dir.CheckNoArtifacts(ctx, run, s.key); err != nil || code != "" {
		return healthband.LocalPatchCodeArtifactExists
	}
	if count, err := dir.KeptKeys(); err != nil {
		return healthband.LocalPatchCodeCacheUnavailable
	} else if count >= healthband.LocalPatchRetentionCap {
		return healthband.LocalPatchCodeCapReached
	}
	return ""
}

// lockKey takes <lp>/<key>.lock with a zero wait; a lock that another
// process holds stops the claim.
func (p *bandLocalPatcher) lockKey(ctx context.Context, s *localPatchSetup) string {
	lock, err := s.dir.AcquireKeyLock(ctx, s.key)
	switch {
	case errors.Is(err, healthband.ErrLocalPatchKeyLocked):
		s.stopped = true
		return ""
	case err != nil:
		return healthband.LocalPatchCodeCacheUnavailable
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
	space, err := p.statfs(s.dir.Path)
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
	prep := healthband.NewLocalPatchPrep(s.target.diagnose, s.target.claimID, s.key, s.baseSHA, defaultString(code, healthband.LocalPatchCodeOK))
	appended, err := p.ledger.AppendLocalPatchPrep(ctx, prep)
	switch {
	case err != nil:
		s.code = healthband.LocalPatchCodeRecordUnavailable
	case !appended:
		s.stopped = true
		p.release(s)
	default:
		s.prepped, s.code = true, code
		s.records = append(s.records, prep)
	}
	return s.prepped && code == ""
}

// appendStage appends one stage record of the claim and keeps it for the
// Cleanup Rules; an error ends the claim record_unavailable.
func (p *bandLocalPatcher) appendStage(ctx context.Context, s *localPatchSetup, stage healthband.LocalPatchRecord) error {
	if err := p.ledger.AppendLocalPatchStage(ctx, stage); err != nil {
		return err
	}
	s.records = append(s.records, stage)
	return nil
}

// cleanup applies the Cleanup Rules, in the order 3, 1, 2, to the artifacts
// that the claim's intent records name, within the cleanup deadline.
func (p *bandLocalPatcher) cleanup(ctx context.Context, s *localPatchSetup, lease time.Time, deadline time.Duration) []healthband.LocalPatchKept {
	if s.dir == nil {
		return nil
	}
	cctx, cancel := p.group(ctx, lease, deadline)
	defer cancel()
	return healthband.CleanupLocalPatch(cctx, p.git.In(p.checkout), s.dir, s.records)
}

// release unlinks and unlocks <lp>/<key>.lock while it is held (T2's
// Release), so a waiter that opened the old file fails its same-file
// check, and closes <lp>.
func (p *bandLocalPatcher) release(s *localPatchSetup) {
	if s.lock != nil {
		_ = s.lock.Release()
		s.lock = nil
	}
	if s.dir != nil {
		_ = s.dir.Close()
		s.dir = nil
	}
}
