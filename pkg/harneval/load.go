package harneval

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ErrBaselineMissing is the baseline_missing precondition: the committed
// baseline file is absent or holds no active surface row.
var ErrBaselineMissing = errors.New(ReasonBaselineMissing)

// Set is a strictly loaded golden set. Tasks are sorted by id; CodexCatalog
// holds the pinned catalog bytes, or nil when the pin selects no catalog.
type Set struct {
	Manifest     Manifest
	Tasks        []Task
	CodexCatalog []byte
}

// ActiveCount counts the active tasks of one kind.
func (s *Set) ActiveCount(kind string) int {
	count := 0
	for _, task := range s.Tasks {
		if task.Kind == kind && task.Status.State == StateActive {
			count++
		}
	}
	return count
}

// LoadSet strictly loads the manifest under root and every task file below its
// active paths. Nothing outside the active paths is read, so the quarantine
// area never reaches evaluation. Any defect yields an *InvalidError.
func LoadSet(root string) (*Set, error) {
	data, err := readRepoFile(root, ManifestPath)
	if err != nil {
		return nil, withPath(err, ManifestPath)
	}
	manifest, err := DecodeManifest(data)
	if err != nil {
		return nil, withPath(err, ManifestPath)
	}
	set := &Set{Manifest: manifest}
	if catalogPath := manifest.Pins.CodexModelCatalog; catalogPath != "" {
		catalog, err := readRepoFile(root, catalogPath)
		if err != nil {
			return nil, withPath(err, catalogPath)
		}
		if _, err := config.ParseCodexModelCatalog(catalog); err != nil {
			return nil, withPath(&InvalidError{Detail: DetailFieldInvalid, Err: err}, catalogPath)
		}
		set.CodexCatalog = catalog
	}
	loader := taskLoader{root: root, seen: map[string]string{}, corpus: map[string]string{}}
	for _, active := range manifest.ActivePaths {
		if err := walkActivePath(root, active, loader.visit); err != nil {
			return nil, err
		}
	}
	set.Tasks = loader.tasks
	sort.Slice(set.Tasks, func(i, j int) bool { return set.Tasks[i].ID < set.Tasks[j].ID })
	return set, nil
}

type taskLoader struct {
	root   string
	seen   map[string]string
	corpus map[string]string
	tasks  []Task
}

func (l *taskLoader) visit(rel string, data []byte) error {
	task, err := DecodeTask(data)
	if err != nil {
		return err
	}
	if prior, duplicate := l.seen[task.ID]; duplicate {
		return invalidf(DetailDuplicateTaskID, "task %q is also defined in %s", task.ID, prior)
	}
	l.seen[task.ID] = rel
	if task.Kind == KindAgent {
		if err := l.checkCorpus(task.CorpusRef); err != nil {
			return err
		}
		if err := l.checkOracleFixtures(task.BlackBoxOracle); err != nil {
			return err
		}
	}
	task.Path = rel
	l.tasks = append(l.tasks, task)
	return nil
}

// checkCorpus compares the corpus file's raw-byte SHA-256 with the pinned one.
func (l *taskLoader) checkCorpus(ref *CorpusRef) error {
	sum, cached := l.corpus[ref.File]
	if !cached {
		data, err := readRepoFile(l.root, ref.File)
		if err != nil {
			return withPath(err, ref.File)
		}
		sum = sha256Hex(data)
		l.corpus[ref.File] = sum
	}
	if sum != ref.FileSHA256 {
		return invalidf(DetailCorpusDigestMismatch, "corpus %s has sha256 %s, task pins %s", ref.File, sum, ref.FileSHA256)
	}
	return nil
}

// walkActivePath visits every regular *.json file below one active path in
// lexical order. A symlink anywhere below root is rejected.
func walkActivePath(root, active string, visit func(rel string, data []byte) error) error {
	if err := lstatComponents(root, active, true); err != nil {
		return withPath(err, active)
	}
	base := filepath.Join(root, filepath.FromSlash(active))
	return filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
		rel := active
		if relative, err := filepath.Rel(base, current); err == nil && relative != "." {
			rel = active + "/" + filepath.ToSlash(relative)
		}
		if walkErr != nil {
			return withPath(&InvalidError{Detail: DetailReadFailed, Err: walkErr}, rel)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return withPath(invalidf(DetailSymlinkNotAllowed, "%s is a symlink", rel), rel)
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		data, err := readCapped(current)
		if err != nil {
			return withPath(&InvalidError{Detail: DetailReadFailed, Err: err}, rel)
		}
		return withPath(visit(rel, data), rel)
	})
}

