package healthband

import (
	"bytes"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Patch Policy item 6 over the paths that a build or an IDE sync runs
// (RR-7). The Cargo manifests and Gradle settings files of the base tree,
// never of the patched one, name the allowed sources that run at build
// time; a patch cannot change a manifest, because .toml, .gradle, and .kts
// are not allowed extensions (item 5). The manifests are the blobs of the
// base listing, read with one git cat-file --batch within
// PatchManifestMaxFiles, PatchManifestMaxBytes, and
// PatchManifestMaxTotalBytes, shallowest first. A manifest band does not
// read (past a bound, or not a regular blob) or cannot parse denies its own
// directory tree: the crate or build it describes, whose targets live there
// by default. That fails closed without denying the whole repository, which
// it does only for an unreadable manifest at the root. Every comparison is
// on folded paths, as item 4's are.

// Bounds of the manifest read.
const (
	PatchManifestMaxFiles      = 1024
	PatchManifestMaxBytes      = 256 << 10
	PatchManifestMaxTotalBytes = 8 << 20
)

type manifestKind int

const (
	manifestCargo manifestKind = iota + 1
	manifestGroovySettings
	manifestKotlinSettings
)

// manifestNames are the folded file names of the manifests read.
var manifestNames = map[string]manifestKind{
	"cargo.toml": manifestCargo, "settings.gradle": manifestGroovySettings, "settings.gradle.kts": manifestKotlinSettings,
}

// buildPaths are the derived denials.
type buildPaths struct {
	trees  []string // each denies the path and everything below it; "" is the whole tree
	rsDirs []string // each denies the .rs files directly in the directory
}

// denies reports whether p falls under a derived denial, after folding.
func (b buildPaths) denies(p string) bool {
	folded := foldPath(p)
	for _, tree := range b.trees {
		if t := foldPath(tree); t == "" || folded == t || strings.HasPrefix(folded, t+"/") {
			return true
		}
	}
	if strings.HasSuffix(folded, ".rs") {
		for _, dir := range b.rsDirs {
			if foldPath(dir) == repoDir(folded) {
				return true
			}
		}
	}
	return false
}

type buildManifest struct {
	path  string
	kind  manifestKind
	entry treeEntry
}

// buildDenials derives the denials from the manifests of the base listing.
// An error is a git fault.
func (r policyRun) buildDenials(base baseView) (buildPaths, error) {
	var out buildPaths
	var manifests []buildManifest
	for name, entry := range base.listed {
		if kind := manifestNames[foldPath(path.Base(name))]; kind != 0 {
			manifests = append(manifests, buildManifest{path: name, kind: kind, entry: entry})
		}
	}
	if len(manifests) == 0 {
		return out, nil
	}
	sort.Slice(manifests, func(i, j int) bool {
		a, b := manifests[i].path, manifests[j].path
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return da < db
		}
		return a < b
	})
	// The bounds count manifests, not distinct blobs, since each manifest
	// is parsed on its own.
	sizes := map[string]int64{}
	var read []buildManifest
	var total int64
	for _, m := range manifests {
		size := m.entry.size
		if !regularBlob(m.entry) || size < 0 || size > PatchManifestMaxBytes || len(read) >= PatchManifestMaxFiles ||
			total+size > PatchManifestMaxTotalBytes {
			out.trees = append(out.trees, repoDir(m.path))
			continue
		}
		sizes[m.entry.oid], total = size, total+size
		read = append(read, m)
	}
	blobs, err := r.readBlobs(sizes)
	if err != nil {
		return buildPaths{}, err
	}
	cargo := map[string][]*cargoManifest{}
	for _, m := range read {
		dir, text := repoDir(m.path), string(blobs[m.entry.oid])
		if m.kind == manifestCargo {
			parsed, err := parseCargoManifest(dir, text)
			if err != nil {
				out.trees = append(out.trees, dir)
				continue
			}
			cargo[foldPath(dir)] = append(cargo[foldPath(dir)], parsed)
			continue
		}
		builds, ok := gradleIncludedBuilds(text, m.kind == manifestKotlinSettings)
		if !ok {
			out.trees = append(out.trees, dir)
			continue
		}
		for _, rel := range builds {
			if build, ok := r.repoJoin(dir, rel); ok {
				out.trees = append(out.trees, build)
			}
		}
	}
	if len(cargo) > 0 {
		tracked := make(map[string]bool, len(base.listed))
		for name := range base.listed {
			tracked[foldPath(name)] = true
		}
		cargoBuildPaths(cargo, func(folded string) bool { return tracked[folded] }, r.repoJoin, &out)
	}
	return out, nil
}

// readBlobs runs git cat-file --batch over the OIDs of sizes, each once, and
// checks every header and length against the listing.
func (r policyRun) readBlobs(sizes map[string]int64) (map[string][]byte, error) {
	if len(sizes) == 0 {
		return nil, nil
	}
	oids := make([]string, 0, len(sizes))
	for oid := range sizes {
		if !validGitOID(oid) {
			return nil, errGitOutput
		}
		oids = append(oids, oid)
	}
	sort.Strings(oids)
	out, err := r.git([]byte(strings.Join(oids, "\n")+"\n"), "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	blobs := make(map[string][]byte, len(oids))
	for _, oid := range oids {
		size := sizes[oid]
		header, rest, found := bytes.Cut(out, []byte{'\n'})
		if !found || string(header) != oid+" blob "+strconv.FormatInt(size, 10) ||
			int64(len(rest)) <= size || rest[size] != '\n' {
			return nil, errGitOutput
		}
		blobs[oid], out = rest[:size], rest[size+1:]
	}
	if len(out) != 0 {
		return nil, errGitOutput
	}
	return blobs, nil
}

// repoJoin resolves rel, a manifest's path, against dir, a repository-
// relative directory ("" is the root). A path that climbs out of the
// repository is outside every patch; an absolute one counts only inside the
// user's checkout.
func (r policyRun) repoJoin(dir, rel string) (string, bool) {
	if strings.HasPrefix(rel, "/") {
		root := filepath.ToSlash(filepath.Clean(r.in.Checkout))
		abs := path.Clean(rel)
		if abs == root {
			return "", true
		}
		inside, found := strings.CutPrefix(abs, strings.TrimSuffix(root, "/")+"/")
		return inside, found
	}
	joined := path.Clean(path.Join(dir, rel))
	switch {
	case joined == ".":
		return "", true
	case joined == ".." || strings.HasPrefix(joined, "../"):
		return "", false
	}
	return joined, true
}

// repoDir is the repository-relative directory of p, "" for the root.
func repoDir(p string) string {
	if dir := path.Dir(p); dir != "." {
		return dir
	}
	return ""
}
