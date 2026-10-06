package editguard

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// S15 (Should) and REQ-EG-22: an expired lock is normal state that the guard
// ignores and the listing reports as stale; re-locking replaces it.
func TestTTL_ExpiredLocksAreIgnoredListedStaleAndReplaced(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, tRel, tContent)
	writeFile(t, root, uRel, "package foo\n")
	clock := newClock(t0)
	store := openTestStore(t, root, clock)
	mustLock(t, store, tRel, skillRel)
	clock.Set(t0.Add(24 * time.Hour))
	mustLock(t, store, uRel)
	clock.Set(t0.Add(25 * time.Hour))
	opts := Options{Now: clock.Now}

	if got := decideOne(root, tRel, opts); got != (Decision{}) {
		t.Errorf("expired lock on T: %+v, want a silent allow", got)
	}
	if got := decideOne(root, uRel, opts); got != denyOf(ClassFixLock, flReason(uRel)) {
		t.Errorf("active lock on u_test.go: %+v", got)
	}
	if got := decideOne(root, skillRel, opts); got != denyOf(ClassGeneratedSurface, gsConReason) {
		t.Errorf("expired lock on the skill: %+v, want the GS-CON deny", got)
	}
	states := map[string]string{}
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		states[entry.Path] = entry.State
	}
	want := map[string]string{tRel: LockStateStale, uRel: LockStateActive, skillRel: LockStateStale}
	if !reflect.DeepEqual(states, want) {
		t.Fatalf("states = %v, want %v", states, want)
	}

	writeFile(t, root, tRel, weakContent)
	mustLock(t, store, tRel)
	if got := recordHashOf(t, root, tRel); got != weakHash {
		t.Errorf("re-locked T hash = %s, want the current hash", got)
	}
	entries, _ = store.List()
	for _, entry := range entries {
		if entry.Path == tRel && (entry.State != LockStateActive || entry.CreatedAt != "2026-10-08T10:00:00Z") {
			t.Errorf("re-locked T = %+v", entry)
		}
	}
}

func TestLock_TTLBounds(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	store := openTestStore(t, root, newClock(t0))
	for _, ttl := range []time.Duration{time.Minute - time.Second, MaxLockTTL + time.Second, -time.Hour} {
		if err := store.Lock([]string{tRel}, ttl); !errors.Is(err, ErrInvalidTTL) {
			t.Errorf("ttl %v: err = %v", ttl, err)
		}
	}
	if err := store.Lock([]string{tRel}, MinLockTTL); err != nil {
		t.Fatal(err)
	}
	if err := store.Lock([]string{uRel}, MaxLockTTL); err != nil {
		t.Fatal(err)
	}
	expiries := map[string]string{}
	entries, _ := store.List()
	for _, entry := range entries {
		expiries[entry.Path] = entry.ExpiresAt
	}
	want := map[string]string{tRel: "2026-10-07T09:01:00Z", uRel: "2026-10-14T09:00:00Z"}
	if !reflect.DeepEqual(expiries, want) {
		t.Fatalf("expiries = %v, want %v", expiries, want)
	}
}

// S17 step 10: after the exit-3 unlock that cleared a corrupt record, the
// guard allows editing T again.
func TestDecide_AfterUnlockAllWithUnverifiableRecord_AllowsT(t *testing.T) {
	t.Parallel()
	root := lockProject(t)
	clock := newClock(t0)
	store := openTestStore(t, root, clock)
	mustLock(t, store, tRel)
	writeFile(t, root, FixLocksDir+"/planted.json", "not json")
	if got := decideOne(root, tRel, Options{Now: clock.Now}); got != denyOf(ClassFixLock, tFLReason) {
		t.Fatalf("before unlock: %+v", got)
	}
	results, err := store.UnlockAll()
	if err != nil || len(results) != 2 || results[0].Verdict != VerdictUnverifiable {
		t.Fatalf("UnlockAll = %+v, %v", results, err)
	}
	if got := decideOne(root, tRel, Options{Now: clock.Now}); got != (Decision{}) {
		t.Fatalf("after unlock: %+v", got)
	}
}
