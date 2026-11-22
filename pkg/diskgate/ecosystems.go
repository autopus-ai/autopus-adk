package diskgate

import (
	"path/filepath"
	"slices"
	"strings"
)

// nodeFlagsWithValue are package-manager options followed by a value.
var nodeFlagsWithValue = []string{"--prefix", "-C", "--dir", "--filter", "-F", "--cwd", "-w", "--workspace"}

// nodeArgs drops `yarn workspace <name>`, which runs the rest in one
// workspace, so `yarn workspace web test` reads as `yarn test`.
func nodeArgs(name string, args []string) []string {
	if name != "yarn" {
		return args
	}
	if sub, rest := subcommand(args, nodeFlagsWithValue...); sub == "workspace" && len(rest) > 1 {
		return rest[1:]
	}
	return args
}

// nodeStep classifies npm, pnpm, yarn, and bun: installs, tests, and build
// scripts. Installs and adds wait on the network and disk; tests, rebuilds,
// and builds load the CPU.
func nodeStep(name string, args []string) step {
	sub, rest := subcommand(nodeArgs(name, args), nodeFlagsWithValue...)
	if sub == "" {
		if name == "yarn" {
			return ioKind("yarn install")
		}
		return step{}
	}
	if slices.Contains([]string{"install", "i", "ci", "add"}, sub) {
		return ioKind(name + " " + sub)
	}
	if slices.Contains([]string{"test", "t", "rebuild"}, sub) || (name != "npm" && sub == "build") {
		return cpuKind(name + " " + sub)
	}
	if sub == "run" || sub == "run-script" {
		if script, _ := subcommand(rest); strings.HasPrefix(script, "build") || strings.HasPrefix(script, "test") {
			return cpuKind(name + " run " + script)
		}
	}
	return step{}
}

// pythonStep classifies package installs and environment syncs: pip
// install, uv sync / uv pip install / uv add, poetry install, and
// python -m pip install.
func pythonStep(name string, args []string) string {
	switch name {
	case "python", "python3":
		if len(args) >= 3 && args[0] == "-m" && (args[1] == "pip" || args[1] == "pip3") && args[2] == "install" {
			return name + " -m pip install"
		}
	case "pip", "pip3":
		if sub, _ := subcommand(args); sub == "install" {
			return name + " install"
		}
	case "uv":
		sub, rest := subcommand(args, "--directory", "--project", "--cache-dir")
		next, _ := subcommand(rest)
		switch {
		case sub == "sync" || sub == "add":
			return "uv " + sub
		case sub == "pip" && (next == "install" || next == "sync"):
			return "uv pip " + next
		}
	case "poetry":
		if sub, _ := subcommand(args, "-C", "--directory"); sub == "install" {
			return "poetry install"
		}
	}
	return ""
}

// jvmTasks are the Gradle and Maven tasks and phases that compile.
var jvmTasks = []string{"build", "assemble", "test", "check", "compile", "package", "install", "verify", "bootJar", "jar"}

// jvmStep classifies Gradle and Maven runs, wrappers included, by their
// first compiling task or phase.
func jvmStep(name string, args []string) string {
	tool := strings.TrimSuffix(filepath.Base(name), "w")
	for _, arg := range args {
		task := arg[strings.LastIndex(arg, ":")+1:]
		if !strings.HasPrefix(arg, "-") && slices.Contains(jvmTasks, task) {
			return tool + " " + arg
		}
	}
	return ""
}
