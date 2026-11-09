#!/usr/bin/env bash
# Internal function library sourced only by prepare-release.sh. The local
# live canary's privileged steps run through one root-owned helper that a
# sudoers rule lets the operator run without a password; see
# scripts/release-tools/install-canary-helper.sh.

readonly canary_helper_path='/Library/PrivilegedHelperTools/co.autopus.release-canary'

# verify_canary_helper refuses a helper that is not the one this source
# commit builds, or that someone other than root could replace.
verify_canary_helper() {
  local built=$1 path parent status=0
  [[ -f "$canary_helper_path" && ! -L "$canary_helper_path" &&
     "$(/usr/bin/stat -f '%u:%g:%Lp' "$canary_helper_path")" == '0:0:755' ]] ||
    fail "the release canary helper is not installed root-owned 0755 at ${canary_helper_path}; run scripts/release-tools/install-canary-helper.sh"
  path=$canary_helper_path
  while parent=$(/usr/bin/dirname "$path") && [[ "$parent" != "$path" ]]; do
    [[ ! -L "$parent" && "$(/usr/bin/stat -f '%u' "$parent")" == '0' &&
       $(( 8#$(/usr/bin/stat -f '%Lp' "$parent") & 8#022 )) -eq 0 ]] ||
      fail "${parent} could let a non-root account replace the release canary helper"
    path=$parent
  done
  [[ "$(shasum -a 256 "$canary_helper_path" | awk '{print $1}')" == "$(shasum -a 256 "$built" | awk '{print $1}')" ]] ||
    fail 'the installed release canary helper differs from this source commit; install-canary-helper.sh builds origin/main, so run it again once this commit is main'
  # With no arguments the helper prints its usage and exits 125; sudo itself
  # exits 1 when it would need a password.
  /usr/bin/sudo -n "$canary_helper_path" >/dev/null 2>&1 || status=$?
  [[ "$status" -eq 125 ]] || fail 'sudo does not let this account run the release canary helper without a password; run scripts/release-tools/install-canary-helper.sh'
}

canary_helper() { /usr/bin/sudo -n "$canary_helper_path" "$@"; }

# Probe mode still elevates its export tool with interactive sudo, so only
# it keeps a sudo timestamp alive; otherwise nothing has to stay alive.
sudo_keepalive_alive() {
  [[ "${probe_enabled:-0}" -eq 1 ]] || return 0
  kill -0 "$sudo_keepalive_pid" >/dev/null 2>&1
}
