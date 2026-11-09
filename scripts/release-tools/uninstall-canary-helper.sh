#!/usr/bin/env bash
# Removes the release canary helper and its sudoers rule, needing no password
# while the rule is still in place. Install again with
# install-canary-helper.sh before the next release.
set -euo pipefail
helper='/Library/PrivilegedHelperTools/co.autopus.release-canary'
rule='/etc/sudoers.d/autopus-release-canary'
fail() { printf 'uninstall canary helper: %s\n' "$1" >&2; exit 1; }
if [[ ! -e "$helper" ]]; then
  [[ ! -e "$rule" ]] || fail "the helper is gone but ${rule} remains; remove it with: sudo rm ${rule}"
  printf 'uninstall canary helper: nothing is installed\n'
  exit 0
fi
# Without the helper nothing can remove a leftover one-shot account, so a
# sweep that does not finish stops the uninstall.
/usr/bin/sudo -n "$helper" sweep || fail 'leftover canary accounts or roots remain; nothing was removed'
/usr/bin/sudo -n "$helper" uninstall
[[ ! -e "$helper" && ! -e "$rule" ]] || fail 'something was left behind'
printf 'uninstall canary helper: removed the helper and its sudoers rule\n'
