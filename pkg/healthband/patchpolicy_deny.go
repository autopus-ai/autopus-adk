package healthband

import (
	"path"
	"strings"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

// Patch Policy items 5–6: the extension allowlist, then the deny list, the
// structural rules (CD-3 M3), the executable rule, and the paths that the
// base manifests make a build run (RR-7, patchpolicy_build.go), each giving
// path_denied. Deny entries match any path segment or the file name after
// folding; the tool-configuration name rules match the file name only.

// allowedExtensions is item 5's allowlist, matched exactly on the file name.
var allowedExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".py": true,
	".rs": true, ".java": true, ".kt": true, ".rb": true, ".php": true, ".cs": true, ".swift": true, ".c": true,
	".h": true, ".cc": true, ".cpp": true, ".hpp": true,
}

// deniedSegments are item 6's patterns, folded; each is matched with
// path.Match against every segment of the folded path. The dot entries also
// fall under the structural dot rule and stay listed for clarity.
var deniedSegments = []string{
	// CI, hooks, harness, editor, and container directories.
	".github", ".gitlab", ".circleci", ".buildkite", ".husky", ".githooks", ".lefthook", ".autopus", ".omp",
	".agents", ".cursor", ".windsurf", ".vscode", ".idea", ".devcontainer",
	// Scripts, included builds, vendored code, and package managers.
	"scripts", "buildsrc", "build-logic", "vendor", "node_modules", ".yarn",
	// Build, dependency, agent, and repository files.
	"makefile", "gnumakefile", "dockerfile*", "docker-compose*", "package.json", "go.mod", "go.sum", "agents.md",
	"claude.md", "gemini.md", ".gitattributes", ".gitmodules", ".pre-commit-config.yaml", "lefthook.yml", "lefthook.yaml",
	// Dotenv and credential paths.
	".env*", "*.env", ".envrc", ".netrc", ".npmrc", ".pypirc", ".git-credentials", ".aws", ".ssh", ".gnupg", ".kube",
	"kubeconfig*", "secrets.*", "*secret*", "*credentials*", "*.pem", "*.key", "id_rsa*", "id_ed25519*", "*.p12",
	"*.pfx", "*.keystore", "*.jks",
	// Files that tools load or run on their own.
	"build.rs", "conftest.py", "setup.py", "noxfile.py", "sitecustomize.py", "usercustomize.py", "*.config.js",
	"*.config.cjs", "*.config.mjs", "*.config.ts", "*.config.cts", "*.config.mts", ".eslintrc.*", ".prettierrc.*",
	".pnpmfile.cjs", "package.swift", "gulpfile.*", "gruntfile.*", "magefile.go", "dangerfile.*", ".pnp.*",
}

// deniedFileNames are the structural tool-configuration rules, folded and
// matched on the file name; *rc.* also denies a name such as src.go, an
// intended fail-closed limitation.
var deniedFileNames = []string{"*.conf.*", "*rc.*", ".*rc"}

// deniedPath runs items 5 and 6 over every path.
func deniedPath(files []*diffFile, entries map[string]treeEntry, built buildPaths) string {
	for _, file := range files {
		if !allowedExtensions[path.Ext(path.Base(file.path))] {
			return PatchCodePathDenied
		}
	}
	for _, file := range files {
		if deniedName(foldPath(file.path)) || (!file.isNew && entries[file.path].mode == "100755") || built.denies(file.path) {
			return PatchCodePathDenied
		}
	}
	return ""
}

// deniedName applies item 6 to a folded path.
func deniedName(folded string) bool {
	segments := strings.Split(folded, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ".") || matchesAny(deniedSegments, segment) || generatedSurface(strings.Join(segments[i:], "/")) {
			return true
		}
	}
	return matchesAny(deniedFileNames, segments[len(segments)-1])
}

// generatedSurface reports a generated surface of workflow's drift gate at
// a segment boundary, compared after folding.
func generatedSurface(rest string) bool {
	for _, prefix := range workflow.GeneratedSurfacePrefixes {
		if strings.HasPrefix(rest, foldPath(prefix)) {
			return true
		}
	}
	for _, exact := range workflow.GeneratedSurfaceExactPaths {
		if rest == foldPath(exact) {
			return true
		}
	}
	return false
}

func matchesAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if matched, _ := path.Match(pattern, name); matched {
			return true
		}
	}
	return false
}
