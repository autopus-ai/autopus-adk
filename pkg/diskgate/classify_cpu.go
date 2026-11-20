package diskgate

import (
	"path/filepath"
	"slices"
	"strings"
)

// ClassifyCPU reports whether a simple command in line would load the CPU:
// the disk gate's compiles and tests, less the installs and checkouts that
// mostly wait on the network and disk, plus linters, type checkers and test
// runners. `auto disk lease` without --cpu reserves no CPU, so the command
// after its `--` is classified; every other `auto` command passes. extra
// adds word prefixes, as in Classify.
func ClassifyCPU(line string, extra ...string) Verdict {
	var prefixes [][]string
	for _, command := range extra {
		if words := strings.Fields(command); len(words) > 0 {
			prefixes = append(prefixes, words)
		}
	}
	return classifyCPU(line, prefixes, 0)
}

func classifyCPU(line string, extra [][]string, depth int) Verdict {
	for _, words := range segments(lex(line)) {
		words = stripPrefixes(words)
		if inner, ok := leaseWithoutCPU(words); ok {
			words = inner
		}
		if len(words) == 0 {
			continue
		}
		name, args := filepath.Base(words[0]), words[1:]
		if (name == "sh" || name == "bash" || name == "zsh" || name == "dash") && depth < maxShellDepth {
			if i := slices.Index(args, "-c"); i >= 0 && i+1 < len(args) {
				if v := classifyCPU(args[i+1], extra, depth+1); v.Heavy {
					return v
				}
			}
			continue
		}
		if v := classifyWords(words, extra, depth); v.Heavy && !ioBound(v.Match) {
			return v
		}
		if match := cpuStep(name, args); match != "" {
			return Verdict{Heavy: true, Match: match}
		}
	}
	return Verdict{}
}

// leaseWithoutCPU returns the command of an `auto disk lease` that reserves
// no CPU slot (no --cpu), which runs as unleased as far as the CPU goes.
func leaseWithoutCPU(words []string) ([]string, bool) {
	if len(words) < 3 || filepath.Base(words[0]) != "auto" || words[1] != "disk" || words[2] != "lease" {
		return nil, false
	}
	sep := slices.Index(words, "--")
	if sep < 0 {
		return nil, false
	}
	for _, w := range words[3:sep] {
		if w == "--cpu" || strings.HasPrefix(w, "--cpu=") {
			return nil, false
		}
	}
	return words[sep+1:], true
}

// ioBound names the disk gate's heavy steps that wait on the network and
// disk rather than the CPU: package installs and syncs, and checkouts. go
// install and cargo install compile, so they stay heavy.
func ioBound(match string) bool {
	if match == "git worktree add" || match == "yarn install" {
		return true
	}
	if tool, _, _ := strings.Cut(match, " "); tool == "go" || tool == "cargo" {
		return false
	}
	for _, sub := range []string{" install", " i", " ci", " add", " sync"} {
		if strings.HasSuffix(match, sub) {
			return true
		}
	}
	return false
}

// cpuTools are linters, type checkers and test runners run directly.
var cpuTools = []string{"eslint", "vitest", "jest", "tsc", "pytest", "mocha", "playwright"}

// nodeFlagsWithValue are package-manager options followed by a value.
var nodeFlagsWithValue = []string{"--prefix", "-C", "--dir", "--filter", "-F", "--cwd", "-w", "--workspace"}

// cpuStep names the CPU-heavy step that is not on the disk gate's list.
func cpuStep(name string, args []string) string {
	switch {
	case slices.Contains(cpuTools, name):
		if onlyInfoFlags(args) {
			return ""
		}
		return name
	case name == "npx" || name == "bunx":
		if tool, rest := subcommand(args, "-p", "--package"); slices.Contains(cpuTools, tool) && !onlyInfoFlags(rest) {
			return name + " " + tool
		}
	case name == "npm" || name == "pnpm" || name == "yarn" || name == "bun":
		return nodeCPUStep(name, args)
	case name == "uv" || name == "poetry":
		if sub, rest := subcommand(args, "--directory", "--project", "-C"); sub == "run" {
			if tool, _ := subcommand(rest); tool != "" {
				return cpuStep(filepath.Base(tool), rest[1:])
			}
		}
	case name == "python" || name == "python3":
		if len(args) >= 2 && args[0] == "-m" && slices.Contains([]string{"pytest", "mypy"}, args[1]) {
			return name + " -m " + args[1]
		}
	case name == "go":
		if sub, _ := subcommand(args, "-C"); sub == "vet" {
			return "go vet"
		}
	case name == "cargo":
		if sub, _ := subcommand(args, "-C", "--manifest-path", "--config", "-Z"); sub == "clippy" || sub == "check" {
			return "cargo " + sub
		}
	case name == "golangci-lint":
		if sub, _ := subcommand(args); sub == "run" {
			return "golangci-lint run"
		}
	}
	return ""
}

// nodeCPUStep catches lint, typecheck and test scripts with options in
// front (`pnpm -r lint`, `npm --prefix web run lint:fix`) and package
// binaries run through the manager (`pnpm exec vitest`, `pnpm vitest`).
func nodeCPUStep(name string, args []string) string {
	sub, rest := subcommand(args, nodeFlagsWithValue...)
	if sub == "run" || sub == "run-script" {
		sub, _ = subcommand(rest, nodeFlagsWithValue...)
		if cpuScript(sub) {
			return name + " run " + sub
		}
		return ""
	}
	if sub == "exec" || sub == "dlx" || slices.Contains(cpuTools, sub) {
		tool := sub
		if sub == "exec" || sub == "dlx" {
			tool, rest = subcommand(rest)
		}
		if slices.Contains(cpuTools, tool) && !onlyInfoFlags(rest) {
			return strings.TrimSpace(name + " " + strings.TrimPrefix(sub+" ", tool+" ") + tool)
		}
		return ""
	}
	if cpuScript(sub) {
		return name + " " + sub
	}
	return ""
}

// cpuScript reports whether a package script name is a lint, type check or
// test script, variants such as lint:fix and test:unit included.
func cpuScript(script string) bool {
	for _, prefix := range []string{"lint", "typecheck", "type-check", "check-types", "test"} {
		if script == prefix || strings.HasPrefix(script, prefix+":") || strings.HasPrefix(script, prefix+"-") {
			return true
		}
	}
	return false
}

// onlyInfoFlags reports whether args only ask a tool for its version or
// help, which ends at once.
func onlyInfoFlags(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, a := range args {
		if !slices.Contains([]string{"--version", "-v", "-V", "--help", "-h"}, a) {
			return false
		}
	}
	return true
}
