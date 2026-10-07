package intake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Path and platform reasons shared by intake, promote, reject, and prune.
const (
	ReasonCandidateIDInvalid  = "candidate_id_invalid"
	ReasonPathUnsafe          = "path_unsafe"
	ReasonPlatformUnsupported = "platform_unsupported"
)

// Repository-relative directories of the intake area and of the surface
// active task directory. All of them sit below the quarantine path that
// SPEC-HARNEVAL-001 never evaluates, except SurfaceTaskDir.
const (
	IntakeDir      = harneval.QuarantinePath
	PromotedDir    = harneval.QuarantinePath + "/promoted"
	RejectedDir    = harneval.QuarantinePath + "/rejected"
	SurfaceTaskDir = harneval.EvalRoot + "/tasks/surface"
)

var (
	errPathUnsafe = errors.New(ReasonPathUnsafe)

	candidateIDPattern = regexp.MustCompile(`^GTC-[0-9a-f]{12}$`)
	// taskIDPattern copies the SPEC-HARNEVAL-001 task id grammar, which that
	// package keeps unexported; a drift test compares it with DecodeTask.
	taskIDPattern = regexp.MustCompile(`^GT-[A-Z][A-Z0-9-]{2,40}$`)
)

// layoutDirs are checked component by component before any intake-area use.
var layoutDirs = []string{IntakeDir, PromotedDir, RejectedDir, SurfaceTaskDir}

// maxRecordBytes caps every file the intake area reads, matching the
// SPEC-HARNEVAL-001 document cap.
const maxRecordBytes = 64 << 20

// ValidCandidateID reports whether id is a candidate id. It is checked before
// an id is ever joined into a path.
func ValidCandidateID(id string) bool { return candidateIDPattern.MatchString(id) }

// ValidTaskID reports whether id fits the SPEC-HARNEVAL-001 task id grammar.
func ValidTaskID(id string) bool { return taskIDPattern.MatchString(id) }

func unsafef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errPathUnsafe, fmt.Sprintf(format, args...))
}

// area confines every intake-area operation to an os.Root opened at the
// project root. A path is checked with Root.Lstat one component at a time,
// so a symlink, junction, or non-directory component is refused even when it
// would resolve inside the root; the Root itself keeps a component swapped in
// after the check from reaching outside the project.
type area struct {
	root *os.Root
}

func openArea(projectRoot string) (*area, error) {
	root, err := os.OpenRoot(projectRoot)
	if err != nil {
		return nil, err
	}
	return &area{root: root}, nil
}

func (a *area) close() error { return a.root.Close() }

// isCleanRel reports whether p is a non-empty slash-separated relative path
// that path.Clean leaves unchanged and that stays below the root.
func isCleanRel(p string) bool {
	if p == "" || p == "." || strings.ContainsAny(p, "\\\x00") || path.IsAbs(p) {
		return false
	}
	return path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

// lstatChain checks rel one component at a time without following links:
// every inner component must be a real directory, and the last one a real
// directory when wantDir is set or a regular file otherwise. A missing
// component returns an error matching fs.ErrNotExist.
func (a *area) lstatChain(rel string, wantDir bool) (fs.FileInfo, error) {
	if !isCleanRel(rel) {
		return nil, unsafef("%q is not a clean relative path", rel)
	}
	parts := strings.Split(rel, "/")
	var info fs.FileInfo
	for index := range parts {
		name := strings.Join(parts[:index+1], "/")
		var err error
		if info, err = a.root.Lstat(name); err != nil {
			return nil, err
		}
		last := index == len(parts)-1
		if (!last || wantDir) && info.Mode().Type() != fs.ModeDir {
			return nil, unsafef("%s is not a real directory", name)
		}
		if last && !wantDir && !info.Mode().IsRegular() {
			return nil, unsafef("%s is not a regular file", name)
		}
	}
	return info, nil
}

// dirExists reports whether rel is a real directory; a missing component is
// not an error, an unsafe one is.
func (a *area) dirExists(rel string) (bool, error) {
	_, err := a.lstatChain(rel, true)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// checkLayout refuses an intake area or surface task directory that has a
// symlink or non-directory anywhere on its path.
func (a *area) checkLayout() error {
	for _, dir := range layoutDirs {
		if _, err := a.dirExists(dir); err != nil {
			return err
		}
	}
	return nil
}

// open opens a checked path and confirms it is still the file that was
// checked.
func (a *area) open(rel string, checked fs.FileInfo) (*os.File, error) {
	file, err := a.root.Open(rel)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil && !os.SameFile(checked, opened) {
		err = unsafef("%s changed while it was opened", rel)
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// readFile reads the regular file rel, refusing an unsafe path and any file
// over maxRecordBytes.
func (a *area) readFile(rel string) ([]byte, error) {
	info, err := a.lstatChain(rel, false)
	if err != nil {
		return nil, err
	}
	file, err := a.open(rel, info)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRecordBytes+1))
	if err == nil && len(data) > maxRecordBytes {
		err = fmt.Errorf("%s is larger than %d bytes", rel, maxRecordBytes)
	}
	return data, err
}

// listDir returns the entries of the real directory rel sorted by name; a
// missing directory has no entries.
func (a *area) listDir(rel string) ([]fs.DirEntry, error) {
	info, err := a.lstatChain(rel, true)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dir, err := a.open(rel, info)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, err
}

// walkJSON visits every regular *.json file below rel in lexical order, the
// files the SPEC-HARNEVAL-001 loader reads as tasks. A symlink, a junction, or
// a non-regular *.json entry anywhere below rel is refused.
func (a *area) walkJSON(rel string, visit func(rel string, data []byte) error) error {
	entries, err := a.listDir(rel)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child, kind := rel+"/"+entry.Name(), entry.Type()
		isJSON := strings.HasSuffix(entry.Name(), ".json")
		switch {
		case kind == fs.ModeDir:
			err = a.walkJSON(child, visit)
		case kind.IsRegular() && isJSON:
			var data []byte
			if data, err = a.readFile(child); err == nil {
				err = visit(child, data)
			}
		case kind&(fs.ModeSymlink|fs.ModeDir|fs.ModeIrregular) != 0 || isJSON:
			err = unsafef("%s is not a regular file or a real directory", child)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// activePaths reads only active_paths from the manifest, which must stay
// clean, below the eval root, and outside the intake area. A missing manifest
// declares no active path.
func (a *area) activePaths() ([]string, error) {
	data, err := a.readFile(harneval.ManifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var manifest struct {
		ActivePaths []string `json:"active_paths"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode %s: %w", harneval.ManifestPath, err)
	}
	for _, active := range manifest.ActivePaths {
		inIntake := active == IntakeDir || strings.HasPrefix(active, IntakeDir+"/")
		if !isCleanRel(active) || !strings.HasPrefix(active, harneval.EvalRoot+"/") || inIntake {
			return nil, unsafef("active path %q must be a clean path below %s/ outside the intake area", active, harneval.EvalRoot)
		}
	}
	return manifest.ActivePaths, nil
}
