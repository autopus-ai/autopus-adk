#!/usr/bin/env bash
# Builds the release canary helper from GitHub's main branch, installs it root-owned
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
temp_base=${TMPDIR:-/tmp}
temp=$(mktemp -d "${temp_base%/}/install-canary-helper.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT

# What becomes a passwordless root program is GitHub's main branch, fetched
# fresh into an empty repository with every git configuration, replace ref,
# and attribute ignored, and built with a clean Go environment. Nothing in
# this checkout's .git (refs, refspecs, replace objects, filters) or in the
# caller's GOFLAGS, GOWORK, module cache, or proxy settings reaches the build.
# A caller who controls this account's PATH or shell startup, or who rewrites
# the build output in this account's temp directory before its hash is taken,
# is out of reach of any script; this one does not claim otherwise.
readonly source_url='https://github.com/autopus-ai/autopus-adk.git'
go_bin=$(command -v go) || fail 'go is not installed'
clean_git() {
  /usr/bin/env -i HOME="$temp" PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    GIT_NO_REPLACE_OBJECTS=1 GIT_ATTR_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0 /usr/bin/git -C "$temp/repo" "$@"
}
/usr/bin/env -i HOME="$temp" PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
  /usr/bin/git init --quiet --bare --template= "$temp/repo"
clean_git fetch --quiet --no-tags "$source_url" refs/heads/main
commit=$(clean_git rev-parse --verify 'FETCH_HEAD^{commit}')
mkdir "$temp/src"
clean_git archive "$commit" | /usr/bin/tar -x -C "$temp/src"
printf 'building from %s main %s\n' "$source_url" "$commit"
(cd "$temp/src" && /usr/bin/env -i HOME="$temp" PATH="$(dirname "$go_bin"):/usr/bin:/bin" \
  GOENV=off GOFLAGS= GOWORK=off GOTOOLCHAIN=go1.27.4 \
  GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org GOMODCACHE="$temp/mod" GOCACHE="$temp/cache" \
  "$go_bin" build -trimpath -buildvcs=false -o "$temp/canaryhelper" ./scripts/companion-release/canaryhelper)
built_sha=$(/usr/bin/shasum -a 256 "$temp/canaryhelper" | /usr/bin/awk '{print $1}')
printf '%s ALL=(root) NOPASSWD: %s\n' "$account" "$destination" >"$temp/rule"
/usr/sbin/visudo -cqf "$temp/rule" || fail 'the generated sudoers rule does not parse'

printf 'installing %s (sha256 %s)\n' "$destination" "$built_sha"
printf 'and %s: %s\n' "$rule" "$(cat "$temp/rule")"
/usr/bin/sudo /usr/bin/install -o root -g wheel -m 0755 "$temp/canaryhelper" "$destination"
# The build output was this account's file until install copied it; check
# what landed is what was built.
if [[ "$(/usr/bin/shasum -a 256 "$destination" | /usr/bin/awk '{print $1}')" != "$built_sha" ]]; then
  /usr/bin/sudo /bin/rm -f "$destination"
  fail 'the installed helper differs from the one just built; removed it'
fi
/usr/bin/sudo /usr/bin/install -o root -g wheel -m 0440 "$temp/rule" "$rule"
/usr/bin/sudo /usr/sbin/visudo -cq || fail 'sudoers no longer parses; remove the rule with sudo rm /etc/sudoers.d/autopus-release-canary'
/usr/bin/sudo -k
/usr/bin/sudo -n "$destination" >/dev/null 2>&1 && status=0 || status=$?
[[ "$status" -ne 1 ]] || fail 'the rule did not take effect'
printf 'done: the release prep can now run the canary helper without a password\n'
