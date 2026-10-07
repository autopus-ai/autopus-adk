package editguard

import (
	"sync"
	"testing"
)

// S17 step 4 on a store that does not exist yet (T15 finding). The first
// lockers race to create the store lock file, and on darwin an os.Root
// (openat) O_CREAT open that races another creator of the same name can fail
// with ENOENT although the directory handle is live. Before the fix this
// surfaced as exit 1 "lock state is unusable" from concurrent `auto fix lock`
// processes: 7 of 30 rounds of 8 processes, and 48 of 60 rounds of 8
// goroutines, had a failed locker.
func TestLock_ConcurrentFirstLocksOfAFreshStore_AllSucceed(t *testing.T) {
	t.Parallel()
	const rounds, lockers = 10, 8
	for round := range rounds {
		root := lockProject(t)
		clock := newClock(t0)
		stores := make([]*Store, lockers)
		for i := range stores {
			stores[i] = openTestStore(t, root, clock)
		}
		errs := make([]error, lockers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, store := range stores {
			wg.Go(func() {
				<-start
				errs[i] = store.Lock([]string{tRel}, 0)
			})
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, locker %d of a fresh store: %v", round, i, err)
			}
		}
		if got := listPaths(t, stores[0]); len(got) != 1 || got[0] != tRel {
			t.Fatalf("round %d: locks %v, want exactly T", round, got)
		}
		if got := recordHashOf(t, root, tRel); got != tHash {
			t.Fatalf("round %d: T's record hash %s, want %s", round, got, tHash)
		}
	}
}
