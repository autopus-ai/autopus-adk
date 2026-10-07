package brainstorm

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// idEntry is one regular BS-BAND file of the scan; id is its file name
// without .md.
type idEntry struct {
	number int
	id     string
	path   string
}

// idScan is every BS-BAND file name across the scan roots of a scope.
type idScan struct {
	entries []idEntry // regular files
	ignored []string  // symlinks and other non-regular entries
}

// base returns the number the next ID goes above and the ignored paths. A
// regular entry counts as is, so a hand-written BS-BAND-7.md still raises
// the next ID. Only when the highest entry leaves no ID in range are the
// entries that fail the structural validator skipped, highest first, until
// a valid BS is found: one planted BS-BAND-999999999.md cannot block every
// later BS, while a real BS at the top still exhausts the range.
func (s idScan) base() (int, []string) {
	ignored := append([]string(nil), s.ignored...)
	sort.SliceStable(s.entries, func(a, b int) bool { return s.entries[a].number > s.entries[b].number })
	if len(s.entries) == 0 {
		return 0, ignored
	}
	if s.entries[0].number < maxIDNumber {
		return s.entries[0].number, ignored
	}
	for _, entry := range s.entries {
		if validBSFile(entry.path, entry.id) {
			return entry.number, ignored
		}
		ignored = append(ignored, entry.path)
	}
	return 0, ignored
}

// validBSFile reports whether path is a regular file of at most the BS body
// bound that passes Validate and names its own ID in line 1.
func validBSFile(path, id string) bool {
	file, err := healthband.OpenRegular(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, healthband.MaxBSBodyBytes+1))
	return err == nil && len(content) <= healthband.MaxBSBodyBytes && len(Validate(content)) == 0 &&
		bytes.HasPrefix(content, []byte("# "+id+": "))
}

// scanIDs reads the scan roots of the scope: <dir>/.autopus/brainstorms and
// <dir>/*/.autopus/brainstorms for every scope directory (BS Root
// Resolution item 2), each read once.
func scanIDs(scope Scope) (idScan, error) {
	var scan idScan
	scanned := map[string]bool{}
	for _, dir := range scope.Dirs {
		roots := []string{filepath.Join(dir, ".autopus", "brainstorms")}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return idScan{}, err
		}
		for _, entry := range entries {
			roots = append(roots, filepath.Join(dir, entry.Name(), ".autopus", "brainstorms"))
		}
		for _, root := range roots {
			if scanned[root] {
				continue
			}
			scanned[root] = true
			if err := scan.addRoot(root); err != nil {
				return idScan{}, err
			}
		}
	}
	return scan, nil
}

// addRoot reads one scan root. A missing root, a path through a file, or a
// directory this user may not read holds no IDs this user allocated. An
// entry named like a BS that is a symlink or not a regular file is ignored.
func (s *idScan) addRoot(root string) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.Is(err, fs.ErrPermission) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		match := idFileName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if !entry.Type().IsRegular() {
			s.ignored = append(s.ignored, path)
			continue
		}
		number, _ := strconv.Atoi(match[1])
		s.entries = append(s.entries, idEntry{number: number, id: strings.TrimSuffix(entry.Name(), ".md"), path: path})
	}
	return nil
}
