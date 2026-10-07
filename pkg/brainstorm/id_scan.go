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

// fullWindowTop is the highest number that leaves MaxIDAttempts IDs above it
// in range; an entry above it holds the top of the range.
const fullWindowTop = maxIDNumber - MaxIDAttempts

// idPlan is where one allocation looks for its ID.
type idPlan struct {
	numbers  []int    // IDs to try in order, at most MaxIDAttempts
	ignored  []string // entries the scan ignored
	rangeEnd []string // entries at the top of the range when numbers are the lowest free IDs
}

// plan returns the IDs one allocation tries and the paths to report. A
// regular entry counts as is, so a hand-written BS-BAND-7.md still raises
// the next ID, and an ID that an entry holds is never tried. Only when the
// highest entry holds the top of the range are the entries that fail the
// structural validator skipped, highest first, until a valid BS is found.
// When fewer than MaxIDAttempts free IDs are left above that BS, the
// allocation takes the lowest free IDs instead and names the entries at the
// top of the range, so no planted file, a BS or not, blocks every later BS.
func (s idScan) plan() idPlan {
	sort.SliceStable(s.entries, func(a, b int) bool { return s.entries[a].number > s.entries[b].number })
	plan := idPlan{ignored: append([]string(nil), s.ignored...)}
	held := make(map[int]bool, len(s.entries))
	for _, entry := range s.entries {
		held[entry.number] = true
	}
	base := 0
	if len(s.entries) > 0 && s.entries[0].number <= fullWindowTop {
		base = s.entries[0].number
	} else {
		for _, entry := range s.entries {
			if validBSFile(entry.path, entry.id) {
				base = entry.number
				break
			}
			plan.ignored = append(plan.ignored, entry.path)
		}
	}
	if plan.numbers = freeIDs(held, base+1); len(plan.numbers) == MaxIDAttempts {
		return plan
	}
	plan.numbers = freeIDs(held, 1)
	for _, entry := range s.entries {
		if entry.number > fullWindowTop {
			plan.rangeEnd = append(plan.rangeEnd, entry.path)
		}
	}
	return plan
}

// freeIDs returns the first MaxIDAttempts numbers from from on, up to
// maxIDNumber, that no entry holds. Every skipped number is an entry, so the
// loop ends within MaxIDAttempts plus len(held) steps.
func freeIDs(held map[int]bool, from int) []int {
	var numbers []int
	for number := from; number <= maxIDNumber && len(numbers) < MaxIDAttempts; number++ {
		if !held[number] {
			numbers = append(numbers, number)
		}
	}
	return numbers
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
