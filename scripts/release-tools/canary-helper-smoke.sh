#!/usr/bin/env bash
# Exercises the installed release canary helper end to end with a throwaway
# nonce, without a release: root, stage, install, one-shot account, run as
# the canary and as nobody, hand-over, unstage, cleanup. Run it after
# install-canary-helper.sh; it needs no password once the rule is in place.
set -euo pipefail
umask 077
helper='/Library/PrivilegedHelperTools/co.autopus.release-canary'
fail() { printf 'canary helper smoke: %s\n' "$1" >&2; exit 1; }
h() { /usr/bin/sudo -n "$helper" "$@"; }

nonce=$(head -c 16 /dev/urandom | xxd -p)
root="/private/tmp/autopus-adk-release-prep-${nonce}-final"
# Made the way prepare-release.sh makes its temp directory, so a path the
# release passes to the helper is a path this smoke passes too.
temp_base=${TMPDIR:-/tmp}
work=$(mktemp -d "${temp_base%/}/canary-helper-smoke.XXXXXX")
cleanup() {
  h account-kill "$nonce" 1 KILL >/dev/null 2>&1 || true
  h account-delete "$nonce" 1 >/dev/null 2>&1 || true
  h remove-root "$nonce" >/dev/null 2>&1 || true
  rm -rf -- "$work"
}
trap cleanup EXIT

mkdir -p "$work/project/sub" && printf 'hello\n' >"$work/project/sub/a.txt"
ln -s sub/a.txt "$work/project/link"
cp /usr/bin/true "$work/fake-auto"

h prepare-root "$nonce"
[[ "$(stat -f '%u:%Lp' "$root")" == '0:755' ]] || fail 'root is not root-owned 0755'
h stage "$nonce" "$work/project"
h install-exec "$nonce" auto "$work/fake-auto" >/dev/null
[[ "$(stat -f '%u:%Lp' "$root/auto")" == '0:555' ]] || fail 'installed candidate is not root-owned 0555'
h account-create "$nonce" 1
h chown "$nonce" canary 1
uid=$(h run-canary "$nonce" 1 -- /usr/bin/id -u)
[[ "$uid" -ge 50000 && "$uid" -le 59999 ]] || fail "run-canary ran as uid ${uid}"
h run-canary "$nonce" 1 -- /bin/sh -c "printf canary >$root/project/from-canary.txt"
h account-delete "$nonce" 1
h chown "$nonce" nobody 1
h run-nobody "$nonce" -- || fail 'run-nobody did not run the installed candidate'
h unstage "$nonce" "$work/out"
[[ "$(cat "$work/out/sub/a.txt")" == 'hello' && "$(cat "$work/out/from-canary.txt")" == 'canary' ]] ||
  fail 'unstage did not bring the project back'
[[ "$(readlink "$work/out/link")" == 'sub/a.txt' && "$(stat -f '%u' "$work/out/sub/a.txt")" == "$(id -u)" ]] ||
  fail 'unstaged files are not links and files owned by this account'
if h stage "$nonce" "$work/project" 2>/dev/null; then fail 'stage ran after the tree was handed over'; fi
h remove-root "$nonce"
[[ ! -e "$root" ]] || fail 'remove-root left the root'
printf 'canary helper smoke: PASS (canary uid %s)\n' "$uid"
