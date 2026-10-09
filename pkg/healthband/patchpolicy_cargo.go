package healthband

// The Cargo half of RR-7: which allowed sources of the base tree a cargo
// build runs. A proc-macro crate runs inside the compiler, a build script
// runs before its crate compiles, and both run the path dependencies they
// pull in, transitively; a [patch] or [replace] path stands in for a
// registry crate that may be one of them. Every value that a rule reads
// must have the type Cargo gives it, else the manifest is unparsable.

// cargoDep is one dependency entry that names a path or the workspace.
type cargoDep struct {
	name, path         string
	hasPath, workspace bool
}

// cargoManifest is what RR-7 reads from one Cargo.toml; every path is
// relative to dir, the manifest's repository-relative directory.
type cargoManifest struct {
	dir       string
	procMacro bool
	libPath   string   // [lib] path, which counts for a proc-macro crate
	builds    []string // package.build paths; nil for the default build.rs
	noBuild   bool     // package.build = false
	deps      []cargoDep
	buildDeps []cargoDep
	wsDeps    map[string]string // [workspace.dependencies] name to path
	vendored  []string          // [patch.<source>] and [replace] paths
}

// cargoDepTables are the dependency tables read, true for a build
// dependency table: normal dependencies count only for a crate that runs at
// build time, and dev-dependencies never run in a build.
var cargoDepTables = map[string]bool{"dependencies": false, "build-dependencies": true, "build_dependencies": true}

func parseCargoManifest(dir, text string) (*cargoManifest, error) {
	doc, err := parseTOML(text)
	if err != nil {
		return nil, err
	}
	m := &cargoManifest{dir: dir, wsDeps: map[string]string{}}
	for _, name := range []string{"package", "project"} {
		pkg, err := doc.subtable(name)
		if err != nil {
			return nil, err
		}
		if err := m.readBuild(pkg.get("build")); err != nil {
			return nil, err
		}
	}
	if err := m.readLib(doc); err != nil {
		return nil, err
	}
	if err := m.readDepTables(doc); err != nil {
		return nil, err
	}
	targets, err := doc.subtable("target")
	if err != nil {
		return nil, err
	}
	for _, value := range targets.values() {
		target, ok := value.(*tomlTable)
		if !ok {
			return nil, errTOML
		}
		if err := m.readDepTables(target); err != nil {
			return nil, err
		}
	}
	return m, m.readSources(doc)
}

// readBuild reads package.build: absent or true is the default build.rs,
// false none, and a string or an array of strings the script paths.
func (m *cargoManifest) readBuild(value any) error {
	switch v := value.(type) {
	case nil:
	case bool:
		m.noBuild = m.noBuild || !v
	case string:
		m.builds = append(m.builds, v)
	case *tomlArray:
		paths, ok := v.stringItems()
		if !ok || len(paths) == 0 {
			return errTOML
		}
		m.builds = append(m.builds, paths...)
	default:
		return errTOML
	}
	return nil
}

// readLib reads [lib]: proc-macro = true (or the legacy proc_macro), or a
// crate-type list that holds "proc-macro", makes a proc-macro crate.
func (m *cargoManifest) readLib(doc *tomlTable) error {
	lib, err := doc.subtable("lib")
	if err != nil {
		return err
	}
	for _, key := range []string{"proc-macro", "proc_macro"} {
		switch v := lib.get(key).(type) {
		case nil:
		case bool:
			m.procMacro = m.procMacro || v
		default:
			return errTOML
		}
	}
	for _, key := range []string{"crate-type", "crate_type"} {
		switch v := lib.get(key).(type) {
		case nil:
		case *tomlArray:
			types, ok := v.stringItems()
			if !ok {
				return errTOML
			}
			for _, kind := range types {
				m.procMacro = m.procMacro || kind == "proc-macro"
			}
		default:
			return errTOML
		}
	}
	switch v := lib.get("path").(type) {
	case nil:
	case string:
		m.libPath = v
	default:
		return errTOML
	}
	return nil
}

// readDepTables reads the dependency tables of a manifest or of one
// [target.<cfg>] table.
func (m *cargoManifest) readDepTables(owner *tomlTable) error {
	for table, build := range cargoDepTables {
		deps, err := owner.subtable(table)
		if err != nil {
			return err
		}
		for _, name := range deps.keys() {
			dep, ok, err := readCargoDep(name, deps.kv[name])
			switch {
			case err != nil:
				return err
			case ok && build:
				m.buildDeps = append(m.buildDeps, dep)
			case ok:
				m.deps = append(m.deps, dep)
			}
		}
	}
	return nil
}

// readCargoDep reads one entry: a version string names no path, and a table
// may name a path or inherit the workspace entry of the same name.
func readCargoDep(name string, value any) (cargoDep, bool, error) {
	switch v := value.(type) {
	case string:
		return cargoDep{}, false, nil
	case *tomlTable:
		dep := cargoDep{name: name}
		switch p := v.get("path").(type) {
		case nil:
		case string:
			dep.path, dep.hasPath = p, true
		default:
			return cargoDep{}, false, errTOML
		}
		switch w := v.get("workspace").(type) {
		case nil:
		case bool:
			dep.workspace = w
		default:
			return cargoDep{}, false, errTOML
		}
		return dep, dep.workspace || dep.hasPath, nil
	}
	return cargoDep{}, false, errTOML
}

// readSources reads [workspace.dependencies], [patch.<source>], and
// [replace].
func (m *cargoManifest) readSources(doc *tomlTable) error {
	workspace, err := doc.subtable("workspace")
	if err != nil {
		return err
	}
	wsDeps, err := workspace.subtable("dependencies")
	if err != nil {
		return err
	}
	for _, name := range wsDeps.keys() {
		dep, ok, err := readCargoDep(name, wsDeps.kv[name])
		if err != nil {
			return err
		}
		if ok && dep.hasPath {
			m.wsDeps[name] = dep.path
		}
	}
	patches, err := doc.subtable("patch")
	if err != nil {
		return err
	}
	sources := []*tomlTable{}
	for _, value := range patches.values() {
		source, ok := value.(*tomlTable)
		if !ok {
			return errTOML
		}
		sources = append(sources, source)
	}
	replace, err := doc.subtable("replace")
	if err != nil {
		return err
	}
	for _, source := range append(sources, replace) {
		for _, name := range source.keys() {
			dep, ok, err := readCargoDep(name, source.kv[name])
			if err != nil {
				return err
			}
			if ok && dep.hasPath {
				m.vendored = append(m.vendored, dep.path)
			}
		}
	}
	return nil
}

// subtable returns the table at key, an empty one when the key is absent;
// any other value is not the table Cargo reads.
func (t *tomlTable) subtable(key string) (*tomlTable, error) {
	switch v := t.get(key).(type) {
	case nil:
		return newTOMLTable(), nil
	case *tomlTable:
		return v, nil
	}
	return nil, errTOML
}

func (t *tomlTable) get(key string) any { return t.kv[key] }

func (t *tomlTable) keys() []string {
	keys := make([]string, 0, len(t.kv))
	for key := range t.kv {
		keys = append(keys, key)
	}
	return keys
}

func (t *tomlTable) values() []any {
	values := make([]any, 0, len(t.kv))
	for _, value := range t.kv {
		values = append(values, value)
	}
	return values
}

// stringItems returns an array's items when every one is a string.
func (a *tomlArray) stringItems() ([]string, bool) {
	out := make([]string, 0, len(a.items))
	for _, item := range a.items {
		s, ok := item.(string)
		if !ok || a.tables {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
