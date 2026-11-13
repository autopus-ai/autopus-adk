package verifygate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Step is one verification command, run in Dir.
type Step struct {
	Name string
	Argv []string
	Dir  string
	// PassExit lists exit statuses besides 0 that do not fail the gate.
	PassExit []int
	// Requires names paths the step cannot run without, such as installed
	// dependencies; a missing one skips the step as an environment problem.
	Requires []string
	// UnavailableExit lists exit statuses that mean the step could not run at
	// all (127: a command inside it was not found), not that the code failed.
	UnavailableExit []int
}

// Plan picks the build and test commands that cover the changed files. It
// only runs a toolchain whose own sources changed: an edited README asks for
// nothing, an edited Go file asks for the build and the tests of its package.
func Plan(root string, changed []string) []Step {
	var steps []Step
	if fileExists(filepath.Join(root, "go.mod")) {
		if pkgs := goPackages(root, changed); len(pkgs) > 0 {
			steps = append(steps,
				Step{Name: "go build", Argv: []string{"go", "build", "./..."}, Dir: root},
				Step{Name: "go test", Argv: append([]string{"go", "test"}, pkgs...), Dir: root})
		}
	}
	if hasExt(changed, nodeExts...) {
		steps = append(steps, nodeSteps(root)...)
	}
	if hasExt(changed, ".py") && hasPytest(root) {
		steps = append(steps, Step{Name: "pytest", Argv: []string{"python3", "-m", "pytest", "-q"}, Dir: root})
	}
	if hasExt(changed, ".rs") && fileExists(filepath.Join(root, "Cargo.toml")) {
		steps = append(steps, Step{Name: "cargo test", Argv: []string{"cargo", "test", "--quiet"}, Dir: root})
	}
	if len(steps) > 0 {
		if step, ok := qaStep(root); ok {
			steps = append(steps, step)
		}
	}
	return steps
}

// goPackages returns the ./dir patterns of changed Go files whose directory
// still holds Go sources. A change under testdata or vendor is not a package.
func goPackages(root string, changed []string) []string {
	var pkgs []string
	for _, f := range changed {
		if path.Ext(f) != ".go" || strings.Contains("/"+f, "/testdata/") || strings.HasPrefix(f, "vendor/") {
			continue
		}
		dir := path.Dir(f)
		matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
		if len(matches) == 0 {
			continue
		}
		pattern := "./" + dir
		if dir == "." {
			pattern = "."
		}
		if !slices.Contains(pkgs, pattern) {
			pkgs = append(pkgs, pattern)
		}
	}
	slices.Sort(pkgs)
	return pkgs
}

// nodeSteps runs the package's own typecheck and test scripts with the
// package manager the project declares: the packageManager field first, then
// a pnpm workspace file, then the lockfile. A declared pnpm or yarn that is
// not on PATH runs through corepack, which is how such projects usually get
// it. The steps need installed dependencies; without node_modules they are
// skipped as an environment problem instead of failing the gate.
func nodeSteps(root string) []Step {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts        map[string]string `json:"scripts"`
		PackageManager string            `json:"packageManager"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	manager := nodeManager(root, pkg.PackageManager)
	prefix := []string{manager}
	if _, err := lookPath(manager); err != nil && manager != "npm" && manager != "bun" {
		if _, err := lookPath("corepack"); err == nil {
			prefix = []string{"corepack", manager}
		}
	}
	var steps []Step
	for _, script := range []string{"typecheck", "test"} {
		if _, ok := pkg.Scripts[script]; ok {
			steps = append(steps, Step{
				Name: manager + " run " + script, Argv: append(append([]string{}, prefix...), "run", script), Dir: root,
				Requires: []string{filepath.Join(root, "node_modules")}, UnavailableExit: []int{127},
			})
		}
	}
	return steps
}

// lookPath finds a tool on PATH; tests replace it to fix the machine.
var lookPath = exec.LookPath

// nodeManager names the package manager a project declares.
func nodeManager(root, declared string) string {
	if name, _, _ := strings.Cut(strings.TrimSpace(declared), "@"); slices.Contains([]string{"npm", "pnpm", "yarn", "bun"}, name) {
		return name
	}
	switch {
	case fileExists(filepath.Join(root, "pnpm-workspace.yaml")), fileExists(filepath.Join(root, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(root, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(root, "bun.lock")), fileExists(filepath.Join(root, "bun.lockb")):
		return "bun"
	}
	return "npm"
}

var nodeExts = []string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".vue", ".svelte"}

// IsSource reports whether a changed file belongs to a toolchain the gate
// verifies.
func IsSource(file string) bool {
	return hasExt([]string{file}, append([]string{".go", ".py", ".rs"}, nodeExts...)...)
}

func hasPytest(root string) bool {
	if fileExists(filepath.Join(root, "pytest.ini")) || fileExists(filepath.Join(root, "conftest.py")) {
		return true
	}
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	return err == nil && strings.Contains(string(data), "pytest")
}

func hasExt(files []string, exts ...string) bool {
	return slices.ContainsFunc(files, func(f string) bool { return slices.Contains(exts, path.Ext(f)) })
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
