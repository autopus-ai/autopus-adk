package editguard

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

const (
	manifestDir    = ".autopus"
	manifestSuffix = "-manifest.json"
	// maxManifestBytes bounds one manifest read; a larger file is unreadable.
	maxManifestBytes = 8 << 20
)

// sourceRepoMarkers identify the ADK source repo, as internal/cli
// doctor_drift_source.go does: all three must exist.
var sourceRepoMarkers = []string{"content", "templates", filepath.Join("cmd", "generate-templates")}

// manifestHit is what the manifests of one root say about one file.
type manifestHit struct {
	display  string // the file as the manifest spells it
	manifest string // first manifest, in lexical order, that lists it always
	always   bool
	override bool // some manifest lists it merge or marker
}

// manifestStage is the manifest stage of one project root (REQ-EG-03, 04, 18).
type manifestStage struct {
	entries map[string]manifestHit
	// fault names the first manifest (or the manifest directory) that could not
	// be read or parsed. A faulted stage reports nothing as generated, because
	// the unreadable file may hold the merge or marker entry that prevents a
	// false deny.
	fault string
}

// loadManifestStage reads every `.autopus/*-manifest.json` of root in lexical
// order. Entries outside the edit-guard namespace are ignored, so a forged
// entry cannot widen what the guard denies.
func loadManifestStage(root string, fold bool) manifestStage {
	stage := manifestStage{entries: map[string]manifestHit{}}
	entries, err := os.ReadDir(filepath.Join(root, manifestDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return stage
		}
		return manifestStage{fault: manifestDir}
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), manifestSuffix) {
			continue
		}
		rel := manifestDir + "/" + entry.Name()
		files, err := readManifestFiles(filepath.Join(root, manifestDir, entry.Name()))
		if err != nil {
			return manifestStage{fault: rel}
		}
		paths := make([]string, 0, len(files))
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			stage.add(p, files[p], rel, fold)
		}
	}
	return stage
}

func readManifestFiles(name string) (map[string]string, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxManifestBytes {
		return nil, errors.New("editguard: manifest exceeds the read bound")
	}
	var doc struct {
		Files map[string]struct {
			Policy string `json:"policy"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	files := make(map[string]string, len(doc.Files))
	for p, entry := range doc.Files {
		files[p] = entry.Policy
	}
	return files, nil
}

func (s manifestStage) add(raw, policy, manifest string, fold bool) {
	display := cleanManifestPath(raw)
	if display == "" {
		return
	}
	key := FoldKey(display, fold)
	if !workflow.InEditGuardNamespace(key) {
		return
	}
	hit := s.entries[key]
	switch policy {
	case "always":
		if !hit.always {
			hit.always, hit.display, hit.manifest = true, display, manifest
		}
	case "merge", "marker":
		hit.override = true
	default:
		return
	}
	s.entries[key] = hit
}

// generated reports whether the stage proves key is a regenerated file.
func (s manifestStage) generated(key string) (manifestHit, bool) {
	if s.fault != "" {
		return manifestHit{}, false
	}
	hit := s.entries[key]
	return hit, hit.always && !hit.override
}

func cleanManifestPath(raw string) string {
	clean := path.Clean(filepath.ToSlash(raw))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return ""
	}
	return clean
}

// isSourceRepo reports whether root is the ADK source repo, which selects the
// GS-SRC reason over GS-CON.
func isSourceRepo(root string) bool {
	for _, marker := range sourceRepoMarkers {
		if _, err := os.Stat(filepath.Join(root, marker)); err != nil {
			return false
		}
	}
	return true
}
