//go:build unix

package procgroup

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// psTimeout bounds one read of the process table.
const psTimeout = time.Second

// row is one live process in a process-table snapshot.
type row struct{ pid, ppid, pgid int }

// tree is what Kill has reached of a command's descendants: the processes
// it tracks by PID and the process groups they lead. A tracked process stays
// tracked while it lives, so a later setsid or reparenting does not lose it.
type tree struct {
	root    int
	self    int
	tracked map[int]bool
	groups  map[int]bool
	// live is the set of tracked groups that had a member in the latest
	// snapshot, the only ones signaled; nil when there is no snapshot, and
	// then only the root group is.
	live map[int]bool
}

func newTree(root int) *tree {
	return &tree{root: root, self: os.Getpid(),
		tracked: map[int]bool{root: true}, groups: map[int]bool{root: true}}
}

// refresh reads the process table, drops tracked processes that are gone,
// adds every child of a tracked process and every member of a tracked group
// (to a fixed point), and reports whether anything tracked is still alive.
// Without a process table (no ps) it falls back to the root group alone.
func (t *tree) refresh() bool {
	rows, err := snapshot()
	if err != nil {
		t.live = nil
		return syscall.Kill(-t.root, 0) == nil
	}
	t.extend(rows)
	return len(t.tracked) > 0
}

// extend is refresh over a given snapshot. Pruning comes first, so a PID
// that is gone, or was reused by an unrelated process, links nothing.
func (t *tree) extend(rows []row) {
	live := make(map[int]bool, len(rows))
	for _, r := range rows {
		live[r.pid] = true
	}
	for pid := range t.tracked {
		if !live[pid] {
			delete(t.tracked, pid)
		}
	}
	for grew := true; grew; {
		grew = false
		for _, r := range rows {
			if t.tracked[r.pid] || r.pid == t.self || !(t.tracked[r.ppid] || t.groups[r.pgid]) {
				continue
			}
			t.tracked[r.pid], grew = true, true
			if r.pid == r.pgid {
				// It left the group to lead its own (setsid, Setpgid):
				// whatever it starts there belongs to the tree too.
				t.groups[r.pid] = true
			}
		}
	}
	t.live = map[int]bool{}
	for _, r := range rows {
		if t.groups[r.pgid] {
			t.live[r.pgid] = true
		}
	}
}

// signal sends sig to every tracked process and every tracked group that
// had a member in the latest snapshot. It returns the result for the root
// group, the one exec.Cmd.Cancel reports on: ESRCH when it had no member.
func (t *tree) signal(sig syscall.Signal) error {
	err := error(syscall.ESRCH)
	if t.live == nil || t.live[t.root] {
		err = syscall.Kill(-t.root, sig)
	}
	for pgid := range t.live {
		if pgid != t.root {
			_ = syscall.Kill(-pgid, sig)
		}
	}
	for pid := range t.tracked {
		_ = syscall.Kill(pid, sig)
	}
	return err
}

// snapshot lists the live, non-zombie processes. The same flags work with
// BSD ps (macOS) and procps (Linux).
func snapshot() ([]row, error) {
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,pgid=,stat=").Output()
	if err != nil {
		return nil, err
	}
	return parsePS(string(out)), nil
}

// parsePS parses `ps -o pid=,ppid=,pgid=,stat=` output, skipping zombies and
// rows that do not parse (a process that exits mid-listing).
func parsePS(out string) []row {
	var rows []row
	for line := range strings.Lines(out) {
		f := strings.Fields(line)
		if len(f) < 4 || strings.HasPrefix(f[3], "Z") {
			continue
		}
		var n [3]int
		ok := true
		for i := range n {
			v, err := strconv.Atoi(f[i])
			n[i], ok = v, ok && err == nil
		}
		if ok {
			rows = append(rows, row{pid: n[0], ppid: n[1], pgid: n[2]})
		}
	}
	return rows
}
