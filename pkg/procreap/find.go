package procreap

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Reasons a candidate is reported with.
const (
	ReasonSelfMatchingPgrep = "self-matching pgrep"
	reasonNoDeadlinePrefix  = "no deadline, waiting "
)

// Options select which processes Find may report.
type Options struct {
	// UID is the only owner whose processes are considered.
	UID int
	// SelfPID is the caller; it and every ancestor are never candidates.
	SelfPID int
	// OlderThan is the minimum age of a candidate shell.
	OlderThan time.Duration
	// SessionPID, when non-zero, restricts candidates to its descendants.
	SessionPID int
}

// Candidate is an idle polling shell and the descendants it would take along.
type Candidate struct {
	Process
	Reason      string
	Descendants []Process
}

var (
	shells      = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true}
	loopPattern = regexp.MustCompile(`\b(until|while)\b[\s\S]*\bdo\b[\s\S]*\bsleep\b`)
	pgrepFull   = regexp.MustCompile(`\bpgrep\s+((?:-[A-Za-z]+\s+)*)(?:"([^"]*)"|'([^']*)'|([^\s;|&)]+))`)
)

// Find returns the idle polling shells in procs, oldest first.
func Find(procs []Process, opts Options) []Candidate {
	byPID := make(map[int]Process, len(procs))
	children := make(map[int][]Process)
	for _, p := range procs {
		byPID[p.PID] = p
		children[p.PPID] = append(children[p.PPID], p)
	}
	protected := ancestors(byPID, opts.SelfPID)

	var out []Candidate
	for _, p := range procs {
		if p.UID != opts.UID || protected[p.PID] || p.Age < opts.OlderThan {
			continue
		}
		if !isShell(p.Command) || !loopPattern.MatchString(p.Command) {
			continue
		}
		if opts.SessionPID != 0 && !descendsFrom(byPID, p.PID, opts.SessionPID) {
			continue
		}
		desc := descendants(children, p.PID)
		if !allSleep(desc) {
			continue
		}
		out = append(out, Candidate{Process: p, Reason: reason(p), Descendants: desc})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Age > out[j].Age })
	return out
}

func reason(p Process) string {
	if SelfMatchingPgrep(p.Command) {
		return ReasonSelfMatchingPgrep
	}
	return reasonNoDeadlinePrefix + FormatAge(p.Age)
}

// SelfMatchingPgrep reports whether a `pgrep -f <pattern>` inside command
// matches command itself, so a loop waiting on it can never see it vanish.
func SelfMatchingPgrep(command string) bool {
	for _, m := range pgrepFull.FindAllStringSubmatch(command, -1) {
		if !strings.Contains(m[1], "f") {
			continue
		}
		pattern := m[2] + m[3] + m[4]
		if pattern == "" {
			continue
		}
		if re, err := regexp.Compile(pattern); err == nil {
			if re.MatchString(command) {
				return true
			}
		} else if strings.Contains(command, pattern) {
			return true
		}
	}
	return false
}

// isShell reports whether the executable of command is a POSIX-family shell.
func isShell(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	return shells[strings.TrimPrefix(path.Base(fields[0]), "-")]
}

func allSleep(procs []Process) bool {
	for _, p := range procs {
		fields := strings.Fields(p.Command)
		if len(fields) == 0 || path.Base(fields[0]) != "sleep" {
			return false
		}
	}
	return true
}

func ancestors(byPID map[int]Process, pid int) map[int]bool {
	seen := map[int]bool{}
	for pid > 0 && !seen[pid] {
		seen[pid] = true
		p, ok := byPID[pid]
		if !ok {
			break
		}
		pid = p.PPID
	}
	return seen
}

func descendsFrom(byPID map[int]Process, pid, root int) bool {
	seen := map[int]bool{}
	for p, ok := byPID[pid]; ok && !seen[p.PID]; p, ok = byPID[p.PPID] {
		seen[p.PID] = true
		if p.PPID == root {
			return true
		}
	}
	return false
}

func descendants(children map[int][]Process, pid int) []Process {
	var out []Process
	queue := []int{pid}
	seen := map[int]bool{pid: true}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, c := range children[next] {
			if !seen[c.PID] {
				seen[c.PID] = true
				out = append(out, c)
				queue = append(queue, c.PID)
			}
		}
	}
	return out
}

// FormatAge renders an age as 5h12m, 42m or 30s.
func FormatAge(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// ShortCommand trims a command line to its loop and at most width runes.
func ShortCommand(command string, width int) string {
	if m := regexp.MustCompile(`\b(until|while)\b`).FindStringIndex(command); m != nil {
		command = command[m[0]:]
	}
	r := []rune(command)
	if width > 1 && len(r) > width {
		return string(r[:width-1]) + "…"
	}
	return command
}
