package editguard

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

// faultOnCall returns a seam that fails the nth call (1-based).
func faultOnCall(n int) func(string) error {
	var mu sync.Mutex
	calls := 0
	return func(string) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == n {
			return errInjected
		}
		return nil
	}
}

// signalOnce returns a seam that closes ch on its first call.
func signalOnce(ch chan struct{}) func() {
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

func run(fn func() error) chan error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	return done
}

// seamTimeout bounds every wait on a seam, so a broken store lock fails the
// test instead of hanging it.
const seamTimeout = 10 * time.Second

func await(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(seamTimeout):
		t.Fatalf("timed out waiting for the %s seam", what)
	}
}

func awaitErr(t *testing.T, done chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(seamTimeout):
		t.Fatal("timed out waiting for a store call to finish")
		return nil
	}
}

func requirePending(t *testing.T, done chan error, what string) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s finished while another call held the store lock: %v", what, err)
	case <-time.After(50 * time.Millisecond):
	}
}

// S17 steps 1 to 3: lock batches are all-or-nothing, and a rollback never
// undoes the success of a call that waited for the store lock.
func TestLock_BatchFailures_LeaveTheStoreAsItWas(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	clock := newClock(t0)
	store := openTestStore(t, root, clock)
	if err := store.Lock([]string{tRel, "missing_test.go"}, 0); !errors.Is(err, ErrInvalidLockTarget) {
		t.Fatalf("step 1 err = %v", err)
	}
	store.seams.beforePublish = faultOnCall(2)
	if err := store.Lock([]string{tRel, uRel}, 0); !errors.Is(err, errInjected) {
		t.Fatalf("step 2 err = %v", err)
	}
	if got := listPaths(t, store); len(got) != 0 {
		t.Fatalf("step 2 left locks %v", got)
	}

	a, b := openTestStore(t, root, clock), openTestStore(t, root, clock)
	published, release, waiting := make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.seams.beforePublish = func(rel string) error {
		if rel == xRel {
			return errInjected
		}
		return nil
	}
	a.seams.afterPublish = func(rel string) {
		if rel == uRel {
			close(published)
			<-release
		}
	}
	b.seams.onBusy = signalOnce(waiting)
	aDone := run(func() error { return a.Lock([]string{uRel, xRel}, 0) })
	await(t, published, "published")
	if got := listPaths(t, store); !reflect.DeepEqual(got, []string{uRel}) {
		t.Fatalf("a reader during A's batch saw %v, want A's in-flight record", got)
	}
	bDone := run(func() error { return b.Lock([]string{uRel}, 0) })
	await(t, waiting, "waiting")
	requirePending(t, bDone, "B")
	clock.Set(t0.Add(time.Minute))
	close(release)
	if err := awaitErr(t, aDone); !errors.Is(err, errInjected) {
		t.Fatalf("A err = %v, want the injected publish fault", err)
	}
	if err := awaitErr(t, bDone); err != nil {
		t.Fatalf("B err = %v", err)
	}
	entries, err := store.List()
	if err != nil || len(entries) != 1 || entries[0].Path != uRel || entries[0].CreatedAt != "2026-10-07T09:01:00Z" {
		t.Fatalf("after A and B: %+v, %v; want one lock for u_test.go created after A exited", entries, err)
	}
}

