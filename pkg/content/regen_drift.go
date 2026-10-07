package content

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DetectTemplateRegenDrift regenerates templates from dir/content into a temp
// directory and returns the committed dir/templates paths, slash-separated and
// relative to templates/, that the regeneration would change. The committed
// tree is never written. Every walk, relative-path, read, and regeneration
// error is returned, so a gate never mistakes an incomplete comparison for
// fresh templates; the doctor's advisory check treats an error as a skip.
func DetectTemplateRegenDrift(dir string) ([]string, error) {
	tmp, err := os.MkdirTemp("", "autopus-regen-")
	if err != nil {
		return nil, fmt.Errorf("regeneration temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := GenerateAllTemplates(filepath.Join(dir, "content"), tmp); err != nil {
		return nil, fmt.Errorf("regenerate templates: %w", err)
	}
	return DiffRegeneratedTemplates(filepath.Join(dir, "templates"), tmp)
}

// DiffRegeneratedTemplates compares both directions and returns the sorted
// stale paths. Regenerated files detect missing or stale committed output;
// committed generator-owned paths detect residue left after a source was
// deleted. Hand-authored template families are excluded from the reverse
// comparison by IsGeneratedTemplatePath. A committed file that does not exist
// is stale; any other failure is returned as an error.
func DiffRegeneratedTemplates(committedDir, regenDir string) ([]string, error) {
	stale := make(map[string]bool)
	regenerated := make(map[string]bool)
	err := filepath.WalkDir(regenDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := slashRel(regenDir, current)
		if err != nil {
			return err
		}
		regenerated[rel] = true
		regenBytes, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		committedBytes, err := os.ReadFile(filepath.Join(committedDir, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			stale[rel] = true
			return nil
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(committedBytes, regenBytes) {
			stale[rel] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("compare regenerated templates: %w", err)
	}
	err = filepath.WalkDir(committedDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := slashRel(committedDir, current)
		if err != nil {
			return err
		}
		if IsGeneratedTemplatePath(rel) && !regenerated[rel] {
			stale[rel] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("compare committed templates: %w", err)
	}
	paths := make([]string, 0, len(stale))
	for rel := range stale {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths, nil
}

func slashRel(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
