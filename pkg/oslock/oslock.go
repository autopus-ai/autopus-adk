// Package oslock takes and releases a non-blocking exclusive OS file lock:
// flock on darwin and linux, LockFileEx on windows. The OS releases the lock
// when the holding descriptor closes, including when its process dies, so a
// holder needs no stale-lock timeout. Every other platform fails closed with an
// error wrapping errors.ErrUnsupported instead of acting as a no-op lock.
//
// The helpers moved here unchanged from pkg/companionmanifest
// (SPEC-EDITGUARD-001 T4), which keeps calling them for its signed-pair
// transaction lock.
package oslock
