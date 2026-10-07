package editguard

import (
	"reflect"
	"testing"
)

// m2: once a record is linked into place the lock is published. A temp file
// that then cannot be removed is skipped by every reader, so Lock reports the
// published locks instead of failing with a record left behind.
func TestLock_TempCleanupFailureAfterLink_KeepsThePublishedLocks(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	store.seams.cleanTemp = func(string) error { return errInjected }
	if err := store.Lock([]string{tRel, uRel}, 0); err != nil {
		t.Fatalf("Lock = %v, want the published locks reported", err)
	}
	if got, want := listPaths(t, store), []string{tRel, uRel}; !reflect.DeepEqual(got, want) {
		t.Fatalf("locks = %q, want %q", got, want)
	}
	if got := decideOne(root, tRel, Options{Now: newClock(t0).Now}); got != denyOf(ClassFixLock, tFLReason) {
		t.Fatalf("Decide(T) = %+v, want the FL deny", got)
	}
}
