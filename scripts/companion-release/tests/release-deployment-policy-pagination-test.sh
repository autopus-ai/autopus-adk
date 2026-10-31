#!/usr/bin/env bash
set -euo pipefail

# v0.50.130 was the 31st deployment policy by name. The coordinate publisher
# read one unpaginated page, saw no policy, tried to create one, and the first
# --apply stopped with exit 75. Every reader must see past the first page.
tests_dir=$(cd "$(dirname "$0")" && pwd)
script_dir=$(cd "$tests_dir/.." && pwd)
mock_gh="$tests_dir/testdata/mock-release-prep-gh.sh"
fail() { printf 'deployment policy pagination test: %s\n' "$1" >&2; exit 1; }
temp=$(mktemp -d)
trap 'rm -rf -- "$temp"' EXIT
mkdir -p "$temp/bin" "$temp/state"
cp "$mock_gh" "$temp/bin/gh"; chmod 0700 "$temp/bin/gh"
state="$temp/state"
export MOCK_RELEASE_PREP_STATE="$state" PATH="$temp/bin:$PATH"
printf '0\n' >"$state/write-count"; : >"$state/calls.log"

# 150 tag policies, v0.50.0 through v0.50.149. By name v0.50.99 sorts last,
# past both the default page of 30 and a page of 100.
jq -n '[range(0; 150) | {id: (1000 + .), type: "tag", name: "v0.50.\(.)"}]' \
  >"$state/deployment-policies.json"
endpoint='repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies'

# The fixture must model GitHub: an unpaginated read stops at 30.
first_page=$(gh api "$endpoint")
jq -e '.total_count == 150 and (.branch_policies | length) == 30' <<<"$first_page" >/dev/null ||
  fail 'mock does not page like GitHub'
jq -e '[.branch_policies[] | select(.name == "v0.50.99")] | length == 0' <<<"$first_page" >/dev/null ||
  fail 'fixture puts v0.50.99 on the first page'

repository='autopus-ai/autopus-adk'
environment_name='adk-companion-release'
# shellcheck source=/dev/null
source "$script_dir/release-coordinate-transaction-lib.sh"
policies=$(deployment_policies_json) || fail 'complete policy read failed'
jq -e '.total_count == 150 and (.branch_policies | length) == 150' <<<"$policies" >/dev/null ||
  fail 'policy read is incomplete'
jq -e '[.branch_policies[] | select(.type == "tag" and .name == "v0.50.99")] | length == 1' \
  <<<"$policies" >/dev/null || fail 'policy read misses the last page'

# A page that disagrees with total_count fails closed instead of reading as absent.
cat >"$temp/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '[{"total_count":31,"branch_policies":[{"id":1,"type":"tag","name":"v0.50.1"}]}]'
EOF
chmod 0700 "$temp/bin/gh"
if deployment_policies_json >/dev/null 2>&1; then
  fail 'short policy read was accepted'
fi
cp "$mock_gh" "$temp/bin/gh"; chmod 0700 "$temp/bin/gh"

# The ruleset gate reads the same environment and must see past a page of 100.
pages=$(gh api --paginate --slurp "$endpoint?per_page=100")
jq -e 'length == 2 and ([.[].branch_policies[]] | length) == 150' <<<"$pages" >/dev/null ||
  fail 'mock --paginate --slurp does not return every page'
grep -Fq 'gh api --paginate --slurp' "$script_dir/verify-release-tag-ruleset.sh" ||
  fail 'ruleset gate reads one page of deployment policies'

# No reader in the coordinate path may go back to a single page.
for reader in publish-release-coordinates.sh release-coordinate-transaction-lib.sh; do
  if grep -E 'gh api "repos/\$\{repository\}/environments/\$\{environment_name\}/deployment-branch-policies"' \
    "$script_dir/$reader" >/dev/null; then
    fail "$reader reads one page of deployment policies"
  fi
done
printf '%s\n' 'deployment policy pagination test: ok'
