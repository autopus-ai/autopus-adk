package diskgate

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// maxShellDepth bounds how many nested `sh -c` strings are classified.
const maxShellDepth = 4

// Verdict is what Classify found. Match names the first heavy step, such as
// "go test", and is empty when Heavy is false.
type Verdict struct {
	Heavy bool
	Match string
}

// envAssign is a NAME=value prefix word.
var envAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// Classify reports whether any simple command in line is heavy. Every
// `auto` command is allowed, `auto disk lease -- <cmd>` included: a lease is
// the sanctioned way to run heavy work while space is short. extra adds
// word prefixes such as "make release" to the built-in list; make itself is
// not heavy by default, because most of its targets are not.
func Classify(line string, extra ...string) Verdict {
	var prefixes [][]string
	for _, command := range extra {
		if words := strings.Fields(command); len(words) > 0 {
			prefixes = append(prefixes, words)
		}
	}
	return classify(line, prefixes, 0)
}

func classify(line string, extra [][]string, depth int) Verdict {
	for _, words := range segments(lex(line)) {
		if v := classifyWords(words, extra, depth); v.Heavy {
			return v
		}
	}
	return Verdict{}
}

func classifyWords(words []string, extra [][]string, depth int) Verdict {
	words = stripPrefixes(words)
	if len(words) == 0 {
		return Verdict{}
	}
	name := filepath.Base(words[0])
	args := words[1:]
	switch name {
	case "auto":
		return Verdict{}
	case "sh", "bash", "zsh", "dash":
		if i := slices.Index(args, "-c"); i >= 0 && i+1 < len(args) && depth < maxShellDepth {
			return classify(args[i+1], extra, depth+1)
		}
		return Verdict{}
	}
	if match := heavyStep(name, args); match != "" {
		return Verdict{Heavy: true, Match: match}
	}
	for _, prefix := range extra {
		if matchesPrefix(name, args, prefix) {
			return Verdict{Heavy: true, Match: strings.Join(prefix, " ")}
		}
	}
	return Verdict{}
}

// matchesPrefix reports whether the command name args starts with the words
// of prefix, the first compared by base name.
func matchesPrefix(name string, args, prefix []string) bool {
	if filepath.Base(prefix[0]) != name || len(args) < len(prefix)-1 {
		return false
	}
	for i, word := range prefix[1:] {
		if args[i] != word {
			return false
		}
	}
	return true
}

// stripPrefixes drops what runs before the command proper: `!`, NAME=value
// assignments, and wrappers such as env, time, nice, nohup, command, exec,
// sudo, and timeout, with their options.
func stripPrefixes(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		switch {
		case w == "!" || w == "{" || w == "}" || envAssign.MatchString(w):
			words = words[1:]
		case w == "env" || w == "command" || w == "exec" || w == "nohup" || w == "time" || w == "sudo" || w == "caffeinate":
			words = words[1:]
			for len(words) > 0 && (strings.HasPrefix(words[0], "-") || envAssign.MatchString(words[0])) {
				words = words[1:]
			}
		case w == "nice":
			words = words[1:]
			if len(words) > 1 && words[0] == "-n" {
				words = words[2:]
			}
		case w == "timeout":
			words = words[1:]
			for len(words) > 0 && strings.HasPrefix(words[0], "-") {
				words = words[1:]
			}
			if len(words) > 0 {
				words = words[1:] // the duration
			}
		default:
			return words
		}
	}
	return nil
}

// subcommand returns the first argument that is not an option, skipping the
// value of each option listed in valued.
func subcommand(args []string, valued ...string) (string, []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if slices.Contains(valued, a) {
			i++
			continue
		}
		if strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+") {
			continue
		}
		return a, args[i+1:]
	}
	return "", nil
}

// heavyStep names the heavy step name and args run, or returns "".
func heavyStep(name string, args []string) string {
	switch name {
	case "go":
		sub, _ := subcommand(args, "-C")
		if slices.Contains([]string{"build", "test", "install", "run"}, sub) {
			return "go " + sub
		}
	case "cargo":
		sub, _ := subcommand(args, "-C", "--manifest-path", "--config", "-Z")
		if slices.Contains([]string{"build", "b", "test", "t", "run", "r", "bench", "install"}, sub) {
			return "cargo " + sub
		}
	case "npm", "pnpm", "yarn", "bun":
		return nodeStep(name, args)
	case "pip", "pip3", "uv", "poetry", "python", "python3":
		return pythonStep(name, args)
	case "gradle", "gradlew", "mvn", "mvnw":
		return jvmStep(name, args)
	case "podman":
		sub, rest := subcommand(args, "--connection", "-c", "--url", "--log-level")
		next, _ := subcommand(rest, "-f", "--file", "-p", "--project-name")
		switch {
		case sub == "build":
			return "podman build"
		case sub == "compose" && (next == "up" || next == "build"):
			return "podman compose " + next
		}
	case "podman-compose":
		if sub, _ := subcommand(args, "-f", "--file", "-p", "--project-name"); sub == "up" || sub == "build" {
			return "podman-compose " + sub
		}
	case "docker":
		sub, rest := subcommand(args, "--context", "-c", "-H", "--host", "--config", "-l", "--log-level")
		switch sub {
		case "build":
			return "docker build"
		case "buildx", "compose":
			if next, _ := subcommand(rest, "-f", "--file", "-p", "--project-name", "--builder"); next == "build" || (sub == "compose" && next == "up") {
				return "docker " + sub + " " + next
			}
		}
	case "docker-compose":
		if sub, _ := subcommand(args, "-f", "--file", "-p", "--project-name"); sub == "up" || sub == "build" {
			return "docker-compose " + sub
		}
	case "xcodebuild":
		for _, a := range args {
			if slices.Contains([]string{"-version", "-list", "-showsdks", "-showBuildSettings", "-help", "-usage"}, a) {
				return ""
			}
		}
		return "xcodebuild"
	case "git":
		sub, rest := subcommand(args, "-C", "-c", "--git-dir", "--work-tree", "--namespace")
		if next, _ := subcommand(rest); sub == "worktree" && next == "add" {
			return "git worktree add"
		}
	}
	return ""
}
