package editguard

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/oslock"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

const (
	storeLockName = ".store.lock"
	tempPrefix    = ".tmp-"
	storeLockWait = 5 * time.Second
	storeLockPoll = 10 * time.Millisecond
	recordPerm    = 0o644
)

// Store is the fix-lock store of one project root. Every mutating call holds
// the exclusive store lock for its whole duration, so lock and unlock batches
// serialize and a rollback never undoes another call's records (REQ-EG-06, 08).
type Store struct {
	// Now is the clock for record timestamps and expiry; nil means time.Now.
	Now func() time.Time

	cwd, root string
	fold      bool
	wait      time.Duration
	seams     storeSeams
}

// storeSeams are test-only fault and pause points; production leaves them nil.
type storeSeams struct {
	beforePublish func(rel string) error
	afterPublish  func(rel string)
	afterVerdicts func()
	beforeRemove  func(path string) error
	onBusy        func()
}

// OpenStore returns the store of the project that contains cwd. Relative
// paths given to its methods resolve against cwd.
func OpenStore(cwd string) (*Store, error) {
	root, err := FindProjectRoot(cwd)
	if err != nil {
		return nil, err
	}
	return &Store{cwd: cwd, root: root, fold: volumeIsCaseInsensitive(root), wait: storeLockWait}, nil
}

// Root returns the project root the store belongs to.
func (s *Store) Root() string { return s.root }

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Lock records the SHA-256 of every named file. It validates the whole batch
// before writing, keeps an unexpired record unchanged so the first hash
// survives re-locking, replaces an expired one, and on a failure removes the
// records this call published, leaving the store as it was. A ttl of 0 means
// DefaultLockTTL.
func (s *Store) Lock(paths []string, ttl time.Duration) error {
	if ttl == 0 {
		ttl = DefaultLockTTL
	}
	if ttl < MinLockTTL || ttl > MaxLockTTL {
		return ErrInvalidTTL
	}
	targets, err := s.lockTargets(paths)
	if err != nil {
		return err
	}
	return s.withStoreLock(func(dir *os.Root) error {
		plans, err := s.planLocks(dir, targets, ttl)
		if err != nil {
			return err
		}
		return s.publish(dir, plans)
	})
}

func (s *Store) lockTargets(paths []string) ([]Target, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: no path given", ErrInvalidLockTarget)
	}
	seen := make(map[string]bool, len(paths))
	targets := make([]Target, 0, len(paths))
	for _, raw := range paths {
		target, err := Resolve(s.cwd, raw)
		if err != nil || target.Root != s.root || !isRegularFile(target.Abs()) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidLockTarget, displayPath(raw))
		}
		if !seen[target.Key] {
			seen[target.Key] = true
			targets = append(targets, target)
		}
	}
	return targets, nil
}

// lockPlan is one record a lock call publishes.
type lockPlan struct {
	rel, name string
	data      []byte
	prev      []byte // the expired record this one replaces, nil for a new one
}

func (s *Store) planLocks(dir *os.Root, targets []Target, ttl time.Duration) ([]lockPlan, error) {
	project, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLockState, err)
	}
	defer func() { _ = project.Close() }()
	now := s.now()
	created := now.UTC().Truncate(time.Second)
	plans := make([]lockPlan, 0, len(targets))
	for _, target := range targets {
		name := recordName(target.Key)
		existing, found, err := lookupRecord(dir, name, s.fold)
		if err != nil || existing.corrupt {
			return nil, fmt.Errorf("%w: the record of %s is unreadable; run auto fix unlock --all",
				ErrLockState, displayPath(target.Rel))
		}
		if found && now.Before(existing.expires) {
			continue
		}
		digest, err := fileDigest(project, target.Rel)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidLockTarget, displayPath(target.Rel))
		}
		// Marshal cannot fail for a struct of strings.
		data, _ := json.Marshal(LockRecord{Schema: LockSchema, Path: target.Rel, SHA256: digest,
			CreatedAt: created.Format(time.RFC3339), ExpiresAt: created.Add(ttl).Format(time.RFC3339)})
		plans = append(plans, lockPlan{rel: target.Rel, name: name, data: data, prev: existing.data})
	}
	return plans, nil
}

func isRegularFile(name string) bool {
	info, err := os.Lstat(name)
	return err == nil && info.Mode().IsRegular()
}

