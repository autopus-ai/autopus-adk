package healthband

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Recovery step of SPEC-SIGMABAND-002 (REQ-11, REQ-12). It runs in every
// non-dry-run band run in which localpatch-events.jsonl exists, whatever the
// flag, after fetchCI and before phase A's store.Lock (plan task T8 wires it
// into internal/cli/react_band.go). It takes .autopus/metrics/.recovery.lock
// (wait at most 5 s, else reason recovery_locked), reads this SPEC's records
// under the store lock (local file IO only), and then, without the store
// lock, ends every local_patch claim and every flag-on diagnosis whose
// lease_until has passed without a result (never retried) by the Recovery
// State Table, with a budget of 120 s per claim and a 30 s timeout on every
// git call. Per claim it derives the paths (a mismatch ends it
// failed:record_invalid and touches nothing), takes the key lock with a zero
// wait (held elsewhere: no result, reason recovery_key_locked), re-reads the
// claim's records under the store lock and acts on that fresh read, never on
// its first snapshot (a result found there ends the handling with no action
// and no record), appends the write-once result under the store lock, and
// unlinks the key lock.

// RecoveryOptions configures the recovery step.
type RecoveryOptions struct {
	// Git runs in the user's checkout; every git call of recovery uses it.
	Git GitPolicyRunner
	// CacheDir is the user cache directory; "" is os.UserCacheDir.
	CacheDir string
	// Now is the clock that decides passed leases; nil is time.Now.
	Now func() time.Time
	// LockWait, ClaimBudget, and GitTimeout default to RecoveryLockWait,
	// LocalPatchRecoveryBudget, and LocalPatchGitTimeout.
	LockWait, ClaimBudget, GitTimeout time.Duration

	// beforeKeyLock runs between the first read and a claim's key lock (a
	// test seam for the CD-3 M7 races).
	beforeKeyLock func(claimID string)
}

// RecoveryReport is what one recovery step did.
type RecoveryReport struct {
	// Reasons are run reasons for the JSON envelope and text output:
	// recovery_locked, recovery_key_locked, or store_locked.
	Reasons []string
	// Results are the result records this step appended, in claim order.
	Results []LocalPatchRecord
	// KeyLocked are the claims skipped because another process holds their
	// key lock; a later run handles them.
	KeyLocked []string
}