// S17 steps 4 to 10: concurrent locks keep the first hash, unlock decides
// before it removes, and a removal fault is finished by a rerun.
func TestLockAndUnlock_ConcurrentAndPausedTransitions(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	clock := newClock(t0)
	store := openTestStore(t, root, clock)
	mustLock(t, store, uRel)

	lockers := make([]*Store, 12)
	for i := range lockers {
		lockers[i] = openTestStore(t, root, clock)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for _, locker := range lockers[:8] {
		wg.Go(func() { errs <- locker.Lock([]string{tRel}, 0) })
	}
	wg.Wait()
	wantList := []ListEntry{
		{Path: tRel, State: LockStateActive, CreatedAt: "2026-10-07T09:00:00Z", ExpiresAt: "2026-10-08T09:00:00Z", Integrity: VerdictUnchanged},
		{Path: uRel, State: LockStateActive, CreatedAt: "2026-10-07T09:00:00Z", ExpiresAt: "2026-10-08T09:00:00Z", Integrity: VerdictUnchanged},
	}
	if got, err := store.List(); err != nil || !reflect.DeepEqual(got, wantList) {
		t.Fatalf("after 8 concurrent locks: %+v, %v", got, err)
	}
	for _, locker := range lockers[8:] {
		wg.Go(func() { errs <- os.WriteFile(filepath.Join(root, tRel), []byte(weakContent), 0o644) })
		wg.Go(func() { errs <- locker.Lock([]string{tRel}, 0) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent lock or rewrite: %v", err)
		}
	}
	if got := recordHashOf(t, root, tRel); got != tHash {
		t.Fatalf("T's record hash = %s, want the first hash %s", got, tHash)
	}
	if _, err := store.Unlock([]string{tRel, "not_locked_test.go"}); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("step 6 err = %v", err)
	}

	c, d := openTestStore(t, root, clock), openTestStore(t, root, clock)
	computed, release, waiting := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c.seams.afterVerdicts = func() { close(computed); <-release }
	d.seams.onBusy = signalOnce(waiting)
	var cResults []UnlockResult
	cDone := run(func() (err error) { cResults, err = c.Unlock([]string{tRel}); return err })
	await(t, computed, "computed")
	dDone := run(func() error { return d.Lock([]string{tRel}, 0) })
	await(t, waiting, "waiting")
	requirePending(t, dDone, "D")
	close(release)
	if err := awaitErr(t, cDone); err != nil {
		t.Fatalf("C err = %v", err)
	}
	if err := awaitErr(t, dDone); err != nil {
		t.Fatalf("D err = %v", err)
	}
	want := []UnlockResult{{Path: tRel, Verdict: VerdictModified, LockedSHA256: tHash, CurrentSHA256: weakHash}}
	if !reflect.DeepEqual(cResults, want) {
		t.Fatalf("C results = %+v", cResults)
	}
	if got := recordHashOf(t, root, tRel); got != weakHash {
		t.Fatalf("D's new record hash = %s, want %s", got, weakHash)
	}

	store.seams.beforeRemove = faultOnCall(2)
	if _, err := store.UnlockAll(); !errors.Is(err, errInjected) {
		t.Fatalf("step 8 err = %v", err)
	}
	if got := listPaths(t, store); len(got) != 1 {
		t.Fatalf("step 8 left %v, want one lock still listed", got)
	}
	store.seams.beforeRemove = nil
	writeFile(t, root, FixLocksDir+"/planted.json", "not json")
	results, err := store.UnlockAll()
	if err != nil || len(results) != 2 {
		t.Fatalf("rerun = %+v, %v", results, err)
	}
	corrupt := UnlockResult{Path: FixLocksDir + "/planted.json", Verdict: VerdictUnverifiable}
	if results[0] != corrupt || results[1].Verdict != VerdictUnchanged {
		t.Fatalf("rerun verdicts = %+v", results)
	}
	if got := listPaths(t, store); len(got) != 0 {
		t.Fatalf("locks after the rerun = %v", got)
	}
}

func TestLock_StoreHeldPastTheWait_FailsBusyAndWritesNothing(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	clock := newClock(t0)
	holder, waiter := openTestStore(t, root, clock), openTestStore(t, root, clock)
	holding, release := make(chan struct{}), make(chan struct{})
	holder.seams.afterPublish = func(string) { close(holding); <-release }
	done := run(func() error { return holder.Lock([]string{tRel}, 0) })
	await(t, holding, "holding")
	waiter.wait = 30 * time.Millisecond
	if err := waiter.Lock([]string{uRel}, 0); !errors.Is(err, ErrStoreBusy) {
		t.Errorf("Lock err = %v, want ErrStoreBusy", err)
	}
	if _, err := waiter.UnlockAll(); !errors.Is(err, ErrStoreBusy) {
		t.Errorf("UnlockAll err = %v, want ErrStoreBusy", err)
	}
	close(release)
	if err := awaitErr(t, done); err != nil {
		t.Fatal(err)
	}
	if got := listPaths(t, waiter); !reflect.DeepEqual(got, []string{tRel}) {
		t.Fatalf("locks = %v", got)
	}
}

// A replaced expired record that a failed batch had overwritten gets its
// previous bytes back.
func TestLock_RollbackRestoresAReplacedExpiredRecord(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	clock := newClock(t0)
	store := openTestStore(t, root, clock)
	if err := store.Lock([]string{tRel}, time.Minute); err != nil {
		t.Fatal(err)
	}
	before := storeFiles(t, root)
	clock.Set(t0.Add(2 * time.Minute))
	writeFile(t, root, tRel, weakContent)
	store.seams.beforePublish = faultOnCall(2)
	if err := store.Lock([]string{tRel, uRel}, 0); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v", err)
	}
	if after := storeFiles(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("store after rollback =\n%v\nwant\n%v", after, before)
	}
	entries, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(FixLocksDir)))
	for _, entry := range entries {
		if entry.Name() != storeLockName && entry.Name()[0] == '.' {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestLock_UnreadableRecordAtTheTargetName_FailsWithoutReplacingIt(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	target, err := Resolve(root, tRel)
	if err != nil {
		t.Fatal(err)
	}
	planted := writeFile(t, root, FixLocksDir+"/"+recordName(target.Key), `{"schema":"autopus.fix_lock.v1"}`)
	if err := store.Lock([]string{tRel}, 0); !errors.Is(err, ErrLockState) {
		t.Fatalf("err = %v, want ErrLockState", err)
	}
	if data, _ := os.ReadFile(planted); string(data) != `{"schema":"autopus.fix_lock.v1"}` {
		t.Fatalf("the unreadable record was replaced: %s", data)
	}
	results, err := store.Unlock([]string{tRel})
	if err != nil || len(results) != 1 || results[0].Verdict != VerdictUnverifiable {
		t.Fatalf("named unlock of a corrupt record = %+v, %v", results, err)
	}
}
