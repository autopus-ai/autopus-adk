package healthband

import (
	"bytes"
	"errors"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/insajin/autopus-adk/pkg/editguard"
)

// Patch Policy items 2–4 against the base commit. The base entry of every
// touched path and of each of its directory prefixes comes from its own
// `git ls-tree -z <base> -- ':(literal)<p>'`, each distinct p once, and only
// an output entry whose path equals p byte for byte counts (CD-3 H2, N3): a
// single ls-tree -t call lists only the trees it recurses into and never a
// symlink or gitlink prefix. The base mode decides, not the headers, so a
// modeless hunk on a tracked symlink or gitlink is refused here.
//
// Git precomposes argv on macOS (core.precomposeUnicode), so a pathspec
// spelled in NFD matches only an NFC entry, and an entry stored in NFD
// matches no pathspec. Item 4's listing therefore carries modes
// (`git ls-tree -r -z <base>`, the --name-only listing plus each entry's
// mode, read from the tree bytes with no pathspec), and every exact-path
// answer is cross-checked against it byte for byte.

// treeEntry is one base entry.
type treeEntry struct {
	mode, kind string
}

// baseView is what the policy read of the base: the exact-path answers, the
// listed non-tree entries, and the folds of every tracked path and prefix.
type baseView struct {
	exact, listed map[string]treeEntry
	folds         map[string][]string
}

var errGitOutput = errors.New("healthband: unexpected git output")

// readBase runs the exact-path queries and the listing.
func (r policyRun) readBase(files []*diffFile) (baseView, error) {
	view := baseView{exact: map[string]treeEntry{}, listed: map[string]treeEntry{}, folds: map[string][]string{}}
	seen := map[string]bool{}
	for _, file := range files {
		for _, p := range append(pathPrefixes(file.path), file.path) {
			if seen[p] {
				continue
			}
			seen[p] = true
			out, err := r.git(nil, "ls-tree", "-z", r.in.BaseSHA, "--", ":(literal)"+p)
			if err != nil {
				return baseView{}, err
			}
			if entry, name, ok := lsTreeRecord(out, p); ok && name == p {
				view.exact[p] = entry
			}
		}
	}
	out, err := r.git(nil, "ls-tree", "-r", "-z", r.in.BaseSHA)
	if err != nil {
		return baseView{}, err
	}
	folded := map[string]bool{}
	for _, record := range bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0}) {
		if len(out) == 0 {
			break // an empty base tree lists nothing
		}
		entry, name, ok := lsTreeRecord(record, "")
		if !ok {
			return baseView{}, errGitOutput
		}
		view.listed[name] = entry
		for _, p := range append(pathPrefixes(name), name) {
			if !folded[p] {
				folded[p] = true
				view.folds[foldPath(p)] = append(view.folds[foldPath(p)], p)
			}
		}
	}
	return view, nil
}

// lsTreeRecord reads the `<mode> <type> <oid>\t<path>` record of want, or
// the first record when want is empty.
func lsTreeRecord(out []byte, want string) (treeEntry, string, bool) {
	for _, record := range bytes.Split(out, []byte{0}) {
		meta, name, ok := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if ok && len(fields) == 3 && (want == "" || string(name) == want) {
			return treeEntry{mode: fields[0], kind: fields[1]}, string(name), true
		}
	}
	return treeEntry{}, "", false
}

// allowedBaseEntries is item 3's base rule: an added path has no entry, a
// modified path is a 100644 or 100755 blob, and every directory prefix is
// absent or a tree, in the exact-path answers and in the listing alike.
func allowedBaseEntries(files []*diffFile, base baseView) bool {
	for _, file := range files {
		exact, exists := base.exact[file.path]
		listed, isListed := base.listed[file.path]
		switch {
		case file.isNew && (exists || isListed):
			return false
		case !file.isNew && (!exists || !regularBlob(exact) || (isListed && !regularBlob(listed))):
			return false
		}
		for _, prefix := range pathPrefixes(file.path) {
			entry, ok := base.exact[prefix]
			if _, isListed := base.listed[prefix]; isListed || (ok && (entry.mode != "040000" || entry.kind != "tree")) {
				return false
			}
		}
	}
	return true
}

func regularBlob(entry treeEntry) bool {
	return entry.kind == "blob" && (entry.mode == "100644" || entry.mode == "100755")
}

// filtered reports a touched path whose filter attribute is set at the base,
// lfs included (item 3, CD-3 F-009): Git Execution Policy item 2 blanks the
// git-lfs driver, so an edit of a pointer file would commit raw content.
// Every touched path must be answered; answers are matched in NFC, because
// git echoes a precomposed argv path.
func (r policyRun) filtered(files []*diffFile) (bool, error) {
	args := []string{"check-attr", "-z", "--source=" + r.in.BaseSHA, "filter", "--"}
	for _, file := range files {
		args = append(args, file.path)
	}
	out, err := r.git(nil, args...)
	if err != nil {
		return false, err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(fields)%3 != 0 {
		return false, errGitOutput
	}
	answered := map[string]bool{}
	for i := 0; i < len(fields); i += 3 {
		if fields[i+1] != "filter" {
			return false, errGitOutput
		}
		if value := fields[i+2]; value != "unspecified" && value != "unset" {
			return true, nil
		}
		answered[norm.NFC.String(fields[i])] = true
	}
	for _, file := range files {
		if !answered[norm.NFC.String(file.path)] {
			return false, errGitOutput
		}
	}
	return false, nil
}

// caseCollision is item 4 (CD-3 M2): two distinct paths or prefixes of the
// diff with one fold, or one whose fold equals that of a different tracked
// path or directory at the base, collide.
func caseCollision(files []*diffFile, tracked map[string][]string) string {
	names := map[string]bool{}
	for _, file := range files {
		for _, p := range append(pathPrefixes(file.path), file.path) {
			names[p] = true
		}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	byFold := map[string]string{}
	for _, name := range sorted {
		key := foldPath(name)
		if other, ok := byFold[key]; ok && other != name {
			return PatchCodeCaseCollision
		}
		byFold[key] = name
		for _, spelling := range tracked[key] {
			if spelling != name {
				return PatchCodeCaseCollision
			}
		}
	}
	return ""
}

// foldPath is the canonical caseless form of a path: editguard's FoldKey
// for a case-insensitive volume, NFD(fold(NFD(p))) with the Cherokee case
// pairs joined, so an NFD and an NFC spelling and every case spelling of one
// name fold alike, as on APFS.
func foldPath(path string) string { return editguard.FoldKey(path, true) }
