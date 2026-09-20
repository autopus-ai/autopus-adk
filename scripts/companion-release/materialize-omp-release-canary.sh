#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'OMP release canary materialization: %s\n' "$1" >&2; exit 1; }

[[ "$(uname -s)" == 'Darwin' && "$(uname -m)" == 'arm64' ]] \
  || fail 'host is not Darwin arm64'
[[ "${GITHUB_RUN_ID:-}" =~ ^[0-9]+$ && "${GITHUB_RUN_ATTEMPT:-}" =~ ^[0-9]+$ ]] \
  || fail 'GitHub run identity is malformed'
[[ -n "${RUNNER_TEMP:-}" && -d "$RUNNER_TEMP" && ! -L "$RUNNER_TEMP" ]] \
  || fail 'runner temporary directory is unsafe'
readonly root="/private/tmp/autopus-adk-omp-canary-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
readonly destination="$root/omp-darwin-arm64"
readonly staging="$RUNNER_TEMP/omp-v17.2.7-darwin-arm64.download"
readonly receipt_staging="$RUNNER_TEMP/omp-canary-cleanup-identity"
readonly expected='cd2f47545cb3f8eb5e15c91bc9054d73967774652e020b432e294803d1b71ea0'

[[ "$(/usr/bin/stat -f '%u:%Sp' /private/tmp)" == '0:drwxrwxrwt' ]] \
  || fail '/private/tmp identity is unsafe'
[[ ! -e "$root" && ! -L "$root" ]] || fail 'canary root already exists'
[[ ! -e "$staging" && ! -L "$staging" ]] || fail 'download staging path already exists'
[[ ! -e "$receipt_staging" && ! -L "$receipt_staging" ]] \
  || fail 'receipt staging path already exists'

/usr/bin/sudo -n /usr/bin/install -d -m 0755 -o root -g wheel "$root"
/usr/bin/sudo -n /usr/bin/install -d -m 0700 -o nobody -g nobody \
  "$root/home" "$root/tmp"
/usr/bin/curl --proto '=https' --proto-redir '=https' --tlsv1.2 \
  --fail --location --silent --show-error \
  --output "$staging" \
  'https://github.com/can1357/oh-my-pi/releases/download/v17.2.7/omp-darwin-arm64'
actual=$(/usr/bin/shasum -a 256 "$staging" | /usr/bin/awk '{print $1}')
[[ "$actual" == "$expected" ]] || fail 'downloaded OMP digest differs'
/usr/bin/sudo -n /usr/bin/install -m 0555 -o root -g wheel "$staging" "$destination"
rm -f -- "$staging"

nobody_uid=$(/usr/bin/id -u nobody)
[[ "$(/usr/bin/stat -f '%u:%Lp' "$root")" == '0:755' ]] || fail 'canary root mode differs'
[[ "$(/usr/bin/stat -f '%u:%Lp' "$root/home")" == "$nobody_uid:700" ]] \
  || fail 'canary home ownership differs'
[[ "$(/usr/bin/stat -f '%u:%Lp' "$root/tmp")" == "$nobody_uid:700" ]] \
  || fail 'canary temp ownership differs'
[[ "$(/usr/bin/stat -f '%u:%Lp' "$destination")" == '0:555' ]] \
  || fail 'canary executable ownership differs'
/usr/bin/sudo -n -u nobody /bin/test -x "$destination" \
  || fail 'canary is not executable by nobody'
[[ "$(/usr/bin/sudo -n -u nobody /usr/bin/shasum -a 256 "$destination" | /usr/bin/awk '{print $1}')" == "$expected" ]] \
  || fail 'installed OMP digest differs'

/usr/bin/printf 'parent=%s\nroot=%s\nomp=%s\n' \
  "$(/usr/bin/stat -f '%d:%i' /private/tmp)" \
  "$(/usr/bin/stat -f '%d:%i' "$root")" \
  "$(/usr/bin/stat -f '%d:%i' "$destination")" >"$receipt_staging"
/usr/bin/sudo -n /usr/bin/install -m 0444 -o root -g wheel \
  "$receipt_staging" "$root/.cleanup-identity"
rm -f -- "$receipt_staging"
