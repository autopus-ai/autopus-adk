package rulecond

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"
)

// State bounds fixed by REQ-STICKYRULE-STATE-01. The counter tree is generated
// runtime state, so it is bounded in both age and count rather than growing one
// file per session forever.
//
// @AX:NOTE [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: magic constants — the 200-entry cap, the 7-day retention, and the 2^30 index ceiling are the whole retention policy for `.autopus/runtime/sticky-rules`; there is no other place they are stated.
const (
	// stickyStateMaxEntries caps the state directory.
	stickyStateMaxEntries = 200
	// stickyStateMaxAge is the retention bound for one counter file.
	stickyStateMaxAge = 7 * 24 * time.Hour
	// stickyStateDirPerm and stickyStatePerm keep counter state owner-readable.
	stickyStateDirPerm  = 0o755
	stickyStateFilePerm = 0o600
	// maxStickyIndex bounds a counter read back from disk. The value is
	// repo-writable state, so an absurd or negative index is treated as
	// unusable rather than propagated into the cadence arithmetic.
	maxStickyIndex = 1 << 30
	// maxCounterBytes bounds how much of a counter entry is read. The entry is
	// repo-writable, so its size is chosen by whoever can write the checkout; a
	// decimal index below maxStickyIndex never comes close to this bound.
	maxCounterBytes = 64
	// counterOpenAttempts bounds the counter open when it reports a missing
	// name; see openCounter.
	counterOpenAttempts = 3
)

// StickyStateKey derives the state filename for a session identifier.
//
// The identifier arrives in the untrusted hook payload, so it is never used as a
// path component. A SHA-256 digest makes traversal unrepresentable rather than
// merely rejected: `../../etc/passwd`, an absolute path, and an empty string all
// produce the same fixed-shape 64-character hex name, and the raw identifier
// never reaches disk (REQ-STICKYRULE-FIRE-02, REQ-STICKYRULE-FIRE-04).
//
// The derivation contains the name, not the target. An attacker chooses the
// session identifier and therefore chooses the digest, and a payload that omits
// session_id reaches the digest of the empty string, so the name is predictable
// by construction. What the runtime may do to a file under that name is decided
// by openCounter, never by the name being hard to guess.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: filename derivation is the containment for the state tree — there is no sanitizer to bypass because no input byte survives into the name.
// @AX:REASON: Replacing the digest with a validated identifier restores a path-component injection channel from an attacker-chosen session_id.
func StickyStateKey(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])
}

// nextPromptIndex increments and persists the prompt counter for one session,
// returning the index this invocation observes.
//
// Every failure is whole-run benign: an unusable state directory, a counter
// entry that is not a plain file this runtime could have written, an unreadable
// counter, and a failed write all return ok false, which produces no stdout and
// no stderr (REQ-STICKYRULE-FIRE-08). Sessions never share a file, so two
// concurrent sessions cannot interfere; two concurrent prompts within one
// session are not a case Claude Code produces.
func nextPromptIndex(root, sessionID string) (int, bool) {
	state, ok := openStateRoot(root)
	if !ok {
		return 0, false
	}
	defer func() { _ = state.Close() }()

	index, ok := bumpCounter(state, StickyStateKey(sessionID))
	if !ok {
		return 0, false
	}
	// Pruning runs after the write so the entry this invocation just created is
	// the newest one and therefore survives its own oldest-first cap.
	pruneStickyState(state)
	return index, true
}

// bumpCounter reads the index one counter entry holds and writes the next one
// back through the same descriptor.
//
// One descriptor is the point. The read and the rewrite share it, so there is no
// second name lookup between the two halves for a repo-planted link to land in,
// and the guards openCounter applies are guards on the inode the write actually
// reaches rather than on whatever a name resolved to earlier.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: the write-side leaf guard — the only place this runtime creates or modifies a file.
// @AX:REASON: Replacing this with an os.WriteFile on a joined path sends the write wherever the counter name resolves, which is how a committed symlink at the leaf reached an inode outside the checkout even after the directory components were contained.
func bumpCounter(state *os.Root, name string) (int, bool) {
	file, ok := openCounter(state, name)
	if !ok {
		return 0, false
	}
	defer file.Close()

	previous, ok := readCounter(state, file, name)
	if !ok {
		return 0, false
	}
	next := previous + 1
	if !writeCounter(file, next) {
		return 0, false
	}
	return next, true
}

// openCounter opens one counter entry for reading and writing, creating it when
// the session is new, and admits only a plain file this runtime could have
// written itself.
//
// The state handle is what contains the open. os.Root resolves every component
// against the directory's own descriptor and refuses a name that leaves it, so a
// counter entry committed as a symlink out of the checkout — dangling or not —
// cannot be opened at all, and the containment radius is the state directory
// rather than the project root. A symlink that stays inside the state directory
// is resolved rather than refused, so the Lstat below still runs: this runtime
// owns every name in that directory and none of them is a link.
//
// A caller-supplied syscall.O_NOFOLLOW would be decoration here. os.Root
// resolves the final component itself, so the flag never reaches the underlying
// open and the refusal above is structural rather than flag-driven.
//
// Link count is the other half, and it is the half no name resolution can
// supply. A hardlink is the same inode under a second directory entry, so the
// path stays inside the state directory while the bytes also live outside it;
// the stat is taken from the open descriptor, which is the inode the write would
// land on.
//
// The open is retried when it reports a missing name. On darwin an os.Root
// (openat) O_CREAT open that races another creator of the same name can fail
// with ENOENT although the directory handle is live, and a counter name is
// shared: every payload without a session_id reaches the digest of the empty
// string. By the next attempt the name exists, so that open no longer creates
// (the same race SPEC-EDITGUARD-001 T15 measured on the edit guard's store
// lock). Every attempt goes through the same handle, and the checks below
// apply to the descriptor the open returns.
func openCounter(state *os.Root, name string) (*os.File, bool) {
	if info, err := state.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return nil, false
	}

	file, err := state.OpenFile(name, os.O_RDWR|os.O_CREATE, stickyStateFilePerm)
	for attempt := 1; errors.Is(err, fs.ErrNotExist) && attempt < counterOpenAttempts; attempt++ {
		file, err = state.OpenFile(name, os.O_RDWR|os.O_CREATE, stickyStateFilePerm)
	}
	if err != nil {
		return nil, false
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || hasMultipleLinks(info) {
		file.Close()
		return nil, false
	}
	return file, true
}

// readCounter reads the index a counter entry holds. A newly created entry is
// empty and reads as zero, which is the first prompt of a session. An entry that
// holds anything other than a usable decimal index is unusable state: it is
// removed so the next prompt of that session starts a fresh counter, and this
// invocation stays benign rather than injecting at a fabricated index.
func readCounter(state *os.Root, file *os.File, name string) (int, bool) {
	raw, err := io.ReadAll(io.LimitReader(file, maxCounterBytes))
	if err != nil {
		return 0, false
	}

	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0, true
	}
	value, convErr := strconv.Atoi(text)
	if convErr != nil || value < 0 || value >= maxStickyIndex {
		// Removal goes through the same handle, so it can only unlink a name
		// inside the state directory.
		_ = state.Remove(name)
		return 0, false
	}
	return value, true
}

// writeCounter replaces the counter entry's contents with one decimal index.
// The descriptor is truncated first so a shorter index cannot leave a suffix of
// the previous one behind.
func writeCounter(file *os.File, index int) bool {
	if err := file.Truncate(0); err != nil {
		return false
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return false
	}
	_, err := file.WriteString(strconv.Itoa(index) + "\n")
	return err == nil
}
