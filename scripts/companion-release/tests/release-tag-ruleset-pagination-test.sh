#!/usr/bin/env bash
set -euo pipefail

# v0.51.4's release ruleset was the 34th in the repository. The ruleset gate
# and the coordinate publisher read one unpaginated page of 30, saw no
# ruleset, and `verify-release-tag-ruleset.sh --armed` reported it missing
# right after it was created. Every ruleset reader must see past a page.
tests_dir=$(cd "$(dirname "$0")" && pwd)
script_dir=$(cd "$tests_dir/.." && pwd)
mock_gh="$tests_dir/testdata/mock-release-prep-gh.sh"
fail() { printf 'release tag ruleset pagination test: %s\n' "$1" >&2; exit 1; }
temp=$(mktemp -d)
trap 'rm -rf -- "$temp"' EXIT
mkdir -p "$temp/bin" "$temp/state"
cp "$mock_gh" "$temp/bin/gh"; chmod 0700 "$temp/bin/gh"
state="$temp/state"
export MOCK_RELEASE_PREP_STATE="$state" PATH="$temp/bin:$PATH"
printf '0\n' >"$state/write-count"; : >"$state/calls.log"
printf 'armed\n' >"$state/ruleset-state"
jq -n '[{id: 596, type: "tag", name: "v0.51.7"}]' >"$state/deployment-policies.json"

# 150 tag rulesets; the release ruleset is the last one, past both the
# default page of 30 and a page of 100.
jq -n '[range(0; 149) | {id: (2000 + .), name: "older-\(.)", target: "tag"}] +
  [{id: 777, name: "autopus-v0.51.7-release-authority", target: "tag"}]' \
  >"$state/rulesets.json"
endpoint='repos/autopus-ai/autopus-adk/rulesets?includes_parents=true&targets=tag'

# The fixture must model GitHub: an unpaginated read stops at 30.
first_page=$(gh api "$endpoint")
jq -e 'length == 30 and ([.[] | select(.id == 777)] | length) == 0' <<<"$first_page" >/dev/null ||
  fail 'mock does not page like GitHub'
pages=$(gh api --paginate --slurp "$endpoint&per_page=100")
jq -e 'length == 2 and ([.[][]] | length) == 150' <<<"$pages" >/dev/null ||
  fail 'mock --paginate --slurp does not return every page'

# The gate finds the ruleset on the last page.
out=$(bash "$script_dir/verify-release-tag-ruleset.sh" --armed 2>&1) ||
  fail "ruleset gate misses a ruleset past the first page: $out"

# No ruleset reader may go back to a single page.
for reader in verify-release-tag-ruleset.sh publish-release-coordinates.sh verify-rotation-ref-ruleset.sh; do
  if grep -E 'gh api "repos/\$\{repository\}/rulesets\?' "$script_dir/$reader" >/dev/null; then
    fail "$reader reads one page of rulesets"
  fi
done
printf '%s\n' 'release tag ruleset pagination test: ok'
