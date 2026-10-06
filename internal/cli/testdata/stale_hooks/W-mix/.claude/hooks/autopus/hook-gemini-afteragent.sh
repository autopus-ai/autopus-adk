#!/bin/sh
# hook-gemini-afteragent.sh — Gemini CLI AfterAgent result collector.
# POSIX shell compatible. Python provides confined JSON and file operations.
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

atomic_write() {
  python3 -c "
import os, secrets, sys
directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
directory_fd = os.open(sys.argv[1], directory_flags)
temporary = '.autopus-hook-' + secrets.token_hex(12)
file_fd = -1
try:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0)
    file_fd = os.open(temporary, flags, 0o600, dir_fd=directory_fd)
    remaining = memoryview(sys.argv[3].encode('utf-8'))
    while remaining:
        written = os.write(file_fd, remaining)
        if written <= 0:
            raise OSError('short write')
        remaining = remaining[written:]
    os.fchmod(file_fd, 0o600)
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
" "$SESSION_DIR" "$1" "$2" 2>/dev/null
}

CURSOR_NAME="gemini-round-cursor"
EFFECTIVE_ROUND=$(python3 -c "
import os, stat, sys
def parse(value, allow_newline=False):
    if allow_newline:
        value = value.rstrip('\\r\\n')
    if not value or any(ch < '0' or ch > '9' for ch in value):
        return None
    number = int(value)
    return number if number <= 2147483646 else None
env_round = parse(sys.argv[3])
cursor_round = None
directory_fd = file_fd = -1
try:
    directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
    directory_fd = os.open(sys.argv[1], directory_flags)
    flags = os.O_RDONLY | getattr(os, 'O_NONBLOCK', 0) | getattr(os, 'O_NOFOLLOW', 0)
    file_fd = os.open(sys.argv[2], flags, dir_fd=directory_fd)
    info = os.fstat(file_fd)
    if stat.S_ISREG(info.st_mode) and info.st_size <= 64:
        with os.fdopen(file_fd, 'rb') as source:
            file_fd = -1
            raw = source.read(65)
        if len(raw) <= 64:
            cursor_round = parse(raw.decode('ascii'), True)
except Exception:
    pass
finally:
    if file_fd >= 0:
        os.close(file_fd)
    if directory_fd >= 0:
        os.close(directory_fd)
rounds = [value for value in (env_round, cursor_round) if value is not None]
if rounds:
    print(max(rounds))
" "$SESSION_DIR" "$CURSOR_NAME" "${AUTOPUS_ROUND:-}" 2>/dev/null) || EFFECTIVE_ROUND=""

if [ -n "$EFFECTIVE_ROUND" ]; then
  RESULT_NAME="gemini-round${EFFECTIVE_ROUND}-result.json"
  DONE_NAME="gemini-round${EFFECTIVE_ROUND}-done"
else
  RESULT_NAME="gemini-result.json"
  DONE_NAME="gemini-done"
fi

# Malformed or empty payloads still publish an empty result followed by done.
if ! python3 -c "
import json, os, secrets, sys
def atomic_write(directory_fd, name, body):
    temporary = '.autopus-hook-' + secrets.token_hex(12)
    file_fd = -1
    try:
        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0)
        file_fd = os.open(temporary, flags, 0o600, dir_fd=directory_fd)
        remaining = memoryview(body)
        while remaining:
            written = os.write(file_fd, remaining)
            if written <= 0:
                raise OSError('short write')
            remaining = remaining[written:]
        os.fchmod(file_fd, 0o600)
        os.fsync(file_fd)
        os.close(file_fd)
        file_fd = -1
        os.replace(temporary, name, src_dir_fd=directory_fd, dst_dir_fd=directory_fd)
    except Exception:
        if file_fd >= 0:
            os.close(file_fd)
        try:
            os.unlink(temporary, dir_fd=directory_fd)
        except Exception:
            pass
        raise
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
if not isinstance(data, dict):
    data = {}
msg = data.get('prompt_response', '')
if not isinstance(msg, str):
    msg = ''
directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
directory_fd = os.open(sys.argv[1], directory_flags)
errors = []
try:
    for name, body in (
        (sys.argv[2], json.dumps({'output': msg, 'exit_code': 0}).encode('utf-8')),
        (sys.argv[3], b''),
    ):
        try:
            atomic_write(directory_fd, name, body)
        except Exception as error:
            errors.append(error)
finally:
    os.close(directory_fd)
if errors:
    raise errors[0]
" "$SESSION_DIR" "$RESULT_NAME" "$DONE_NAME" 2>/dev/null; then
  exit 0
fi

if command -v cmux >/dev/null 2>&1; then
  if [ -n "$EFFECTIVE_ROUND" ] && [ "$EFFECTIVE_ROUND" -gt 1 ] 2>/dev/null; then
    cmux wait-for -S "done-gemini-round${EFFECTIVE_ROUND}" >/dev/null 2>&1 || true
  else
    cmux wait-for -S "done-gemini" >/dev/null 2>&1 || true
  fi
fi

if [ -n "$EFFECTIVE_ROUND" ]; then
  NEXT_ROUND=$((EFFECTIVE_ROUND + 1))
  READY_NAME="gemini-round${NEXT_ROUND}-ready"
  INPUT_NAME="gemini-round${NEXT_ROUND}-input.json"
  ABORT_NAME="gemini-round${NEXT_ROUND}-abort"
  READY_FILE="${SESSION_DIR}/${READY_NAME}"
  INPUT_FILE="${SESSION_DIR}/${INPUT_NAME}"
  ABORT_FILE="${SESSION_DIR}/${ABORT_NAME}"
  if ! atomic_write "$READY_NAME" ""; then
    exit 0
  fi

  WAIT_COUNT=0
  MAX_WAIT=600
  while [ "$WAIT_COUNT" -lt "$MAX_WAIT" ]; do
    if [ -f "$ABORT_FILE" ]; then
      rm -f "$READY_FILE" "$ABORT_FILE" 2>/dev/null || true
      exit 0
    fi
    if [ -e "$INPUT_FILE" ] || [ -L "$INPUT_FILE" ]; then
      PROMPT=$(python3 -c "
import json, os, stat, sys
directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
directory_fd = os.open(sys.argv[1], directory_flags)
file_fd = -1
try:
    flags = os.O_RDONLY | getattr(os, 'O_NONBLOCK', 0) | getattr(os, 'O_NOFOLLOW', 0)
    file_fd = os.open(sys.argv[2], flags, dir_fd=directory_fd)
    info = os.fstat(file_fd)
    if not stat.S_ISREG(info.st_mode) or info.st_size > 1024 * 1024:
        raise ValueError('unsafe input file')
    with os.fdopen(file_fd, 'rb') as source:
        file_fd = -1
        raw = source.read(1024 * 1024 + 1)
    if len(raw) > 1024 * 1024:
        raise ValueError('oversized input file')
    data = json.loads(raw)
    prompt = data.get('prompt', '') if isinstance(data, dict) else ''
    provider = data.get('provider') if isinstance(data, dict) else None
    round_number = data.get('round') if isinstance(data, dict) else None
    valid_round = isinstance(round_number, int) and not isinstance(round_number, bool)
    if provider == sys.argv[3] and valid_round and round_number == int(sys.argv[4]) and isinstance(prompt, str) and prompt:
        sys.stdout.write(prompt)
finally:
    if file_fd >= 0:
        os.close(file_fd)
    os.close(directory_fd)
" "$SESSION_DIR" "$INPUT_NAME" "gemini" "$NEXT_ROUND" 2>/dev/null) || PROMPT=""
      if [ -n "$PROMPT" ]; then
        if ! atomic_write "$CURSOR_NAME" "$NEXT_ROUND"; then
          rm -f "$INPUT_FILE" "$READY_FILE" 2>/dev/null || true
          exit 0
        fi
        rm -f "$INPUT_FILE" "$READY_FILE" 2>/dev/null || true
        printf '%s' "$PROMPT" | python3 -c "
import json, sys
json.dump({'decision': 'block', 'reason': sys.stdin.read()}, sys.stdout)
" 2>/dev/null || true
      else
        rm -f "$INPUT_FILE" "$READY_FILE" 2>/dev/null || true
      fi
      exit 0
    fi
    python3 -c "import time; time.sleep(0.2)" || sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
  done
  rm -f "$READY_FILE" 2>/dev/null || true
fi
