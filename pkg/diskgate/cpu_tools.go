package diskgate

import (
	"path/filepath"
	"slices"
	"strings"
)

// cpuTools are linters, type checkers and test runners run directly.
var cpuTools = []string{"eslint", "vitest", "jest", "tsc", "pytest", "mocha", "playwright", "mypy"}

// versionShort is the short version flag of each cpuTool. pytest -v and
// mypy -v ask for verbose output, not the version, so they run the tool.
var versionShort = map[string]string{
	"eslint": "-v", "vitest": "-v", "jest": "-v", "tsc": "-v",
	"pytest": "-V", "mocha": "-V", "playwright": "-V", "mypy": "-V",
}

// cpuToolRun reports whether tool with args does its work: neither asks
// only for its version or help, nor, for playwright, does anything but run
// tests (`playwright install` downloads browsers).
func cpuToolRun(tool string, args []string) bool {
	if !slices.Contains(cpuTools, tool) || onlyInfoFlags(tool, args) {
		return false
	}
	if tool == "playwright" {
		sub, _ := subcommand(args, "-c", "--config")
		return sub == "test"
	}
	return true
}

// onlyInfoFlags reports whether args only ask tool for its version or help,
// which ends at once.
func onlyInfoFlags(tool string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	info := []string{"--version", "--help", "-h"}
	if short, ok := versionShort[tool]; ok {
		info = append(info, short)
	}
	for _, a := range args {
		if !slices.Contains(info, a) {
			return false
		}
	}
	return true
}

// nodeCPUStep catches lint, typecheck and test scripts with options in
// front (`pnpm -r lint`, `npm --prefix web run lint:fix`), package binaries
// run through the manager (`pnpm exec vitest`, `pnpm vitest`), and
// `yarn workspace web lint`.
func nodeCPUStep(name string, args []string) string {
	sub, rest := subcommand(nodeArgs(name, args), nodeFlagsWithValue...)
	switch {
	case sub == "run" || sub == "run-script":
		sub, _ = subcommand(rest, nodeFlagsWithValue...)
		if cpuScript(sub) {
			return name + " run " + sub
		}
	case sub == "exec" || sub == "dlx":
		if tool, toolArgs := subcommand(rest); tool != "" {
			if match := cpuStep(filepath.Base(tool), toolArgs); match != "" {
				return name + " " + sub + " " + match
			}
		}
	case slices.Contains(cpuTools, sub):
		if cpuToolRun(sub, rest) {
			return name + " " + sub
		}
	case cpuScript(sub):
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

// turboValued are the turbo options followed by a value.
var turboValued = []string{"--filter", "-F", "--concurrency", "--cache-dir", "--output-logs", "--log-order",
	"--env-mode", "--cwd", "--graph", "--profile"}

// turboStep names a turbo run of a build, lint, type check or test task:
// `turbo run test`, `turbo test lint`.
func turboStep(args []string) string {
	sub, rest := subcommand(args, turboValued...)
	prefix, tasks := "turbo ", []string{sub}
	if sub == "run" {
		prefix, tasks = "turbo run ", nil
		for task, more := subcommand(rest, turboValued...); task != ""; task, more = subcommand(more, turboValued...) {
			tasks = append(tasks, task)
		}
	}
	for _, task := range tasks {
		if cpuScript(task) || strings.HasPrefix(task, "build") {
			return prefix + task
		}
	}
	return ""
}
