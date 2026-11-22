package memindex

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/learn"
)

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(body), nil
}

func sourceFreshness(projectDir, sourceRef, expectedHash string) string {
	current, err := currentSourceHash(projectDir, sourceRef)
	if err != nil {
		if os.IsNotExist(err) {
			return Missing
		}
		return Stale
	}
	if current != expectedHash {
		return Stale
	}
	return Fresh
}

func currentSourceHash(projectDir, sourceRef string) (string, error) {
	pathPart, fragment, _ := strings.Cut(sourceRef, "#")
	if strings.HasPrefix(pathPart, "L-") {
		return hashLearningLine(filepath.Join(projectDir, ".autopus", "learnings", "pipeline.jsonl"), pathPart)
	}
	path := filepath.Join(projectDir, filepath.FromSlash(pathPart))
	if strings.HasPrefix(fragment, "L-") {
		return hashLearningLine(path, fragment)
	}
	return hashFile(path)
}

func hashLearningLine(path, id string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	// The same merged view scanLearning indexes: the first lesson holding id,
	// with the copies a union merge left resolved to one line.
	lines, _, err := learn.ScanStore(f)
	if err != nil {
		return "", err
	}
	for _, l := range lines {
		if l.Entry.ID == id {
			return hashBytes([]byte(l.Raw)), nil
		}
	}
	return "", fmt.Errorf("%w: %s#%s", os.ErrNotExist, path, id)
}

func slashRel(projectDir, path string) string {
	rel, err := filepath.Rel(projectDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func pathWithinForCreate(root, target string) (bool, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return false, err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	parent := filepath.Dir(targetAbs)
	parent, err = existingParent(parent)
	if err != nil {
		return false, err
	}
	resolved := filepath.Join(parent, filepath.Base(targetAbs))
	rel, err := filepath.Rel(rootAbs, resolved)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func existingParent(path string) (string, error) {
	current := filepath.Clean(path)
	missing := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
