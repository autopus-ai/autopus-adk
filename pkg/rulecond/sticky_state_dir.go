package rulecond

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// stickyStateKeyPattern is the shape StickyStateKey produces. Pruning is
// restricted to names matching it, so the sticky runtime can never remove a file
// it did not create.
var stickyStateKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// openStateRoot opens the counter state directory under projectRoot as a
// directory handle, creating the path when it is absent.
//
// The handle is the containment frame, and it is built one component at a time
// rather than by resolving a path string. Re-checking a joined path after
// creating it only proves what the name meant at that instant: every later
// syscall resolves the same name again from scratch, so a component swapped
// afterwards is followed by the write rather than by the check. Each descent
// below is relative to the parent's own descriptor, so a component that changes
// afterwards cannot retarget a descriptor already held, and os.Root refuses any
// name that leaves the directory it was opened on.
//
// A cloned repository can commit `.autopus`, `.autopus/runtime`, or
// `sticky-rules` itself as a symlink, because git mode 120000 survives a clone,
// and a followed link would put counter files inside the link target and hand
// pruneStickyState a delete loop rooted outside the checkout. Every refusal
// returns false, which nextPromptIndex reports as the whole-run benign "unusable
// state directory" case of REQ-STICKYRULE-FIRE-08: nothing is created, nothing
// is pruned, and neither stdout nor stderr is written.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: the write-side containment frame — every counter open and every retention delete goes through the handle this returns.
// @AX:REASON: Returning a path string instead of a handle reinstates name re-resolution on every later syscall, which is the seam a concurrent component swap used to land a write inside a link target.
func openStateRoot(projectRoot string) (*os.Root, bool) {
	state, err := openStateDir(projectRoot, StickyStateDirRelPath, true)
	return state, err == nil
}

// runtimeStateDirRelPath is the parent of every named runtime state directory.
const runtimeStateDirRelPath = ".autopus/runtime"

// errUnusableStateComponent reports a state path component that exists but is
// not a real directory.
var errUnusableStateComponent = errors.New("rulecond: state path component is not a directory")

// OpenRuntimeStateDir opens `.autopus/runtime/<name>` below projectRoot as a
// directory handle through the same component-wise descent as the sticky
// counter state, creating absent components. A symlinked or non-directory
// component is refused, never followed (SPEC-EDITGUARD-001 REQ-EG-09).
func OpenRuntimeStateDir(projectRoot, name string) (*os.Root, error) {
	return openRuntimeStateDir(projectRoot, name, true)
}

// LookupRuntimeStateDir is OpenRuntimeStateDir for readers: it creates
// nothing, and an absent component yields an error wrapping fs.ErrNotExist so a
// reader can tell "no state yet" from unusable state.
func LookupRuntimeStateDir(projectRoot, name string) (*os.Root, error) {
	return openRuntimeStateDir(projectRoot, name, false)
}

func openRuntimeStateDir(projectRoot, name string, create bool) (*os.Root, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return nil, errors.New("rulecond: invalid runtime state directory name")
	}
	return openStateDir(projectRoot, runtimeStateDirRelPath+"/"+name, create)
}

// openStateDir descends the slash-separated relative path below projectRoot
// one component at a time, each step relative to the parent's descriptor.
func openStateDir(projectRoot, relative string, create bool) (*os.Root, error) {
	// The project root is the trust anchor and is the one path here that may
	// legitimately sit behind a symlink, because the system prefix above a
	// checkout routinely does. See trustedroot.go for that contract.
	base, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return nil, err
	}
	current, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}

	for _, component := range strings.Split(relative, "/") {
		if component == "" {
			continue
		}
		next, err := descendStateComponent(current, component, create)
		_ = current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

// descendStateComponent returns a handle to one component of the state path,
// creating it first when create is set and it is absent.
//
// Anything already at that name which is not a real directory — a symlink, a
// regular file, a device node — is refused rather than followed, which is the
// same rule containedProjectPath applies on the read side. The Lstat and the
// open are both relative to the parent descriptor, so neither one re-resolves
// the components above it.
func descendStateComponent(parent *os.Root, component string, create bool) (*os.Root, error) {
	if create {
		if err := parent.Mkdir(component, stickyStateDirPerm); err != nil &&
			!errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	info, err := parent.Lstat(component)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsDir() {
		return nil, errUnusableStateComponent
	}
	return parent.OpenRoot(component)
}

// pruneStickyState enforces the REQ-STICKYRULE-STATE-01 bounds: entries older
// than stickyStateMaxAge are deleted, and the directory is then capped at
// stickyStateMaxEntries by removing the oldest first. Pruning is best effort and
// silent; a failure to remove one entry never affects the injection itself.
//
// state MUST come from openStateRoot. That is what bounds the sweep: every
// lookup and every delete below is relative to that directory's own descriptor,
// so a name cannot resolve out of it and the loop can never delete inside a link
// target. Real trees hold exactly the names this loop matches — `.git/lfs/objects`
// and container blob stores are directories of 64-character hex filenames.
//
// @AX:WARN [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: unconditional delete loop over a directory a repository can populate.
// @AX:REASON: Three things keep this from deleting files it did not create — the openStateRoot handle, the stickyStateKeyPattern match on each entry name, and the Lstat regular-file guard on each entry; dropping any one turns a retention sweep into data loss under a repo-controlled path.
func pruneStickyState(state *os.Root) {
	entries, ok := readStateEntries(state)
	if !ok {
		return
	}

	type stateEntry struct {
		name    string
		modTime time.Time
	}
	cutoff := time.Now().Add(-stickyStateMaxAge)
	kept := make([]stateEntry, 0, len(entries))

	for _, entry := range entries {
		if !stickyStateKeyPattern.MatchString(entry.Name()) {
			continue
		}
		// Lstat rather than Stat, and a regular-file check rather than a bare
		// directory check: a digest-named symlink planted in the state directory
		// is neither state this runtime created nor something it may follow.
		info, infoErr := state.Lstat(entry.Name())
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = state.Remove(entry.Name())
			continue
		}
		kept = append(kept, stateEntry{name: entry.Name(), modTime: info.ModTime()})
	}

	if len(kept) <= stickyStateMaxEntries {
		return
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].modTime.Before(kept[j].modTime) })
	for _, entry := range kept[:len(kept)-stickyStateMaxEntries] {
		_ = state.Remove(entry.name)
	}
}

// readStateEntries lists the state directory through its own descriptor. os.Root
// exposes no ReadDir, so the directory is opened as itself and read from the
// descriptor rather than re-opened by path.
func readStateEntries(state *os.Root) ([]os.DirEntry, bool) {
	dir, err := state.Open(".")
	if err != nil {
		return nil, false
	}
	defer dir.Close()

	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, false
	}
	return entries, true
}