// readRepoFile reads a regular file at a clean repository-relative path and
// refuses a symlink at any component below root.
func readRepoFile(root, rel string) ([]byte, error) {
	if !isCleanRelPath(rel) {
		return nil, invalidf(DetailUncleanPath, "%q is not a clean relative path", rel)
	}
	if err := lstatComponents(root, rel, false); err != nil {
		return nil, err
	}
	data, err := readCapped(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, &InvalidError{Detail: DetailReadFailed, Err: err}
	}
	return data, nil
}

// maxDocumentBytes caps every file the package reads: a golden set file, a
// corpus, the baseline, and the documents of a live session directory.
const maxDocumentBytes = 64 << 20

// readCapped reads a whole file of at most maxDocumentBytes. A regular file
// over the cap is refused before any byte is read; the limited reader bounds a
// file that grows meanwhile or reports no size.
func readCapped(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	tooLarge := fmt.Errorf("%s is larger than the 64 MiB document cap", filepath.Base(name))
	if info, err := file.Stat(); err == nil && info.Mode().IsRegular() && info.Size() > maxDocumentBytes {
		return nil, tooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocumentBytes {
		return nil, tooLarge
	}
	return data, nil
}

// lstatComponents checks each component of rel below root without following
// links: no component may be a symlink, inner components must be directories,
// and the last must be a directory when wantDir is set or a regular file.
func lstatComponents(root, rel string, wantDir bool) error {
	parts := strings.Split(rel, "/")
	current := root
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return &InvalidError{Detail: DetailReadFailed, Err: err}
		}
		component := strings.Join(parts[:index+1], "/")
		if info.Mode()&fs.ModeSymlink != 0 {
			return invalidf(DetailSymlinkNotAllowed, "%s is a symlink", component)
		}
		wantsDirectory := index < len(parts)-1 || wantDir
		if wantsDirectory && !info.IsDir() {
			return invalidf(DetailReadFailed, "%s is not a directory", component)
		}
		if !wantsDirectory && !info.Mode().IsRegular() {
			return invalidf(DetailReadFailed, "%s is not a regular file", component)
		}
	}
	return nil
}

// LoadBaseline strictly loads the committed baseline under root.
func LoadBaseline(root string) (*Baseline, error) {
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(BaselinePath))); errors.Is(err, fs.ErrNotExist) {
		return nil, ErrBaselineMissing
	}
	data, err := readRepoFile(root, BaselinePath)
	if err != nil {
		return nil, withPath(err, BaselinePath)
	}
	baseline, err := DecodeBaseline(data)
	if err != nil {
		return nil, withPath(err, BaselinePath)
	}
	for _, row := range baseline.Rows {
		if row.Kind == KindSurface && row.State == StateActive {
			return &baseline, nil
		}
	}
	return nil, ErrBaselineMissing
}

// DecodeBaseline strictly decodes and validates a harness_eval_baseline.v1
// document; rows must be in strictly ascending id order.
func DecodeBaseline(data []byte) (Baseline, error) {
	var baseline Baseline
	if err := strictDecode(data, &baseline); err != nil {
		return baseline, err
	}
	if baseline.SchemaVersion != BaselineSchemaV1 || blank(baseline.SetVersion) ||
		!sha256Pattern.MatchString(baseline.SetDigest) {
		return baseline, invalidf(DetailFieldInvalid, "baseline needs %s, a set_version, and a 64-hex set_digest", BaselineSchemaV1)
	}
	for index, row := range baseline.Rows {
		if index > 0 && row.ID <= baseline.Rows[index-1].ID {
			if row.ID == baseline.Rows[index-1].ID {
				return baseline, invalidf(DetailDuplicateTaskID, "baseline row %q repeats", row.ID)
			}
			return baseline, invalidf(DetailFieldInvalid, "baseline rows are not in ascending id order at %q", row.ID)
		}
		if err := validateBaselineRow(row); err != nil {
			return baseline, err
		}
	}
	return baseline, nil
}

func validateBaselineRow(row BaselineRow) error {
	okResult := (row.Kind == KindAgent && row.Result == ResultNotRun) ||
		(row.Kind == KindSurface && (row.Result == ResultPass || row.Result == ResultFail))
	okState := (row.State == StateActive && row.RetiredReason == "") ||
		(row.State == StateRetired && !blank(row.RetiredReason))
	okAccepted := row.AcceptedRegressionReason == "" || (row.Kind == KindSurface && row.Result == ResultFail)
	if !taskIDPattern.MatchString(row.ID) || !okResult || !okState || !okAccepted ||
		!sha256Pattern.MatchString(row.ExpectationDigest) {
		return invalidf(DetailFieldInvalid, "baseline row %q breaks the row contract", row.ID)
	}
	return nil
}
