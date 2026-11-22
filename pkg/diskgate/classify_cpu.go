package diskgate

import (
	"path/filepath"
	"slices"
	"strings"
)

// ClassifyCPU reports whether a simple command in line would load the CPU:
// the disk gate's compiles and tests, less the installs and checkouts that
// mostly wait on the network and disk, plus linters, type checkers, test
// runners, and the `auto` steps that run suites. `auto disk lease` without
// --cpu reserves no CPU, so the command after its `--` is classified; every
// other `auto` command passes. extra adds word prefixes, as in Classify;
// they count as CPU work, so `make ci` added there is gated.
func ClassifyCPU(line string, extra ...string) Verdict {
	return classifyCPU(line, parsePrefixes(extra), 0)
}

func classifyCPU(line string, extra [][]string, depth int) Verdict {
	for _, words := range segments(lex(line)) {
		words = stripPrefixes(words)
		if inner, ok := leaseWithoutCPU(words); ok {
			words = stripPrefixes(inner)
		}
		if len(words) == 0 {
			continue
		}
		name, args := filepath.Base(words[0]), words[1:]
		switch name {
		case "auto":
			if match := autoCPUStep(args); match != "" {
				return Verdict{Heavy: true, Match: match}
			}
			continue
		case "sh", "bash", "zsh", "dash":
			if script, ok := shellScript(args); ok && depth < maxShellDepth {
				if v := classifyCPU(script, extra, depth+1); v.Heavy {
					return v
				}
			}
			continue
		}
		if s := classifyWords(words, extra, depth); s.match != "" && !s.io {
			return Verdict{Heavy: true, Match: s.match}
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

// autoCPUStep names the `auto` subcommands that run whole suites: the QA
// runners, checklist runs, and workflow gates.
func autoCPUStep(args []string) string {
	sub, rest := subcommand(args)
	next, after := subcommand(rest)
	if onlyHelpFlags(after) {
		return ""
	}
	switch {
	case sub == "qa" && slices.Contains([]string{"exec", "run", "loop", "mutate"}, next),
		sub == "checklist" && next == "run", sub == "workflow" && next == "gate":
		return "auto " + sub + " " + next
	}
	return ""
}

// onlyHelpFlags reports whether args are nothing but -h, --help and
// --version, which print and exit without running the step.
func onlyHelpFlags(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, a := range args {
		if a != "-h" && a != "--help" && a != "--version" {
			return false
		}
	}
	return true
}

// uvRunValued are the `uv run` and `poetry run` options followed by a value.
var uvRunValued = []string{"--with", "-w", "--with-requirements", "--with-editable", "--python", "-p",
	"--directory", "--project", "-P", "-C", "--package", "--extra", "--group", "--only-group", "--no-group",
	"--env-file", "--index", "--default-index", "--index-url", "-i", "--extra-index-url", "--find-links", "-f",
	"--cache-dir", "--config-file"}

// cpuStep names the CPU-heavy step that is not on the disk gate's list.
func cpuStep(name string, args []string) string {
	switch {
	case slices.Contains(cpuTools, name):
		if cpuToolRun(name, args) {
			return name
		}
	case name == "npx" || name == "bunx":
		if tool, rest := subcommand(args, "-p", "--package"); tool != "" {
			if match := cpuStep(filepath.Base(tool), rest); match != "" {
				return name + " " + match
			}
		}
	case name == "npm" || name == "pnpm" || name == "yarn" || name == "bun":
		return nodeCPUStep(name, args)
	case name == "uv" || name == "poetry":
		if sub, rest := subcommand(args, "--directory", "--project", "-C", "-P"); sub == "run" {
			if tool, toolArgs := subcommand(rest, uvRunValued...); tool != "" {
				return cpuStep(filepath.Base(tool), toolArgs)
			}
		}
	case name == "python" || name == "python3":
		if len(args) >= 2 && args[0] == "-m" && slices.Contains([]string{"pytest", "mypy"}, args[1]) {
			return name + " -m " + args[1]
		}
	case name == "go":
		sub, rest := subcommand(args, "-C")
		if sub == "vet" {
			return "go vet"
		}
		if tool, toolArgs := subcommand(rest); sub == "tool" && tool != "" {
			if match := cpuStep(filepath.Base(tool), toolArgs); match != "" {
				return "go tool " + match
			}
		}
	case name == "cargo":
		if sub, _ := subcommand(args, "-C", "--manifest-path", "--config", "-Z"); sub == "clippy" || sub == "check" {
			return "cargo " + sub
		}
	case name == "golangci-lint":
		if sub, _ := subcommand(args); sub == "run" {
			return "golangci-lint run"
		}
	case name == "turbo":
		return turboStep(args)
	}
	return ""
}
