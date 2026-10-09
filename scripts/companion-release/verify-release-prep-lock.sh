#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'release-prep lock verification: %s\n' "$1" >&2; exit 1; }
usage() {
  printf '%s\n' 'usage: verify-release-prep-lock.sh REF COMMIT REPORT' >&2
  exit 64
}
[[ $# -eq 3 ]] || usage
readonly lock_ref=$1 lock_commit=$2 report=$3
readonly expected_ref='refs/heads/omp-context-evidence-v0.50.125-source'
readonly report_name='omp-context-promotion-report.v1.json'

[[ "$lock_ref" == "$expected_ref" ]] || fail 'lock ref is not exact A35'
[[ "$lock_commit" =~ ^[0-9a-f]{40}$ ]] || fail 'lock commit is malformed'
[[ -f "$report" && ! -L "$report" ]] || fail 'expected report is unsafe'
for tool in awk cmp git; do command -v "$tool" >/dev/null || fail "$tool is unavailable"; done
[[ "$(git rev-parse --show-toplevel)" == "$(pwd -P)" ]] || fail 'verification must run at repository root'
remote_lock=$(git ls-remote --refs origin "$lock_ref") || fail 'cannot inspect remote prep lock'
[[ "$remote_lock" == "$lock_commit"$'\t'"$lock_ref" ]] || fail 'remote prep lock differs'
git fetch --no-tags origin "$lock_ref"
[[ "$(git rev-parse --verify FETCH_HEAD)" == "$lock_commit" ]] || fail 'fetched prep lock differs'
[[ "$(git cat-file -t "$lock_commit")" == 'commit' ]] || fail 'prep lock object is not a commit'
[[ "$(git rev-list --parents -n 1 "$lock_commit")" == "$lock_commit" ]] || fail 'prep lock is not an orphan'
lock_entry=$(git ls-tree "$lock_commit")
[[ "$(git ls-tree -r --name-only "$lock_commit")" == "$report_name" &&
   "${lock_entry%% *}" == '100644' ]] || fail 'prep lock tree is unsafe'
git cat-file blob "${lock_commit}:${report_name}" | cmp - "$report" || fail 'prep lock report differs'
[[ "$(git ls-remote --refs origin "$lock_ref")" == "$lock_commit"$'\t'"$lock_ref" ]] ||
  fail 'remote prep lock changed during verification'
printf '%s\n' "$lock_commit"
