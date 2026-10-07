package editguard

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// searchTargets mirrors the correctPath search of Gemini CLI 0.52.0's replace
// tool (bundle chunk-7LQRUKPT.js:289558): a relative file_path that names no
// file is swapped for the one workspace file whose path ends with it, the
// file name matched whole and both paths compared as plain strings with
// backslashes read as slashes. Walking the workspace would break the latency
// budget, so the guard checks every protected file it can name without a
// walk, in each root that encloses the literal path, and each match is one
// more target. A path that several files end with, which the host does not
// redirect, is denied too: the guard sees only the protected files, so it
// fails closed.
func (ev *evaluation) searchTargets(cwd, raw string, located []Target) []Target {
	name := filepath.Base(raw) // path.basename on the host's OS
	abs, err := absolutePath(cwd, raw)
	if err != nil || filepath.IsAbs(raw) || name == "." || name == ".." || exists(filepath.Clean(abs)) {
		return nil
	}
	suffix := strings.ReplaceAll(raw, `\`, "/")
	var found []Target
	for i, at := range located {
		if !slices.ContainsFunc(located[:i], func(seen Target) bool { return seen.Root == at.Root }) {
			found = append(found, ev.stagesOf(at).search(ev, suffix, name)...)
		}
	}
	return found
}

// search returns, in lexical order, the protected files of one root that the
// host's search could pick for suffix: guard state with the file's name, the
// active locks, and the generated manifest entries. A faulted manifest stage
// drops only its own matches (REQ-EG-18).
func (s *rootStages) search(ev *evaluation, suffix, name string) []Target {
	want, wantName := FoldKey(suffix, s.fold), FoldKey(name, s.fold)
	rootKey := strings.TrimSuffix(FoldKey(filepath.ToSlash(s.root), s.fold), "/")
	var found []Target
	consider := func(rel, key string) {
		if path.Base(key) == wantName && strings.HasSuffix(strings.ReplaceAll(rootKey+"/"+key, `\`, "/"), want) {
			found = append(found, Target{Root: s.root, Rel: rel, Key: key, CaseInsensitive: s.fold})
		}
	}
	for _, rel := range []string{manifestDir + "/" + name, FixLocksDir + "/" + name} {
		if key := FoldKey(rel, s.fold); isGuardState(key) && exists(filepath.Join(s.root, filepath.FromSlash(rel))) {
			consider(rel, key)
		}
	}
	for _, lock := range s.lockView(ev).active {
		consider(lock.path, lock.key)
	}
	if manifests := s.manifestStage(); manifests.fault != "" {
		ev.note("manifest unreadable: " + displayPath(manifests.fault))
	} else {
		for key, hit := range manifests.entries {
			if hit.always && !hit.override {
				consider(hit.display, key)
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Rel < found[j].Rel })
	return found
}

func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}