func lookupRecord(dir *os.Root, name string, fold bool) (storedRecord, bool, error) {
	if _, err := dir.Lstat(name); errors.Is(err, fs.ErrNotExist) {
		return storedRecord{}, false, nil
	} else if err != nil {
		return storedRecord{}, false, err
	}
	return readRecord(dir, name, fold), true, nil
}

func (s *Store) publish(dir *os.Root, plans []lockPlan) error {
	for i, plan := range plans {
		if err := s.publishOne(dir, plan); err != nil {
			return errors.Join(fmt.Errorf("editguard: publish the lock of %s: %w", displayPath(plan.rel), err),
				rollback(dir, plans[:i]))
		}
		if s.seams.afterPublish != nil {
			s.seams.afterPublish(plan.rel)
		}
	}
	return nil
}

func (s *Store) publishOne(dir *os.Root, plan lockPlan) error {
	if s.seams.beforePublish != nil {
		if err := s.seams.beforePublish(plan.rel); err != nil {
			return err
		}
	}
	if plan.prev != nil {
		return replaceRecord(dir, plan.name, plan.data)
	}
	temp, err := writeTemp(dir, plan.data)
	if err != nil {
		return err
	}
	// Link refuses an existing name, so a record is created exactly once, and a
	// reader only ever sees the bytes of a complete temp file.
	return errors.Join(dir.Link(temp, plan.name), removeIfPresent(dir, temp))
}

// replaceRecord swaps name's content atomically through a complete temp file.
func replaceRecord(dir *os.Root, name string, data []byte) error {
	temp, err := writeTemp(dir, data)
	if err != nil {
		return err
	}
	if err := dir.Rename(temp, name); err != nil {
		return errors.Join(err, removeIfPresent(dir, temp))
	}
	return nil
}

// rollback undoes this call's published records, newest first: a new record
// is removed and a replaced expired one gets its previous bytes back.
func rollback(dir *os.Root, done []lockPlan) error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		if done[i].prev != nil {
			errs = append(errs, replaceRecord(dir, done[i].name, done[i].prev))
		} else {
			errs = append(errs, removeIfPresent(dir, done[i].name))
		}
	}
	return errors.Join(errs...)
}

func writeTemp(dir *os.Root, data []byte) (string, error) {
	name := tempPrefix + rand.Text()
	file, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, recordPerm)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return "", errors.Join(err, removeIfPresent(dir, name))
	}
	return name, nil
}

func removeIfPresent(dir *os.Root, name string) error {
	if err := dir.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// withStoreLock runs fn holding the exclusive store lock, waiting at most 5
// seconds for it. The OS releases the lock if this process dies.
func (s *Store) withStoreLock(fn func(dir *os.Root) error) error {
	dir, err := rulecond.OpenRuntimeStateDir(s.root, fixLocksName)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrLockState, FixLocksDir)
	}
	defer func() { _ = dir.Close() }()
	lock, err := openStoreLock(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := s.acquire(lock); err != nil {
		return err
	}
	defer func() { _ = oslock.Unlock(lock) }()
	return fn(dir)
}

func openStoreLock(dir *os.Root) (*os.File, error) {
	unusable := fmt.Errorf("%w: %s/%s", ErrLockState, FixLocksDir, storeLockName)
	if info, err := dir.Lstat(storeLockName); err == nil && !info.Mode().IsRegular() {
		return nil, unusable
	}
	file, err := dir.OpenFile(storeLockName, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, unusable
	}
	opened, openedErr := file.Stat()
	named, namedErr := dir.Lstat(storeLockName)
	if openedErr != nil || namedErr != nil || !opened.Mode().IsRegular() || !os.SameFile(opened, named) {
		_ = file.Close()
		return nil, unusable
	}
	return file, nil
}

func (s *Store) acquire(lock *os.File) error {
	deadline := time.Now().Add(s.wait)
	for {
		busy, err := oslock.TryLock(lock)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrLockState, err)
		}
		if !busy {
			return nil
		}
		if s.seams.onBusy != nil {
			s.seams.onBusy()
		}
		if !time.Now().Before(deadline) {
			return ErrStoreBusy
		}
		time.Sleep(storeLockPoll)
	}
}
