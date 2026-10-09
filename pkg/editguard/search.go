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
// fails closed. The host then percent-decodes the picked file and resolves it
// (resolveToRealPath, chunk-7LQRUKPT.js:252203), so a file whose own name
// holds an escape is written at its decoded path: each decoded spelling of
// the path is searched as well (decodedSearches).
func (ev *evaluation) searchTargets(cwd, raw string, located []Target) []Target {
	name := filepath.Base(raw) // path.basename on the host's OS
	abs, err := absolutePath(cwd, raw)
	if err != nil || filepath.IsAbs(raw) || name == "." || name == ".." || exists(filepath.Clean(abs)) {
		return nil
	}
	suffix := strings.ReplaceAll(raw, `\`, "/")
	searches := append([]searchSuffix{{suffix, name}}, decodedSearches(suffix)...)
	var found []Target
	for i, at := range located {
		if slices.ContainsFunc(located[:i], func(seen Target) bool { return seen.Root == at.Root }) {
			continue
		}
		for _, s := range searches {
			found = append(found, ev.stagesOf(at).search(ev, s.suffix, s.name)...)
		}
	}
	return found
}

// searchSuffix is one path the host may write for a searched file: a suffix
// of it, and its file name, matched whole.
type searchSuffix struct{ suffix, name string }

// decodedSearches returns, for each decodedRounds spelling of a searched
// suffix, the part of the decoded path the host writes that the guard can
// know. The picked file is the unseen part of its path followed by suffix, so
// the decoded suffix is resolved as path.resolve resolves it after that
// unseen part: a `.` drops out, a `..` removes the name before it, and a `..`
// with no known name before it climbs into the unseen part, after which only
// what follows is known and it starts at a segment boundary. When suffix
// holds a slash its first segment may end a longer name, so a first segment of
// `.` or `..` counts as a climb too. A decoded path that ends in a directory,
// or holds a NUL byte, is never written and adds nothing. The workspace walk
// builds the picked path from directory entries, so it holds no empty, `.`,
// or `..` segment: a suffix with one after its first segment matches no file,
// and the host writes the literal path instead.
func decodedSearches(suffix string) []searchSuffix {
	for _, segment := range strings.Split(suffix, "/")[1:] {
		if segment == "" || segment == "." || segment == ".." {
			return nil
		}
	}
	var searches []searchSuffix
	for _, decoded := range decodedRounds(suffix) {
		if strings.ContainsRune(decoded, 0) {
			continue
		}
		if filepath.Separator == '\\' {
			decoded = strings.ReplaceAll(decoded, `\`, "/")
		}
		atBoundary := !strings.Contains(suffix, "/") // the name is matched whole
		var known []string
		for i, segment := range strings.Split(decoded, "/") {
			partial := i == 0 && !atBoundary
			switch {
			case i == 0 && segment == "":
				atBoundary = true
			case partial && (segment == "." || segment == ".."):
				atBoundary = true
			case segment == "" || segment == ".":
			case segment == ".." && (len(known) > 1 || len(known) == 1 && atBoundary):
				known = known[:len(known)-1]
			case segment == "..":
				known, atBoundary = nil, true
			default:
				known = append(known, segment)
			}
		}
		if len(known) == 0 {
			continue
		}
		tail := strings.Join(known, "/")
		if atBoundary {
			tail = "/" + tail
		}
		searches = append(searches, searchSuffix{tail, known[len(known)-1]})
	}
	return searches
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
