package diskgate

import (
	"path/filepath"
	"slices"
	"strings"
)

// nodeStep classifies npm, pnpm, yarn, and bun: installs, tests, and build
// scripts.
func nodeStep(name string, args []string) string {
	sub, rest := subcommand(args, "--prefix", "-C", "--dir", "--filter", "-F", "--cwd", "-w", "--workspace")
	if sub == "" {
		if name == "yarn" {
			return "yarn install"
		}
		return ""
	}
	installs := []string{"install", "i", "ci", "add", "test", "t", "rebuild"}
	if name != "npm" {
		installs = append(installs, "build")
	}
	if slices.Contains(installs, sub) {
		return name + " " + sub
	}
	if sub == "run" || sub == "run-script" {
		if script, _ := subcommand(rest); strings.HasPrefix(script, "build") || strings.HasPrefix(script, "test") {
			return name + " run " + script
		}
	}
	return ""
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
