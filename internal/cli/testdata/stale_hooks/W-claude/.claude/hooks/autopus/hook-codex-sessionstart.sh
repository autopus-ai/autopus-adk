#!/bin/sh
# hook-codex-sessionstart.sh — codex session-start hook for autopus.
# Writes a ready-signal file so the orchestrator detects session readiness via
# file IPC instead of screen scraping (SPEC-ORCH-022). No-op unless
# AUTOPUS_SESSION_ID is set. POSIX shell; does not require stdin.
set -e

SESSION_ID="${AUTOPUS_SESSION_ID:-}"
if [ -z "$SESSION_ID" ]; then
  exit 0
fi
case "$SESSION_ID" in
  *[!a-zA-Z0-9_-]*) exit 0 ;;
esac
SESSION_DIR="${AUTOPUS_SESSION_DIR:-/tmp/autopus/${SESSION_ID}}"
case "$SESSION_DIR" in
  /*) ;;
  *) exit 0 ;;
esac
if [ ! -d "$SESSION_DIR" ] || [ -L "$SESSION_DIR" ]; then
  exit 0
fi
case "${AUTOPUS_ROUND:-}" in *[!0-9]*) AUTOPUS_ROUND="" ;; esac
ROUND="${AUTOPUS_ROUND:-0}"
READY_NAME="codex-round${ROUND}-ready"
python3 -c "
import os, secrets, sys
directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
directory_fd = os.open(sys.argv[1], directory_flags)
temporary = '.autopus-hook-' + secrets.token_hex(12)
file_fd = -1
try:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0)
    file_fd = os.open(temporary, flags, 0o600, dir_fd=directory_fd)
    os.fsync(file_fd)
    os.close(file_fd)
    file_fd = -1
    os.replace(temporary, sys.argv[2], src_dir_fd=directory_fd, dst_dir_fd=directory_fd)
except Exception:
    if file_fd >= 0:
        os.close(file_fd)
    try:
        os.unlink(temporary, dir_fd=directory_fd)
    except Exception:
        pass
    raise
finally:
    os.close(directory_fd)
" "$SESSION_DIR" "$READY_NAME" 2>/dev/null || true
exit 0
