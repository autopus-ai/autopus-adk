package healthband

import "path"

// The build-time closure of the Cargo manifests (patchpolicy_cargo.go):
// proc-macro crates, build dependencies, [patch] and [replace] paths, and,
// from each of them, every path dependency, transitively.

// cargoBuildPaths adds the Cargo denials of manifests (by folded
// directory; two spellings of one directory are both read) to out: every
// build-time crate's directory, every custom build script with its
// directory, and the .rs files beside a build script in a crate root.
// tracked reports a tracked file by its folded path.
func cargoBuildPaths(manifests map[string][]*cargoManifest, tracked func(string) bool, join func(dir, rel string) (string, bool), out *buildPaths) {
	wsDeps := map[string][]string{}
	var all []*cargoManifest
	for _, list := range manifests {
		all = append(all, list...)
	}
	for _, m := range all {
		for name, rel := range m.wsDeps {
			if dir, ok := join(m.dir, rel); ok {
				wsDeps[name] = append(wsDeps[name], dir)
			}
		}
	}
	var queue []string
	follow := func(m *cargoManifest, deps []cargoDep) {
		for _, dep := range deps {
			if dep.workspace {
				queue = append(queue, wsDeps[dep.name]...)
			} else if dir, ok := join(m.dir, dep.path); ok && dep.hasPath {
				queue = append(queue, dir)
			}
		}
	}
	for _, m := range all {
		if m.procMacro {
			queue = append(queue, m.dir)
			if lib, ok := join(m.dir, path.Dir(m.libPath)); ok && m.libPath != "" {
				out.trees = append(out.trees, lib) // a [lib] path outside the crate
			}
		}
		follow(m, m.buildDeps)
		for _, rel := range m.vendored {
			if dir, ok := join(m.dir, rel); ok {
				queue = append(queue, dir)
			}
		}
		m.buildScripts(tracked, join, out)
	}
	seen := map[string]bool{}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if seen[foldPath(dir)] {
			continue
		}
		seen[foldPath(dir)] = true
		out.trees = append(out.trees, dir)
		for _, m := range manifests[foldPath(dir)] {
			follow(m, m.deps)
			follow(m, m.buildDeps)
		}
	}
}

// buildScripts adds a manifest's build scripts: a custom script is denied
// with its directory when that is not the crate root; a script in the crate
// root, build.rs included, shares the root's module files, so the root's
// .rs files are denied (build.rs itself is denied by name).
func (m *cargoManifest) buildScripts(tracked func(string) bool, join func(dir, rel string) (string, bool), out *buildPaths) {
	if m.noBuild && len(m.builds) == 0 {
		return
	}
	if len(m.builds) == 0 && tracked(foldPath(path.Join(m.dir, "build.rs"))) {
		out.rsDirs = append(out.rsDirs, m.dir)
	}
	for _, rel := range m.builds {
		script, ok := join(m.dir, rel)
		if !ok {
			continue
		}
		out.trees = append(out.trees, script)
		if dir := repoDir(script); foldPath(dir) != foldPath(m.dir) {
			out.trees = append(out.trees, dir)
		} else {
			out.rsDirs = append(out.rsDirs, m.dir)
		}
	}
}
