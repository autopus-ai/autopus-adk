#!/usr/bin/env bash
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd -- "$tests_dir/.." && pwd)
fail() { printf 'release homebrew hardening test: %s\n' "$1" >&2; exit 1; }

# The mock tap head must equal the pin the publisher enforces, or every case
# below stops at the head check instead of exercising the drift it targets.
# Reading the pin keeps the fixture from desyncing when a release bumps it.
prior_tap_commit=$(sed -n "s/^readonly PRIOR_TAP_COMMIT='\([0-9a-f]\{40\}\)'\$/\1/p" \
  "$script_dir/publish-homebrew-formula-bridge.sh")
[[ -n "$prior_tap_commit" ]] || fail 'cannot read PRIOR_TAP_COMMIT from the publisher'
prior_cask_blob=$(sed -n "s/^readonly PRIOR_CASK_BLOB='\([0-9a-f]\{40\}\)'$/\1/p" \
  "$script_dir/publish-homebrew-formula-bridge.sh")
[[ -n "$prior_cask_blob" ]] || fail 'cannot read PRIOR_CASK_BLOB from the publisher'

temp=$(mktemp -d "${TMPDIR:-/tmp}/release-homebrew-hardening-test.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT
state="$temp/tap-state"
mkdir -m 0700 "$state" "$temp/bin"
install -m 0700 "$tests_dir/testdata/mock-tap-gh.sh" "$temp/bin/gh"
checksums="$temp/checksums.txt"
{
  printf '%064d  autopus-adk_0.50.122_darwin_amd64.tar.gz\n' 1
  printf '%064d  autopus-adk_0.50.122_darwin_arm64.tar.gz\n' 2
  printf '%064d  autopus-adk_0.50.122_linux_amd64.tar.gz\n' 3
  printf '%064d  autopus-adk_0.50.122_linux_arm64.tar.gz\n' 4
} >"$checksums"

# A33 updates only the Cask from the exact A32 tap head and keeps Formula frozen.
# The digests are A32's published archive digests, so rendering them has to
# reproduce the live tap blob byte for byte. That equality is what proves the
# PRIOR_* pins name the tap this release actually builds on.
source "$script_dir/publish-homebrew-formula-bridge-render.sh"
render_homebrew_cask "$temp/prior-cask.rb" 0.50.121 \
  '6df337b4ff4efb01f4070e941e31df37bb81f647e75b230b826e9ae5c0fd7661' \
  'de90e3165a504c8c8dbe128114c5f2b9fc9c72890d12d1cc3695c3aea3e7dd17' \
  'ddf313f8b0e0b297086b5b95458d9fa371e921792eff42d5a22bf838a9294e34' \
  '2684b264b9fd95e77001dd77afb6bccb9b474a7bb03a06311d0e802a2b0bab0a'
[[ "$(git -C "$temp" hash-object "$temp/prior-cask.rb")" == \
   "$prior_cask_blob" ]] \
  || fail 'rendered A32 Cask bytes differ from the pinned predecessor blob'
render_homebrew_formula_bridge "$temp/frozen-formula.rb" v0.50.71 0.50.71 \
  "$(printf '%064d' 1)" "$(printf '%064d' 2)" \
  "$(printf '%064d' 3)" "$(printf '%064d' 4)"
jq -n --arg content "$(base64 <"$temp/prior-cask.rb" | tr -d '\r\n')" \
  --arg sha "$prior_cask_blob" '{sha:$sha,content:$content}' >"$state/cask.json"
jq -n --arg content "$(base64 <"$temp/frozen-formula.rb" | tr -d '\r\n')" \
  '{sha:"4ebc6c38925002dec00759823d4dd847a499818a",content:$content}' >"$state/formula.json"
jq -n --arg sha "$prior_tap_commit" '{ref:"refs/heads/main",object:{type:"commit",sha:$sha,url:"https://example.invalid/prior-commit"}}' \
  >"$state/branch.json"
cp "$state/formula.json" "$temp/formula-before.json"
bridge_env=(PATH="$temp/bin:$PATH" MOCK_TAP_STATE="$state" GITHUB_REF_NAME=v0.50.122
  MOCK_TAP_PRIOR_COMMIT="$prior_tap_commit"
  COMPANION_VERSION=0.50.122 COMPANION_HOMEBREW_POLICY=cask-only
  COMPANION_CHECKSUMS_PATH="$checksums" HOMEBREW_TAP_TOKEN=fixture)
env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh"
[[ "$(<"$state/ref-update.calls")" == 1 &&
   "$(<"$state/blob-create.calls")" == 1 &&
   "$(<"$state/tree-create.calls")" == 1 &&
   "$(<"$state/commit-create.calls")" == 1 &&
   "$(<"$state/formula-get.calls")" == 1 ]] \
  || fail 'A33 did not update only the Cask'
cmp -s "$temp/formula-before.json" "$state/formula.json" \
  || fail 'frozen v0.50.71 Formula blob or bytes changed'
env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh"
[[ "$(<"$state/ref-update.calls")" == 1 &&
   "$(<"$state/blob-create.calls")" == 1 &&
   "$(<"$state/tree-create.calls")" == 1 &&
   "$(<"$state/commit-create.calls")" == 1 &&
   "$(<"$state/formula-get.calls")" == 2 ]] \
  || fail 'A33 Cask-only reconciler is not idempotent'

# An already-current Cask must bind to one stable head with the frozen Formula.
touch "$state/idempotent-formula-race"
if env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh" \
  >/dev/null 2>&1; then
  fail 'A33 accepted idempotent Cask bytes across concurrent Formula drift'
fi
rm -f -- "$state/idempotent-formula-race"
[[ "$(<"$state/ref-update.calls")" == 1 &&
   "$(jq -er '.object.sha' "$state/branch.json")" == \
     '8888888888888888888888888888888888888888' &&
   "$(jq -er '.sha' "$state/formula.json")" == \
     '6666666666666666666666666666666666666666' ]] \
  || fail 'A33 mutated tap state after idempotent Formula drift'
jq -n '{ref:"refs/heads/main",object:{type:"commit",sha:"3333333333333333333333333333333333333333",url:"https://example.invalid/target-commit"}}' \
  >"$state/branch.json"
jq -n --arg content "$(base64 <"$temp/frozen-formula.rb" | tr -d '\r\n')" \
  '{sha:"4ebc6c38925002dec00759823d4dd847a499818a",content:$content}' >"$state/formula.json"
rm -f -- "$state/branch-get.calls"
touch "$state/idempotent-ref-race"
if env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh" \
  >/dev/null 2>&1; then
  fail 'A33 accepted a branch move after idempotent tree verification'
fi
rm -f -- "$state/idempotent-ref-race"
[[ "$(<"$state/ref-update.calls")" == 1 &&
   "$(jq -er '.object.sha' "$state/branch.json")" == \
     '4444444444444444444444444444444444444444' ]] \
  || fail 'A33 updated Cask during idempotent head verification'

# An update-needed retry must reject pre-existing tap-head or Formula drift.
jq -n --arg content "$(base64 <"$temp/prior-cask.rb" | tr -d '\r\n')" \
  --arg sha "$prior_cask_blob" '{sha:$sha,content:$content}' >"$state/cask.json"
jq -n '{ref:"refs/heads/main",object:{type:"commit",sha:"4444444444444444444444444444444444444444",url:"https://example.invalid/racer"}}' \
  >"$state/branch.json"
if env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh" \
  >/dev/null 2>&1; then
  fail 'A33 accepted a drifted Homebrew tap predecessor commit'
fi
[[ "$(<"$state/ref-update.calls")" == 1 ]] \
  || fail 'A33 updated Cask after predecessor commit drift'
jq -n --arg sha "$prior_tap_commit" '{ref:"refs/heads/main",object:{type:"commit",sha:$sha,url:"https://example.invalid/prior-commit"}}' \
  >"$state/branch.json"
jq -n --arg content "$(base64 <"$temp/frozen-formula.rb" | tr -d '\r\n')" \
  '{sha:"5555555555555555555555555555555555555555",content:$content}' >"$state/formula.json"
if env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh" \
  >/dev/null 2>&1; then
  fail 'A33 accepted frozen Formula blob drift'
fi
[[ "$(<"$state/ref-update.calls")" == 1 ]] \
  || fail 'A33 mutated tap state after frozen Formula drift'

# A branch move after the head check must make the non-force ref CAS fail.
jq -n --arg content "$(base64 <"$temp/frozen-formula.rb" | tr -d '\r\n')" \
  '{sha:"4ebc6c38925002dec00759823d4dd847a499818a",content:$content}' >"$state/formula.json"
jq -n --arg sha "$prior_tap_commit" '{ref:"refs/heads/main",object:{type:"commit",sha:$sha,url:"https://example.invalid/prior-commit"}}' \
  >"$state/branch.json"
touch "$state/race-before-ref"
if env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh" \
  >/dev/null 2>&1; then
  fail 'A33 accepted a concurrent Homebrew branch move'
fi
rm -f -- "$state/race-before-ref"
[[ "$(<"$state/ref-update.calls")" == 1 &&
   "$(jq -er '.object.sha' "$state/branch.json")" == \
     '4444444444444444444444444444444444444444' ]] \
  || fail 'A33 overwrote a concurrent Homebrew branch move'

# A concurrent Formula-changing commit must also win the race and reject A33.
jq -n --arg content "$(base64 <"$temp/frozen-formula.rb" | tr -d '\r\n')" \
  '{sha:"4ebc6c38925002dec00759823d4dd847a499818a",content:$content}' >"$state/formula.json"
jq -n --arg sha "$prior_tap_commit" '{ref:"refs/heads/main",object:{type:"commit",sha:$sha,url:"https://example.invalid/prior-commit"}}' \
  >"$state/branch.json"
touch "$state/formula-race-before-ref"
if env "${bridge_env[@]}" bash "$script_dir/publish-homebrew-formula-bridge.sh" \
  >/dev/null 2>&1; then
  fail 'A33 accepted a concurrent Formula drift commit'
fi
rm -f -- "$state/formula-race-before-ref"
[[ "$(<"$state/ref-update.calls")" == 1 &&
   "$(jq -er '.sha' "$state/formula.json")" == \
     '6666666666666666666666666666666666666666' ]] \
  || fail 'A33 overwrote a concurrent Formula drift commit'

# Tap drift must fail before anything irreversible exists. In normal prep that
# means before the prep lock and coordinate transaction; in the release
# workflow it means before GoReleaser, because a published immutable release
# can never get its Cask afterwards.
prep="$script_dir/prepare-release.sh"
gate="$script_dir/verify-homebrew-tap-pins.sh"
release_workflow="$script_dir/../../.github/workflows/release.yaml"
[[ -f "$gate" && ! -L "$gate" && -x "$gate" ]] || fail 'missing or unsafe tap pin gate'
grep -Fq 'verify-homebrew-tap-pins.sh' "$prep" || fail 'prep does not delegate to the gate'
grep -Fq "PRIOR_TAP_COMMIT" "$gate" || fail 'gate does not read the tap head pin'
grep -Fq "PRIOR_CASK_BLOB" "$gate" || fail 'gate does not read the Cask blob pin'
(( $(grep -nF 'verify-homebrew-tap-pins.sh' "$prep" | cut -d: -f1) < \
   $(grep -nF 'if [[ "$operation" == '\''preflight'\'' ]]' "$prep" | cut -d: -f1) )) \
  || fail 'Homebrew tap pins are checked after release mutation admission'
(( $(grep -n 'verify-homebrew-tap-pins.sh' "$release_workflow" | tail -1 | cut -d: -f1) < \
   $(grep -n 'goreleaser release --clean' "$release_workflow" | cut -d: -f1) )) \
  || fail 'Homebrew tap pins are checked after GoReleaser publishes'

printf 'release homebrew hardening test: PASS\n'
