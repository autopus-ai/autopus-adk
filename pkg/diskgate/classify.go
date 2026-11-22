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

// step is one heavy step heavyStep recognised. io marks a step that mostly
// waits on the network and disk, an install or a checkout, which the CPU
// classification leaves out.
type step struct {
	match string
	io    bool
}

// envAssign is a NAME=value prefix word.
var envAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// Classify reports whether any simple command in line is heavy. Every
// `auto` command is allowed, `auto disk lease -- <cmd>` included: a lease is
// the sanctioned way to run heavy work while space is short. extra adds
// word prefixes such as "make release" to the built-in list; make itself is
// not heavy by default, because most of its targets are not.
func Classify(line string, extra ...string) Verdict {
	return classify(line, parsePrefixes(extra), 0)
}

// parsePrefixes splits each extra command into its words, dropping blanks.
func parsePrefixes(extra []string) [][]string {
	var prefixes [][]string
	for _, command := range extra {
		if words := strings.Fields(command); len(words) > 0 {
			prefixes = append(prefixes, words)
		}
	}
	return prefixes
}

func classify(line string, extra [][]string, depth int) Verdict {
	for _, words := range segments(lex(line)) {
		if s := classifyWords(words, extra, depth); s.match != "" {
			return Verdict{Heavy: true, Match: s.match}
		}
	}
	return Verdict{}
}

func classifyWords(words []string, extra [][]string, depth int) step {
	words = stripPrefixes(words)
	if len(words) == 0 {
		return step{}
	}
	name := filepath.Base(words[0])
	args := words[1:]
	switch name {
	case "auto":
		return step{}
	case "sh", "bash", "zsh", "dash":
		if script, ok := shellScript(args); ok && depth < maxShellDepth {
			if v := classify(script, extra, depth+1); v.Heavy {
				return step{match: v.Match}
			}
		}
		return step{}
	}
	if s := heavyStep(name, args); s.match != "" {
		return s
	}
	for _, prefix := range extra {
		if matchesPrefix(name, args, prefix) {
			return step{match: strings.Join(prefix, " ")}
		}
	}
	return step{}
}

// shellScript returns the script of `sh -c script`, the -c possibly in a
// cluster of short options such as -lc or -ec.
func shellScript(args []string) (string, bool) {
	for i, a := range args {
		if !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
			continue
		}
		if strings.ContainsRune(a[1:], 'c') && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
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

// xargsValued are the xargs options followed by a value.
var xargsValued = []string{"-n", "-P", "-I", "-L", "-s", "-E", "-d", "-a", "-J", "-R", "-S"}

// stripPrefixes drops what runs before the command proper: `!`, NAME=value
// assignments, and wrappers such as env, time, nice, nohup, command, exec,
// sudo, timeout, and xargs, with their options. Wrappers are compared by
// base name, so /usr/bin/time counts.
func stripPrefixes(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		switch base := filepath.Base(w); {
		case w == "!" || w == "{" || w == "}" || envAssign.MatchString(w):
			words = words[1:]
		case base == "command" && lookupOnly(words[1:]):
			// `command -v` and `-V` look a name up; nothing runs.
			return nil
		case slices.Contains([]string{"env", "command", "exec", "nohup", "time", "sudo", "caffeinate"}, base):
			words = words[1:]
			for len(words) > 0 && (strings.HasPrefix(words[0], "-") || envAssign.MatchString(words[0])) {
				words = words[1:]
			}
		case base == "nice":
			words = skipOptions(words[1:], "-n", "--adjustment")
		case base == "xargs":
			words = skipOptions(words[1:], xargsValued...)
		case base == "timeout":
			words = skipOptions(words[1:], "-s", "--signal", "-k", "--kill-after")
			if len(words) > 0 {
				words = words[1:] // the duration
			}
		default:
			return words
		}
	}
	return nil
}

// lookupOnly reports whether the options of `command` ask only to describe
// the name (-v or -V, possibly clustered as in -pv), not to run it.
func lookupOnly(args []string) bool {
	for _, a := range args {
		if a == "--" || !strings.HasPrefix(a, "-") {
			return false
		}
		if strings.ContainsAny(a[1:], "vV") {
			return true
		}
	}
	return false
}

// skipOptions drops the leading options of words, and the value of each
// option listed in valued.
func skipOptions(words []string, valued ...string) []string {
	for len(words) > 0 && strings.HasPrefix(words[0], "-") {
		if slices.Contains(valued, words[0]) && len(words) > 1 {
			words = words[1:]
		}
		words = words[1:]
	}
	return words
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
