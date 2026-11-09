#!/usr/bin/env bash
# Builds the release canary helper from this checkout, installs it root-owned
# at /Library/PrivilegedHelperTools/co.autopus.release-canary, and adds a
# sudoers rule that lets the current account run that one file without a
# password. After this, `release-prep.sh --apply` needs no interactive sudo.
#
# Run it once, and again whenever prepare-release reports that the installed
# helper differs from the source commit. It asks for your password itself.
# Remove with: sudo rm /etc/sudoers.d/autopus-release-canary /Library/PrivilegedHelperTools/co.autopus.release-canary
set -euo pipefail
umask 077

fail() { printf 'install canary helper: %s\n' "$1" >&2; exit 1; }
[[ "$(uname -s)" == 'Darwin' ]] || fail 'macOS only'
[[ "$(id -u)" -ne 0 ]] || fail 'run as your own account; the script calls sudo where it needs root'
account=$(id -un)
[[ "$account" =~ ^[a-z_][a-z0-9_.-]*$ ]] || fail "account name ${account} cannot be written into a sudoers rule safely"

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
readonly destination='/Library/PrivilegedHelperTools/co.autopus.release-canary'
readonly rule='/etc/sudoers.d/autopus-release-canary'
temp=$(mktemp -d)
trap 'rm -rf -- "$temp"' EXIT

env GOENV=off GOTOOLCHAIN=go1.27.3 go build -trimpath -buildvcs=false -o "$temp/canaryhelper" ./scripts/companion-release/canaryhelper
printf '%s ALL=(root) NOPASSWD: %s\n' "$account" "$destination" >"$temp/rule"
/usr/sbin/visudo -cqf "$temp/rule" || fail 'the generated sudoers rule does not parse'

printf 'installing %s (sha256 %s)\n' "$destination" "$(shasum -a 256 "$temp/canaryhelper" | awk '{print $1}')"
printf 'and %s: %s\n' "$rule" "$(cat "$temp/rule")"
/usr/bin/sudo /usr/bin/install -o root -g wheel -m 0755 "$temp/canaryhelper" "$destination"
/usr/bin/sudo /usr/bin/install -o root -g wheel -m 0440 "$temp/rule" "$rule"
/usr/bin/sudo /usr/sbin/visudo -cq || fail 'sudoers no longer parses; remove the rule with sudo rm /etc/sudoers.d/autopus-release-canary'
/usr/bin/sudo -k
/usr/bin/sudo -n "$destination" >/dev/null 2>&1 && status=0 || status=$?
[[ "$status" -ne 1 ]] || fail 'the rule did not take effect'
printf 'done: the release prep can now run the canary helper without a password\n'
