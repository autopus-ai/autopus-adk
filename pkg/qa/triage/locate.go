package triage

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

const (
	// generatedDirName is the directory the compiler writes specs into,
	// directly under the Playwright testDir.
	generatedDirName = "autopus-generated"
	// maxSearchDepth bounds the walk for generated directories, so a large
	// monorepo costs a few directory reads rather than a crawl.
	maxSearchDepth = 5
	// messageLookback bounds how far above a cited line the failure message
	// is collected; Playwright prints the message, the call log, and the code
	// frame above the stack line.
	messageLookback = 40
	stackLookahead  = 30
)

var (
	// citedLocation matches `path/x.spec.ts:29:5` and `path/x.spec.ts:29`.
	citedLocation = regexp.MustCompile(`([^\s'"()\[\]<>,]+\.(?:ts|tsx|js|jsx|mjs|cjs)):(\d+)(?::\d+)?`)
	// codeFrameMarker matches Playwright's `>  29 |` failing-line marker.
	codeFrameMarker = regexp.MustCompile(`^\s*>\s*(\d+)\s*\|`)
	// codeFrameRow matches any code-frame row, marked or not, and the caret row.
	codeFrameRow = regexp.MustCompile(`^\s*>?\s*\d*\s*\|`)
	// failureHeader matches a reporter's numbered failure header.
	failureHeader = regexp.MustCompile(`^\s*\d+\) `)
	skippedDirs   = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "coverage": true}
)

// stepHit is a failing generated-spec line the step map knows.
type stepHit struct {
	// SpecPath is project-relative and slash-separated when the spec lies
	// under the project, otherwise the path as the runner cited it.
	SpecPath   string
	Line       int
	Step       scenario.StepRef
	ScenarioID string
	// Window is the runner output from the failure header to the cited line.
	Window string
	// Message is Window without code-frame rows: what the runner said, not
	// the neighbouring source it quoted.
	Message string
}

type citation struct {
	path string
	line int
	at   int
}

// locateGeneratedStep returns the first cited location that a compiled step
// map knows. Explicit `path:line` citations come before code-frame markers:
// a marker is tied to the last path printed above it, which is only a guess
// when the failing frame sits in a helper file.
func locateGeneratedStep(projectDir string, lines []string) (stepHit, bool) {
	finder := &specFinder{projectDir: projectDir, maps: map[string]*scenario.StepMap{}}
	for _, cite := range citations(lines) {
		cited := normalizeSlashes(cite.path)
		if !strings.Contains(cited, generatedDirName+"/") {
			continue
		}
		spec, stepMap, ok := finder.find(cited)
		if !ok {
			continue
		}
		ref, ok := stepMap.Lookup(cite.line)
		if !ok {
			continue
		}
		window := failureWindow(lines, cite.at)
		return stepHit{
			SpecPath: finder.display(spec, cited), Line: cite.line, Step: ref, ScenarioID: stepMap.ScenarioID,
			Window: window, Message: withoutCodeFrame(window),
		}, true
	}
	return stepHit{}, false
}

func citations(lines []string) []citation {
	explicit := make([]citation, 0)
	framed := make([]citation, 0)
	lastPath := ""
	for i, line := range lines {
		if marker := codeFrameMarker.FindStringSubmatch(line); marker != nil {
			if n, err := strconv.Atoi(marker[1]); err == nil && lastPath != "" {
				framed = append(framed, citation{path: lastPath, line: n, at: i})
			}
			continue
		}
		if codeFrameRow.MatchString(line) {
			continue // quoted source, not a location
		}
		for _, match := range citedLocation.FindAllStringSubmatch(line, -1) {
			n, err := strconv.Atoi(match[2])
			if err != nil {
				continue
			}
			path := strings.TrimPrefix(match[1], "file://")
			explicit = append(explicit, citation{path: path, line: n, at: i})
			lastPath = path
		}
	}
	return append(explicit, framed...)
}

func failureWindow(lines []string, at int) string {
	start := at
	for start > 0 && at-start < messageLookback && !failureHeader.MatchString(lines[start]) {
		start--
	}
	end := at + 1
	for end < len(lines) && end <= at+2 && !failureHeader.MatchString(lines[end]) {
		end++
	}
	return strings.Join(lines[start:end], "\n")
}

func withoutCodeFrame(window string) string {
	kept := make([]string, 0)
	for _, line := range strings.Split(window, "\n") {
		if !codeFrameRow.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// specFinder resolves a cited generated spec to the spec whose step map
// exists. Runners cite specs absolute, relative to the project, or relative
// to the Playwright testDir, and redaction can rewrite an absolute path into
// one that no longer exists, so every form falls back to the generated
// directories found under the project.
type specFinder struct {
	projectDir string
	roots      []string
	walked     bool
	maps       map[string]*scenario.StepMap
}

func (f *specFinder) find(cited string) (string, scenario.StepMap, bool) {
	for _, candidate := range f.candidates(cited) {
		if stepMap, ok := f.load(candidate); ok {
			return candidate, stepMap, true
		}
	}
	return "", scenario.StepMap{}, false
}

func (f *specFinder) load(spec string) (scenario.StepMap, bool) {
	cached, seen := f.maps[spec]
	if !seen {
		if loaded, err := scenario.LoadStepMap(scenario.StepMapPath(spec)); err == nil {
			cached = &loaded
		}
		f.maps[spec] = cached
	}
	if cached == nil {
		return scenario.StepMap{}, false
	}
	return *cached, true
}

func (f *specFinder) candidates(cited string) []string {
	native := filepath.FromSlash(cited)
	out := make([]string, 0, 4)
	if filepath.IsAbs(native) {
		out = append(out, native)
	} else if f.projectDir != "" {
		out = append(out, filepath.Join(f.projectDir, native))
	}
	if f.projectDir == "" {
		return out
	}
	idx := strings.LastIndex(cited, generatedDirName+"/")
	rest := filepath.FromSlash(cited[idx+len(generatedDirName)+1:])
	for _, root := range f.generatedRoots() {
		out = append(out, filepath.Join(root, rest))
	}
	return out
}

// generatedRoots lists every generated directory under the project, in
// lexical order, skipping dependency, build, and hidden trees.
func (f *specFinder) generatedRoots() []string {
	if f.walked {
		return f.roots
	}
	f.walked = true
	root := filepath.Clean(f.projectDir)
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || path == root {
			return nil
		}
		name := entry.Name()
		if name == generatedDirName {
			f.roots = append(f.roots, path)
			return fs.SkipDir
		}
		if skippedDirs[name] || strings.HasPrefix(name, ".") || depthBelow(root, path) >= maxSearchDepth {
			return fs.SkipDir
		}
		return nil
	})
	return f.roots
}

func (f *specFinder) display(spec, cited string) string {
	if f.projectDir == "" {
		return cited
	}
	rel, err := filepath.Rel(realPath(f.projectDir), realPath(spec))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return cited
	}
	return filepath.ToSlash(rel)
}

func depthBelow(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return maxSearchDepth
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}

func realPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

func normalizeSlashes(path string) string {
	return strings.ReplaceAll(path, `\`, "/")
}