// RecoverLocalPatches is the recovery step. It returns an error only for a
// store or lock I/O failure or a done context; every git or artifact fault
// keeps the artifact and ends the claim with its table state.
func (s *Store) RecoverLocalPatches(ctx context.Context, opts RecoveryOptions) (report RecoveryReport, err error) {
	if _, err := os.Lstat(s.Path(LocalPatchEventsFile)); errors.Is(err, os.ErrNotExist) {
		return report, nil
	} else if err != nil {
		return report, err
	}
	lock, err := s.lockRecovery(ctx, durationOr(opts.LockWait, RecoveryLockWait))
	if errors.Is(err, filelock.ErrTimeout) {
		report.Reasons = append(report.Reasons, ReasonRecoveryLocked)
		return report, nil
	}
	if err != nil {
		return report, err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	snapshot, err := s.ReadLocalPatchLocked(ctx)
	if errors.Is(err, ErrStoreLocked) {
		report.Reasons = append(report.Reasons, ReasonStoreLocked)
		return report, nil
	}
	if err != nil {
		return report, err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	due := snapshot.recoveryCandidates(now().UTC())
	if len(due) == 0 {
		return report, nil
	}
	r := &lpRecovery{store: s, opts: opts, report: &report}
	if ready, err := r.open(ctx); err != nil || !ready {
		return report, err
	}
	defer r.close()
	for _, claimID := range due {
		if err := r.recoverClaim(ctx, snapshot, claimID); err != nil {
			return report, err
		}
	}
	return report, nil
}

// lockRecovery takes .recovery.lock after refusing a symlinked .autopus or
// .autopus/metrics directory, as Store.Lock does.
func (s *Store) lockRecovery(ctx context.Context, wait time.Duration) (*filelock.Lock, error) {
	for _, dir := range []string{filepath.Dir(s.dir), s.dir} {
		if info, err := os.Lstat(dir); err != nil {
			return nil, err
		} else if !info.IsDir() {
			return nil, errUnsafeStorePath
		}
	}
	return filelock.Acquire(ctx, s.Path(RecoveryLockFile), wait)
}

// recoveryCandidates returns, in first-record order, every claim with a
// claim or prep record whose lease has passed at now without a result.
func (l *LocalPatchLog) recoveryCandidates(now time.Time) []string {
	var due, seen []string
	for _, record := range l.Records {
		id := record.RecordClaimID()
		if record.Kind != LocalPatchKindClaim && record.Kind != LocalPatchKindPrep || slices.Contains(seen, id) {
			continue
		}
		seen = append(seen, id)
		if lease := lpFactsOf(l.ClaimRecords(id)).lease; !l.Ended(id) && !lease.IsZero() && now.After(lease) {
			due = append(due, id)
		}
	}
	return due
}

type lpRecovery struct {
	store  *Store
	opts   RecoveryOptions
	report *RecoveryReport
	loc    LocalPatchLocation
	dir    *LocalPatchDir // nil when <lp> is unavailable: every claim is record_invalid
}

// open derives <lp> from the current user cache directory and the
// repository. An <lp> that Open refuses (cache_unavailable) leaves dir nil,
// so every due claim ends failed:record_invalid and nothing is touched. A
// repository that git cannot resolve, or a git call that was stopped at its
// timeout, leaves the step not ready, so a later run recovers instead.
func (r *lpRecovery) open(ctx context.Context) (ready bool, err error) {
	timeout := durationOr(r.opts.GitTimeout, LocalPatchGitTimeout)
	resolveCtx, cancel := context.WithTimeout(ctx, timeout)
	loc, err := ResolveLocalPatchLocation(resolveCtx, r.opts.Git, r.opts.CacheDir)
	cancel()
	if err != nil {
		return false, ctx.Err()
	}
	openCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	dir, _, err := loc.Open(openCtx, r.opts.Git)
	if err != nil {
		return false, ctx.Err()
	}
	r.loc, r.dir = loc, dir
	return true, nil
}

func (r *lpRecovery) close() {
	if r.dir != nil {
		_ = r.dir.Close()
	}
}

// recoverClaim handles one due claim within its budget.
func (r *lpRecovery) recoverClaim(ctx context.Context, snapshot *LocalPatchLog, claimID string) error {
	claimCtx, cancel := context.WithTimeout(ctx, durationOr(r.opts.ClaimBudget, LocalPatchRecoveryBudget))
	defer cancel()
	facts := lpFactsOf(snapshot.ClaimRecords(claimID))
	if r.dir == nil || !facts.derived(r.loc) {
		return r.end(ctx, NewLocalPatchResult(claimID, LocalPatchCodeRecordInvalid))
	}
	if r.opts.beforeKeyLock != nil {
		r.opts.beforeKeyLock(claimID)
	}
	lock, err := r.dir.AcquireKeyLock(claimCtx, facts.key)
	if err != nil {
		r.report.KeyLocked = append(r.report.KeyLocked, claimID)
		if !slices.Contains(r.report.Reasons, ReasonRecoveryKeyLocked) {
			r.report.Reasons = append(r.report.Reasons, ReasonRecoveryKeyLocked)
		}
		return nil
	}
	fresh, err := r.store.ReadLocalPatchLocked(ctx)
	if err == nil && !fresh.Ended(claimID) {
		facts = lpFactsOf(fresh.ClaimRecords(claimID))
		result := NewLocalPatchResult(claimID, LocalPatchCodeRecordInvalid)
		if facts.derived(r.loc) {
			result = r.settle(claimCtx, facts)
		}
		err = r.end(ctx, result)
	} else if errors.Is(err, ErrStoreLocked) {
		err = nil // the claim keeps no result; a later run handles it
	}
	return errors.Join(err, lock.Release())
}

// end appends a recovery result (write-once); a busy store lock leaves the
// claim for a later run.
func (r *lpRecovery) end(ctx context.Context, result LocalPatchRecord) error {
	result.Recovered = true
	appended, err := r.store.AppendLocalPatchResult(ctx, result)
	if errors.Is(err, ErrStoreLocked) {
		return nil
	}
	if appended {
		r.report.Results = append(r.report.Results, result)
	}
	return err
}

func durationOr(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
