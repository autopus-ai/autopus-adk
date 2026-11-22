package diskgate

import "slices"

// cpuKind and ioKind build the step a match names.
func cpuKind(match string) step { return step{match: match} }
func ioKind(match string) step  { return step{match: match, io: true} }

// heavyStep names the heavy step name and args run, or returns a zero step.
func heavyStep(name string, args []string) step {
	switch name {
	case "go":
		sub, _ := subcommand(args, "-C")
		if slices.Contains([]string{"build", "test", "install", "run"}, sub) {
			return cpuKind("go " + sub)
		}
	case "cargo":
		sub, rest := subcommand(args, "-C", "--manifest-path", "--config", "-Z")
		if slices.Contains([]string{"build", "b", "test", "t", "run", "r", "bench", "install"}, sub) {
			return cpuKind("cargo " + sub)
		}
		if next, _ := subcommand(rest); sub == "nextest" && next == "run" {
			return cpuKind("cargo nextest run")
		}
	case "npm", "pnpm", "yarn", "bun":
		return nodeStep(name, args)
	case "pip", "pip3", "uv", "poetry", "python", "python3":
		if match := pythonStep(name, args); match != "" {
			return ioKind(match)
		}
	case "gradle", "gradlew", "mvn", "mvnw":
		if match := jvmStep(name, args); match != "" {
			return cpuKind(match)
		}
	case "podman":
		sub, rest := subcommand(args, "--connection", "-c", "--url", "--log-level")
		next, _ := subcommand(rest, "-f", "--file", "-p", "--project-name")
		switch {
		case sub == "build":
			return cpuKind("podman build")
		case sub == "compose" && (next == "up" || next == "build"):
			return cpuKind("podman compose " + next)
		}
	case "podman-compose":
		if sub, _ := subcommand(args, "-f", "--file", "-p", "--project-name"); sub == "up" || sub == "build" {
			return cpuKind("podman-compose " + sub)
		}
	case "docker":
		sub, rest := subcommand(args, "--context", "-c", "-H", "--host", "--config", "-l", "--log-level")
		switch sub {
		case "build":
			return cpuKind("docker build")
		case "buildx", "compose":
			if next, _ := subcommand(rest, "-f", "--file", "-p", "--project-name", "--builder"); next == "build" || (sub == "compose" && next == "up") {
				return cpuKind("docker " + sub + " " + next)
			}
		}
	case "docker-compose":
		if sub, _ := subcommand(args, "-f", "--file", "-p", "--project-name"); sub == "up" || sub == "build" {
			return cpuKind("docker-compose " + sub)
		}
	case "xcodebuild":
		for _, a := range args {
			if slices.Contains([]string{"-version", "-list", "-showsdks", "-showBuildSettings", "-help", "-usage"}, a) {
				return step{}
			}
		}
		return cpuKind("xcodebuild")
	case "git":
		sub, rest := subcommand(args, "-C", "-c", "--git-dir", "--work-tree", "--namespace")
		if next, _ := subcommand(rest); sub == "worktree" && next == "add" {
			return ioKind("git worktree add")
		}
	}
	return step{}
}
