// Package procreap finds idle polling shells that an agent session left
// behind (a `until`/`while` + `sleep` loop that will never end) and,
// on request, terminates them together with their sleeping children.
//
// Everything except Snapshot and Reap is a pure function over a process
// snapshot, so the selection rules can be tested with fixtures.
package procreap

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Process is one row of a process snapshot.
type Process struct {
	PID     int
	PPID    int
	UID     int
	Age     time.Duration
	Command string
}

// psArgs lists every process with its full command line. The same flags work
// with BSD ps (macOS) and procps (Linux).
var psArgs = []string{"-A", "-ww", "-o", "pid=,ppid=,uid=,etime=,command="}

// Snapshot reads the current process table through ps.
func Snapshot(ctx context.Context) ([]Process, error) {
	out, err := exec.CommandContext(ctx, "ps", psArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return ParsePS(string(out))
}

// ParsePS parses `ps -o pid=,ppid=,uid=,etime=,command=` output. Rows that do
// not parse are skipped: a process that exits mid-listing is not an error.
func ParsePS(out string) ([]Process, error) {
	var procs []Process
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if p, ok := parseRow(sc.Text()); ok {
			procs = append(procs, p)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("parse ps output: %w", err)
	}
	return procs, nil
}

func parseRow(line string) (Process, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return Process{}, false
	}
	var nums [3]int
	for i := range nums {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			return Process{}, false
		}
		nums[i] = n
	}
	age, err := ParseEtime(fields[3])
	if err != nil {
		return Process{}, false
	}
	// The command is everything after the fourth field, spacing preserved.
	rest := line
	for i := 0; i < 4; i++ {
		rest = strings.TrimLeft(rest, " \t")
		rest = rest[strings.IndexAny(rest, " \t"):]
	}
	return Process{PID: nums[0], PPID: nums[1], UID: nums[2], Age: age,
		Command: strings.TrimSpace(rest)}, true
}

// ParseEtime parses the ps elapsed-time format [[dd-]hh:]mm:ss.
func ParseEtime(s string) (time.Duration, error) {
	days := 0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, fmt.Errorf("etime %q: %w", s, err)
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("etime %q: want [[dd-]hh:]mm:ss", s)
	}
	total := 0
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("etime %q: bad field %q", s, part)
		}
		total = total*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(total)*time.Second, nil
}
