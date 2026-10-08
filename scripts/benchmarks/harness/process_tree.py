"""Every process a trusted-runner stage started, followed past setsid and orphaning (SPEC-HARNEVAL-003 S2).

A stage is the process the runner started (its root) and everything descended from it. Its session or
process group alone loses a descendant that calls setsid, and on macOS an orphan is reparented to
launchd, so a walk from the root alone loses it once its parent has exited. A Tree therefore

  * walks the whole process table while the stage runs (every POLL seconds) and reaches, to a fixed
    point, every child of a reached process and every member of a group or session a reached process
    leads; a reached process stays tracked by pid while it lives, so a later setsid or reparenting does
    not lose it, and a group or session it led stays reachable after it exits;
  * for a stage confined by a Seatbelt profile instance, also counts every live process that instance
    confines (it may write the stage's scratch root but not that root's parent): no setsid or double
    fork sheds a sandbox, so a daemon the walks never saw is still found;
  * sweeps when the stage ends: SIGKILL every reached and confined process and its group, walk again,
    and repeat until a walk finds none. stragglers says any was found, leftover that one survived.
"""
import ctypes
import os
from pathlib import Path
import signal
import subprocess
import sys
import threading
import time

POLL = 0.05
_ZOMBIE, _BSD_INFO, _FILTER_PATH, _NO_REPORT = 5, 3, 1, 0x40000000


class _BSDInfo(ctypes.Structure):
    """libproc's struct proc_bsdinfo (PROC_PIDTBSDINFO)."""
    _fields_ = [(name, ctypes.c_uint32) for name in ('flags', 'status', 'xstatus', 'pid', 'ppid', 'uid', 'gid', 'ruid',
                                                     'rgid', 'svuid', 'svgid', 'rfu')] + \
               [('comm', ctypes.c_char * 16), ('name', ctypes.c_char * 32)] + \
               [(name, ctypes.c_uint32) for name in ('nfiles', 'pgid', 'pjobc', 'tdev', 'tpgid')] + \
               [('nice', ctypes.c_int32), ('start_sec', ctypes.c_uint64), ('start_usec', ctypes.c_uint64)]


def _libsystem():
    if sys.platform != 'darwin':
        return None
    lib = ctypes.CDLL('/usr/lib/libSystem.B.dylib', use_errno=True)
    lib.proc_listallpids.argtypes = [ctypes.c_void_p, ctypes.c_int]
    lib.proc_pidinfo.argtypes = [ctypes.c_int, ctypes.c_int, ctypes.c_uint64, ctypes.c_void_p, ctypes.c_int]
    # sandbox_check(pid, operation, filter, ...): the path is the one variadic argument.
    lib.sandbox_check.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int]
    lib.sandbox_check.restype = ctypes.c_int
    return lib


_LIB = _libsystem()


def table() -> dict:
    """{pid: (ppid, pgid, sid, uid)} of every live, non-zombie process the runner may inspect."""
    rows = {}
    if _LIB is not None:
        count = _LIB.proc_listallpids(None, 0)
        pids = (ctypes.c_int * (count + 512))()
        info = _BSDInfo()
        for pid in pids[:_LIB.proc_listallpids(pids, ctypes.sizeof(pids))]:
            if pid > 0 and _LIB.proc_pidinfo(pid, _BSD_INFO, 0, ctypes.byref(info), ctypes.sizeof(info)) == \
                    ctypes.sizeof(info) and info.status != _ZOMBIE:
                rows[pid] = (info.ppid, info.pgid, info.uid)
    else:
        listing = subprocess.run(['/bin/ps', '-A', '-o', 'pid=,ppid=,pgid=,uid=,stat='], capture_output=True,
                                 text=True, check=True)
        for fields in (line.split() for line in listing.stdout.splitlines()):
            if len(fields) == 5 and not fields[4].startswith('Z'):
                rows[int(fields[0])] = tuple(int(value) for value in fields[1:4])
    table = {}
    for pid, (ppid, pgid, uid) in rows.items():
        try:
            table[pid] = (ppid, pgid, os.getsid(pid), uid)
        except OSError:
            continue
    return table


def confined(rows: dict, scratch: Path | None) -> set:
    """The live processes of this uid that the stage's profile instance confines: sandboxed, allowed to
    write the existing directory `scratch` and denied a write to its parent (sandbox_check judges only
    existing paths). Empty without a scratch root or off macOS."""
    if scratch is None or _LIB is None:
        return set()
    allowed, denied = str(scratch).encode(), str(Path(scratch).parent).encode()
    check, uid = _LIB.sandbox_check, os.getuid()
    return {pid for pid, row in rows.items() if row[3] == uid and pid != os.getpid() and check(pid, None, 0) == 1
            and check(pid, b'file-write-data', _FILTER_PATH | _NO_REPORT, allowed) == 0
            and check(pid, b'file-write-data', _FILTER_PATH | _NO_REPORT, denied) == 1}


class Tree:
    """The descendants of one stage root; see the module docstring."""

    def __init__(self, root: int, scratch: Path | None = None):
        self.root, self.scratch = root, scratch
        self.known, self.leaders = set(), {root}
        self._lock, self._stop, self._thread = threading.Lock(), threading.Event(), None

    def reach(self, rows: dict) -> set:
        """The fixed point over one table: tracked pids still alive, children of reached processes, and
        members of a group or session a reached process (alive or not) leads. The root and the runner
        never count; `known` becomes the reached set and `leaders` keeps every pid ever reached."""
        with self._lock:
            reached = {self.root} | (self.known & rows.keys())
            grown = True
            while grown:
                grown = False
                for pid, (ppid, pgid, sid, _uid) in rows.items():
                    if pid not in reached and (ppid in reached or pgid in self.leaders or sid in self.leaders):
                        reached.add(pid)
                        self.leaders.add(pid)
                        grown = True
            reached -= {self.root, os.getpid()}
            self.known = reached
            self.leaders |= reached
            return set(reached)

    def watch(self) -> 'Tree':
        """Walk every POLL seconds until sweep(); a failed walk is retried on the next tick."""
        def loop():
            while not self._stop.wait(POLL):
                try:
                    self.reach(table())
                except (OSError, ValueError, subprocess.SubprocessError):
                    continue
        self._thread = threading.Thread(target=loop, daemon=True)
        self._thread.start()
        return self

    def members(self) -> tuple:
        rows = table()
        return rows, self.reach(rows) | confined(rows, self.scratch)

    def sweep(self, rounds: int = 40) -> tuple:
        """(stragglers, leftover) after killing every member and its group until a walk finds none."""
        self._stop.set()
        if self._thread is not None:
            self._thread.join()
        found, own_group = False, os.getpgrp()
        for _ in range(rounds):
            rows, members = self.members()
            if self.root in rows:
                _kill(os.kill, self.root)
            if not members:
                return found, False
            found = True
            for pid in members:
                _kill(os.kill, pid)
            for group in {rows[pid][1] for pid in members if pid in rows} - {own_group}:
                _kill(os.killpg, group)
            time.sleep(0.05)
        return found, bool(self.members()[1])


def _kill(send, target: int) -> None:
    try:
        send(target, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass
