#!/bin/sh
# hook-codex-stop.sh — Codex CLI Stop hook for autopus result collection.
# Reads hook JSON from stdin, extracts last_assistant_message,
# writes result.json and done signal to the session directory.
# POSIX shell compatible. No jq dependency — uses python3 for JSON.
set -e

SESSION_ID="${AUTOPUS_SESSION_ID:-}"
if [ -z "$SESSION_ID" ]; then
  exit 0
fi

# Validate session ID to prevent path traversal (alphanumeric, hyphen, underscore only)
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

# Atomically replace an entry relative to an already-open, non-symlink session
# directory. The temporary file is mode 0600 and target symlinks are replaced.
atomic_write() {
  python3 -c "
import os, secrets, sys
directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
directory_fd = os.open(sys.argv[1], directory_flags)
temporary = '.autopus-hook-' + secrets.token_hex(12)
body = sys.argv[3].encode('ascii')
file_fd = -1
try:
    file_flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, 'O_NOFOLLOW', 0)
    file_fd = os.open(temporary, file_flags, 0o600, dir_fd=directory_fd)
    os.fchmod(file_fd, 0o600)
    remaining = memoryview(body)
    while remaining:
        written = os.write(file_fd, remaining)
        if written <= 0:
            raise OSError('short write')
        remaining = remaining[written:]
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

# Resolve the effective round from the environment and confined cursor file.
CURSOR_NAME="codex-round-cursor"
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
    file_flags = os.O_RDONLY | getattr(os, 'O_NONBLOCK', 0) | getattr(os, 'O_NOFOLLOW', 0)
    file_fd = os.open(sys.argv[2], file_flags, dir_fd=directory_fd)
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
  RESULT_NAME="codex-round${EFFECTIVE_ROUND}-result.json"
  DONE_NAME="codex-round${EFFECTIVE_ROUND}-done"
else
  RESULT_NAME="codex-result.json"
  DONE_NAME="codex-done"
fi

# Read the documented Codex hook payload from stdin. Some runtimes expose a
# last_assistant_message compatibility field; otherwise use transcript_path as
# a best-effort fallback and select the latest assistant output_text message.
# Result capture must never suppress the done signal below. Always materialize a
# result file (empty output on failure), then write done after it.
python3 -c "
import json, os, secrets, stat, sys

max_transcript_bytes = 8 * 1024 * 1024

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
msg = data.get('last_assistant_message', '')
if not isinstance(msg, str):
    msg = ''
transcript = data.get('transcript_path')
if not msg and isinstance(transcript, str) and transcript:
    transcript_fd = -1
    try:
        flags = os.O_RDONLY | getattr(os, 'O_NONBLOCK', 0) | getattr(os, 'O_NOFOLLOW', 0)
        transcript_fd = os.open(transcript, flags)
        info = os.fstat(transcript_fd)
        if stat.S_ISREG(info.st_mode) and info.st_size <= max_transcript_bytes:
            with os.fdopen(transcript_fd, 'rb') as source:
                transcript_fd = -1
                transcript_body = source.read(max_transcript_bytes + 1)
            if len(transcript_body) <= max_transcript_bytes:
                lines = transcript_body.splitlines()
            else:
                lines = []
            for line in lines:
                try:
                    item = json.loads(line)
                    payload = item.get('payload', {}) if isinstance(item, dict) else {}
                    if not isinstance(payload, dict):
                        continue
                    if item.get('type') != 'response_item' or payload.get('type') != 'message' or payload.get('role') != 'assistant':
                        continue
                    parts = [part.get('text', '') for part in payload.get('content', []) if isinstance(part, dict) and part.get('type') == 'output_text']
                    candidate = ''.join(part for part in parts if isinstance(part, str)).strip()
                    if candidate:
                        msg = candidate
                except Exception:
                    continue
    except Exception:
        pass
    finally:
        if transcript_fd >= 0:
            os.close(transcript_fd)

directory_flags = os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0)
directory_fd = os.open(sys.argv[1], directory_flags)
try:
    result = json.dumps({'output': msg, 'exit_code': 0}).encode('utf-8')
    try:
        atomic_write(directory_fd, sys.argv[2], result)
    except Exception:
        pass
    try:
        atomic_write(directory_fd, sys.argv[3], b'')
    except Exception:
        pass
finally:
    os.close(directory_fd)
" "$SESSION_DIR" "$RESULT_NAME" "$DONE_NAME" 2>/dev/null || true

# Send cmux completion signal for SignalDetector (SPEC-SURFCOMP-001 R8).
if command -v cmux >/dev/null 2>&1; then
  if [ -n "$EFFECTIVE_ROUND" ] && [ "$EFFECTIVE_ROUND" -gt 1 ] 2>/dev/null; then
    cmux wait-for -S "done-codex-round${EFFECTIVE_ROUND}" >/dev/null 2>&1 || true
  else
    cmux wait-for -S "done-codex" >/dev/null 2>&1 || true
  fi
fi

# --- Bidirectional IPC: Ready signal + Input watch loop (SPEC-ORCH-017) ---
# Only activate for round-scoped sessions.
if [ -n "$EFFECTIVE_ROUND" ]; then
  NEXT_ROUND=$((EFFECTIVE_ROUND + 1))
  READY_NAME="codex-round${NEXT_ROUND}-ready"
  INPUT_NAME="codex-round${NEXT_ROUND}-input.json"
  ABORT_NAME="codex-round${NEXT_ROUND}-abort"
  READY_FILE="${SESSION_DIR}/${READY_NAME}"
  INPUT_FILE="${SESSION_DIR}/${INPUT_NAME}"
  ABORT_FILE="${SESSION_DIR}/${ABORT_NAME}"

  # Signal ready for next round input.
  atomic_write "$READY_NAME" ""

  # Poll for input file (200ms intervals, 120s timeout = 600 iterations).
  # @AX:NOTE [AUTO] magic constants 200ms/600 iterations — must match Go-side fileIPCReadyTimeout budget
  WAIT_COUNT=0
  MAX_WAIT=600
  while [ "$WAIT_COUNT" -lt "$MAX_WAIT" ]; do
    if [ -f "$ABORT_FILE" ]; then
      rm -f "${READY_FILE}" "${ABORT_FILE}"
      exit 0
    fi
    if [ -f "$INPUT_FILE" ]; then
      STOP_OUTPUT=$(python3 -c "
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
    with os.fdopen(file_fd) as source:
        file_fd = -1
        data = json.load(source)
    prompt = data.get('prompt', '') if isinstance(data, dict) else ''
    provider = data.get('provider') if isinstance(data, dict) else None
    round_number = data.get('round') if isinstance(data, dict) else None
    valid_round = isinstance(round_number, int) and not isinstance(round_number, bool)
    if provider == sys.argv[3] and valid_round and round_number == int(sys.argv[4]) and isinstance(prompt, str) and prompt:
        sys.stdout.write(json.dumps(
            {'decision': 'block', 'reason': prompt},
            ensure_ascii=True,
            separators=(',', ':'),
        ))
finally:
    if file_fd >= 0:
        os.close(file_fd)
    os.close(directory_fd)
" "$SESSION_DIR" "$INPUT_NAME" "codex" "$NEXT_ROUND") || STOP_OUTPUT=""
      if [ -n "$STOP_OUTPUT" ]; then
        atomic_write "$CURSOR_NAME" "$NEXT_ROUND"
        rm -f "${INPUT_FILE}" "${READY_FILE}"
        printf '%s' "$STOP_OUTPUT"
      else
        rm -f "${INPUT_FILE}" "${READY_FILE}"
      fi
      exit 0
    fi
    python3 -c "import time; time.sleep(0.2)" || sleep 1
    WAIT_COUNT=$((WAIT_COUNT + 1))
  done

  # Timeout — clean up ready signal and exit normally.
  rm -f "${READY_FILE}"
fi
