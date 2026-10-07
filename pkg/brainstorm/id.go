package brainstorm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Allocation limits (REQ-13, BS Root Resolution item 4).
const (
	LockWait      = 30 * time.Second // per-user allocation lock wait
	MaxIDAttempts = 5                // consecutive IDs tried before bs_id_exhausted
	maxIDNumber   = 999_999_999      // BS-BAND IDs carry at most 9 digits
)

// Lock file names: per-user under the user cache directory, and the
// fallback inside the BS root when no user cache directory is usable.
const (
	lockName         = "bs-band.lock"
	fallbackLockName = ".bs-band.lock"
)

// Reason codes of Write errors that healthband does not define.
const (
	ReasonScopeTooDeep   = "bs_scope_too_deep"
	ReasonInvalidRequest = "bs_invalid_request"
	ReasonInvalidBS      = "bs_invalid"
	ReasonWriteFailed    = "bs_write_failed"
)

var (
	// ErrLockTimeout reports that the per-user allocation lock stayed busy.
	ErrLockTimeout = errors.New("brainstorm: BS-BAND allocation lock wait limit reached")
	// ErrIDExhausted reports MaxIDAttempts consecutive create collisions.
	ErrIDExhausted = errors.New("brainstorm: BS-BAND IDs exhausted")
	// ErrInvalidBS reports a rendered BS that failed Validate.
	ErrInvalidBS = errors.New("brainstorm: rendered BS failed the structural validator")
	errUnsafeDir = errors.New("brainstorm: .autopus/brainstorms must be a directory, not a symlink")
)

// idFileName matches an allocated ID; any 1–9 digit number counts, so a
// hand-written BS-BAND-7.md still raises the next ID.
var idFileName = regexp.MustCompile(`^BS-BAND-([0-9]{1,9})\.md$`)

// Options configures Write.
type Options struct {
	// CacheDir returns the per-user cache directory; nil uses os.UserCacheDir.
	CacheDir func() (string, error)
	// LockWait bounds the allocation lock wait; zero uses LockWait.
	LockWait time.Duration
	// beforeCreate runs after the scan and before each exclusive create.
	beforeCreate func(path string)
}

// Result names the BS file Write created.
type Result struct {
	ID   string
	Path string
}

// Reason maps a Write error to its reason code; nil maps to "".
func Reason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrLockTimeout):
		return healthband.ReasonBSLockTimeout
	case errors.Is(err, ErrIDExhausted):
		return healthband.ReasonBSIDExhausted
	case errors.Is(err, ErrScopeTooDeep):
		return ReasonScopeTooDeep
	case errors.Is(err, ErrInvalidRequest):
		return ReasonInvalidRequest
	case errors.Is(err, ErrInvalidBS):
		return ReasonInvalidBS
	}
	return ReasonWriteFailed
}

// Write allocates the next BS-BAND ID of projectDir's scope and creates the
// BS in projectDir/.autopus/brainstorms/ (REQ-13). Under the per-user lock it
// scans every scan root of the scope, renders and validates the BS for one
// above the highest ID, and creates it exclusively; a collision moves to the
// next ID, at most MaxIDAttempts times. Nothing is overwritten, and no lock
// file is created inside a repository unless no user cache dir is usable.
func Write(ctx context.Context, projectDir string, req Request, opts Options) (Result, error) {
	if err := req.validate(); err != nil {
		return Result{}, err
	}
	start, err := filepath.Abs(projectDir)
	if err != nil {
		return Result{}, err
	}
	scope, err := ResolveScope(start)
	if err != nil {
		return Result{}, err
	}
	dir := filepath.Join(start, ".autopus", "brainstorms")
	if err := realDirOrMissing(filepath.Dir(dir), dir); err != nil {
		return Result{}, err
	}
	lock, err := acquire(ctx, scope.Root, opts)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = lock.Unlock() }()
	highest, err := highestID(scope)
	if err != nil {
		return Result{}, err
	}
	return createNext(dir, highest, req, opts.beforeCreate)
}

// acquire takes the allocation lock; a busy lock past the wait, or a
// deadline reached while waiting, is ErrLockTimeout.
func acquire(ctx context.Context, root string, opts Options) (*filelock.Lock, error) {
	wait := opts.LockWait
	if wait <= 0 {
		wait = LockWait
	}
	lock, err := filelock.Acquire(ctx, lockPath(root, opts.CacheDir), wait)
	if errors.Is(err, filelock.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("%w: %w", ErrLockTimeout, err)
	}
	return lock, err
}

// createNext renders, validates, and exclusively creates the BS for the
// first free ID above highest, trying at most MaxIDAttempts IDs.
func createNext(dir string, highest int, req Request, beforeCreate func(string)) (Result, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	for number := highest + 1; number <= highest+MaxIDAttempts && number <= maxIDNumber; number++ {
		id := fmt.Sprintf("BS-BAND-%03d", number)
		content, err := Render(id, req)
		if err != nil {
			return Result{}, err
		}
		if problems := Validate(content); len(problems) > 0 {
			return Result{}, fmt.Errorf("%w: %s", ErrInvalidBS, strings.Join(problems, ","))
		}
		path := filepath.Join(dir, id+".md")
		if beforeCreate != nil {
			beforeCreate(path)
		}
		if err := createExclusive(path, content); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return Result{}, err
		}
		return Result{ID: id, Path: path}, nil
	}
	return Result{}, ErrIDExhausted
}

// lockPath is <user cache dir>/autopus/bs-band.lock, or the fallback in the
// BS root's brainstorms directory when no absolute cache dir is available.
func lockPath(root string, cacheDir func() (string, error)) string {
	if cacheDir == nil {
		cacheDir = os.UserCacheDir
	}
	if dir, err := cacheDir(); err == nil && filepath.IsAbs(dir) {
		return filepath.Join(dir, "autopus", lockName)
	}
	return filepath.Join(root, ".autopus", "brainstorms", fallbackLockName)
}

// realDirOrMissing refuses a path that exists as anything but a real
// directory, so a symlink cannot redirect the BS outside the project.
func realDirOrMissing(paths ...string) error {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errUnsafeDir
		}
	}
	return nil
}

// highestID returns the highest BS-BAND number across the scan roots of the
// scope: <dir>/.autopus/brainstorms and <dir>/*/.autopus/brainstorms for
// every scope directory (BS Root Resolution item 2), each read once.
func highestID(scope Scope) (int, error) {
	highest := 0
	scanned := map[string]bool{}
	for _, dir := range scope.Dirs {
		roots := []string{filepath.Join(dir, ".autopus", "brainstorms")}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return 0, err
		}
		for _, entry := range entries {
			roots = append(roots, filepath.Join(dir, entry.Name(), ".autopus", "brainstorms"))
		}
		for _, root := range roots {
			if scanned[root] {
				continue
			}
			scanned[root] = true
			found, err := highestIn(root)
			if err != nil {
				return 0, err
			}
			highest = max(highest, found)
		}
	}
	return highest, nil
}

// highestIn reads one scan root. A missing root, a path through a file, or
// a directory this user may not read holds no IDs this user allocated.
func highestIn(root string) (int, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, fs.ErrPermission) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, entry := range entries {
		if match := idFileName.FindStringSubmatch(entry.Name()); match != nil {
			number, _ := strconv.Atoi(match[1])
			highest = max(highest, number)
		}
	}
	return highest, nil
}

// createExclusive creates path only if it does not exist (O_EXCL also
// refuses a planted symlink) and removes a partly written file on failure.
func createExclusive(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}
