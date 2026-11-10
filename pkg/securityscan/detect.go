package securityscan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Ecosystem is a dependency ecosystem found in the project directory.
// Manifest names the file that decided it; Tool is the package manager for
// node (npm, pnpm, yarn, yarn-berry), empty when no lockfile says.
type Ecosystem struct {
	Name     string `json:"name"`
	Manifest string `json:"manifest"`
	Tool     string `json:"tool,omitempty"`
}

// Detect lists the ecosystems whose files sit directly in dir, in a fixed
// order: go, node, rust, python.
func Detect(dir string) []Ecosystem {
	has := func(name string) bool {
		info, err := os.Stat(filepath.Join(dir, name))
		return err == nil && !info.IsDir()
	}
	var out []Ecosystem
	if has("go.mod") {
		out = append(out, Ecosystem{Name: "go", Manifest: "go.mod"})
	}
	if has("package.json") {
		out = append(out, nodeEcosystem(has))
	}
	if has("Cargo.lock") {
		out = append(out, Ecosystem{Name: "rust", Manifest: "Cargo.lock"})
	} else if has("Cargo.toml") {
		out = append(out, Ecosystem{Name: "rust", Manifest: "Cargo.toml"})
	}
	if py := pythonManifest(dir, has); py != "" {
		out = append(out, Ecosystem{Name: "python", Manifest: py})
	}
	return out
}

func nodeEcosystem(has func(string) bool) Ecosystem {
	switch {
	case has("pnpm-lock.yaml"):
		return Ecosystem{Name: "node", Manifest: "pnpm-lock.yaml", Tool: "pnpm"}
	case has("yarn.lock") && has(".yarnrc.yml"):
		return Ecosystem{Name: "node", Manifest: "yarn.lock", Tool: "yarn-berry"}
	case has("yarn.lock"):
		return Ecosystem{Name: "node", Manifest: "yarn.lock", Tool: "yarn"}
	case has("package-lock.json"):
		return Ecosystem{Name: "node", Manifest: "package-lock.json", Tool: "npm"}
	case has("npm-shrinkwrap.json"):
		return Ecosystem{Name: "node", Manifest: "npm-shrinkwrap.json", Tool: "npm"}
	}
	return Ecosystem{Name: "node", Manifest: "package.json"}
}

// pythonManifest prefers requirements files, the only input pip-audit can
// check without installing anything; it lists all of them, comma-separated.
func pythonManifest(dir string, has func(string) bool) string {
	reqs, _ := filepath.Glob(filepath.Join(dir, "requirements*.txt"))
	for i, r := range reqs {
		reqs[i] = filepath.Base(r)
	}
	slices.Sort(reqs)
	if len(reqs) > 0 {
		return strings.Join(reqs, ", ")
	}
	for _, name := range []string{"pyproject.toml", "poetry.lock", "uv.lock"} {
		if has(name) {
			return name
		}
	}
	return ""
}
