package diskspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Root kinds as refusal lines name them.
const (
	KindCache   = "cache"
	KindWorkDir = "work dir"
)

// Reasons a consumer is unavailable or a root is refused, as JSON reason
// codes. A refused root exists but must not be trimmed (REQ-DISK-TRIM-06).
const (
	ReasonGOCACHEOff        = "gocache_off"
	ReasonRootMissing       = "root_missing"
	ReasonUnreadable        = "unreadable"
	ReasonRelativeRoot      = "relative_root"
	ReasonNotADirectory     = "not_a_directory"
	ReasonFilesystemRoot    = "filesystem_root"
	ReasonHomeDirectory     = "home_directory"
	ReasonProjectDirectory  = "project_directory"
	ReasonNotOwned          = "not_owned"
	ReasonNotAGoBuildCache  = "not_a_go_build_cache"
	ReasonRootChanged       = "root_changed"
	ReasonNotAGitRepository = "not_a_git_repository"
	ReasonGitUnavailable    = "git_unavailable"
)

// goEnvTimeout bounds `go env GOCACHE` (REQ-DISK-STATUS-02).
const goEnvTimeout = 5 * time.Second

// Root is a resolved consumer root, or why it cannot be read or trimmed.
type Root struct {
	Kind string
	Path string
	// ID is the root's identity at scan time; removal re-verifies it.
	ID      FileID
	Reason  string
	Message string
	// Refused marks a root that exists but is unacceptable: a trim then
	// deletes nothing from either root and exits 1.
	Refused bool
}

// Available reports whether the consumer can be read.
func (r Root) Available() bool { return r.Reason == "" }

// Locator holds the process facts root resolution checks against.
type Locator struct {
	Run        Runner
	Home       string
	ProjectDir string
	UID        int
}

// Cache resolves the Go build cache as REQ-DISK-STATUS-02 orders it: $GOCACHE
// when set, otherwise `go env GOCACHE`, otherwise os.UserCacheDir()/go-build.
// `go env` runs only when $GOCACHE is unset (REQ-DISK-TRIM-07).
func (l Locator) Cache(ctx context.Context) Root {
	raw := os.Getenv("GOCACHE")
	if raw == "" {
		raw = goEnvCache(ctx, l.Run)
	}
	if raw == "" {
		if dir, err := os.UserCacheDir(); err == nil {
			raw = filepath.Join(dir, "go-build")
		}
	}
	if raw == "off" {
		return Root{Kind: KindCache, Reason: ReasonGOCACHEOff, Message: "GOCACHE=off"}
	}
	return l.inspect(KindCache, raw)
}

// WorkDir resolves where go build creates its work directories: $GOTMPDIR
// when set, otherwise os.TempDir(). The go command creates them with
// os.MkdirTemp(GOTMPDIR, "go-build").
func (l Locator) WorkDir() Root {
	raw := os.Getenv("GOTMPDIR")
	if raw == "" {
		raw = os.TempDir()
	}
	return l.inspect(KindWorkDir, raw)
}

// goEnvCache asks the go command. It runs in the temp directory so that a
// go.mod naming a newer toolchain cannot make it download one; GOCACHE comes
// from the environment and go.env, never from a module.
func goEnvCache(ctx context.Context, run Runner) string {
	ctx, cancel := context.WithTimeout(ctx, goEnvTimeout)
	defer cancel()
	out, err := run(ctx, os.TempDir(), "go", "env", "GOCACHE")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// inspect canonicalizes a root and applies the refusals both roots share
// (REQ-DISK-TRIM-06, REQ-DISK-TRIM-10); only a cache root needs the README
// marker Go writes when it creates a cache.
func (l Locator) inspect(kind, raw string) Root {
	if raw == "" {
		return Root{Kind: kind, Reason: ReasonRootMissing, Message: "no " + kind + " root could be resolved"}
	}
	if !filepath.IsAbs(raw) {
		return l.refuse(Root{Kind: kind, Path: raw}, ReasonRelativeRoot, "relative path")
	}
	canonical, err := filepath.EvalSymlinks(raw)
	if errors.Is(err, fs.ErrNotExist) {
		return Root{Kind: kind, Path: raw, Reason: ReasonRootMissing, Message: fmt.Sprintf("%s root %s does not exist", kind, raw)}
	}
	if err != nil {
		return Root{Kind: kind, Path: raw, Reason: ReasonUnreadable, Message: fmt.Sprintf("%s root %s cannot be resolved: %v", kind, raw, err)}
	}
	root := Root{Kind: kind, Path: canonical}
	info, err := os.Lstat(canonical)
	if err != nil {
		return Root{Kind: kind, Path: canonical, Reason: ReasonUnreadable, Message: fmt.Sprintf("%s root %s cannot be read: %v", kind, canonical, err)}
	}
	st := statOf(info)
	root.ID = st.id
	switch {
	case !info.IsDir():
		return l.refuse(root, ReasonNotADirectory, "not a directory")
	case canonical == filepath.Dir(canonical):
		return l.refuse(root, ReasonFilesystemRoot, "file system root")
	case l.Home != "" && canonical == canonicalOrSelf(l.Home):
		return l.refuse(root, ReasonHomeDirectory, "home directory")
	case l.ProjectDir != "" && canonical == canonicalOrSelf(l.ProjectDir):
		return l.refuse(root, ReasonProjectDirectory, "project directory")
	case st.known && st.uid != l.UID:
		return l.refuse(root, ReasonNotOwned, "not owned by uid "+strconv.Itoa(l.UID))
	}
	if kind == KindCache {
		if marker, err := os.Lstat(filepath.Join(canonical, "README")); err != nil || !marker.Mode().IsRegular() {
			return l.refuse(root, ReasonNotAGoBuildCache, "no README marker")
		}
	}
	return root
}

func (l Locator) refuse(root Root, reason, why string) Root {
	root.Reason, root.Refused = reason, true
	root.Message = fmt.Sprintf("%s is not an acceptable %s root (%s)", root.Path, root.Kind, why)
	return root
}

func canonicalOrSelf(path string) string {
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		return canonical
	}
	return filepath.Clean(path)
}
