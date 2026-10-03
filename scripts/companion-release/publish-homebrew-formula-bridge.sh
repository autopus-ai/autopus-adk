#!/usr/bin/env bash
set -euo pipefail
umask 077

readonly RELEASE_TAG='v0.50.122'
readonly RELEASE_VERSION='0.50.122'
readonly RELEASE_POLICY='cask-only'
readonly TAP_REPOSITORY='Insajin/homebrew-autopus'
readonly TAP_BRANCH='main'
readonly PRIOR_TAP_COMMIT='ce5abeba3bbac26ad1240e29bd5360bcf743d772'
readonly CASK_PATH='Casks/auto.rb'
readonly PRIOR_CASK_BLOB='386eaa06151b6def02db763ccfe3a7984c8338ce'
readonly FORMULA_PATH='Formula/auto.rb'
readonly FROZEN_FORMULA_BLOB='4ebc6c38925002dec00759823d4dd847a499818a'

fail() {
  printf 'homebrew cask publication: %s\n' "$1" >&2
  exit 1
}

require_environment() {
  local name="$1"
  [[ -n "${!name-}" ]] || fail "required environment variable ${name} is missing"
}

for name in \
  GITHUB_REF_NAME COMPANION_VERSION COMPANION_HOMEBREW_POLICY COMPANION_CHECKSUMS_PATH
do
  require_environment "$name"
done
[[ "$GITHUB_REF_NAME" == "$RELEASE_TAG" ]] \
  || fail 'release tag is outside the exact Cask publication policy'
[[ "$COMPANION_VERSION" == "$RELEASE_VERSION" ]] \
  || fail 'release version is outside the exact Cask publication policy'
[[ "$COMPANION_HOMEBREW_POLICY" == "$RELEASE_POLICY" ]] \
  || fail 'release policy is not exact cask-only publication'

tap_token="${HOMEBREW_TAP_TOKEN-}"
if [[ -z "$tap_token" ]]; then
  tap_token="${GH_TOKEN-}"
fi
[[ -n "$tap_token" ]] \
  || fail 'HOMEBREW_TAP_TOKEN or GH_TOKEN is required for the tap API'

for tool in gh jq base64 mktemp cmp tr; do
  command -v "$tool" >/dev/null 2>&1 || fail "required tool ${tool} is unavailable"
done
if command -v shasum >/dev/null 2>&1; then
  sha256_command=(shasum -a 256)
elif command -v sha256sum >/dev/null 2>&1; then
  sha256_command=(sha256sum)
else
  fail 'required SHA-256 tool shasum or sha256sum is unavailable'
fi
if printf '' | base64 --decode >/dev/null 2>&1; then
  base64_decode=(base64 --decode)
elif printf '' | base64 -D >/dev/null 2>&1; then
  base64_decode=(base64 -D)
else
  fail 'base64 decoder is unavailable'
fi

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) \
  || fail 'cannot resolve Formula bridge script directory'
render_helper="$script_dir/publish-homebrew-formula-bridge-render.sh"
[[ -f "$render_helper" && ! -L "$render_helper" ]] \
  || fail 'Formula bridge renderer is not a regular non-symlink file'
# shellcheck source=publish-homebrew-formula-bridge-render.sh
source "$render_helper"
declare -F render_homebrew_cask >/dev/null \
  || fail 'Cask renderer contract is incomplete'

checksums_path="$COMPANION_CHECKSUMS_PATH"
[[ -f "$checksums_path" && ! -L "$checksums_path" ]] \
  || fail 'checksums input is not a regular file'

darwin_amd64_name="autopus-adk_${RELEASE_VERSION}_darwin_amd64.tar.gz"
darwin_arm64_name="autopus-adk_${RELEASE_VERSION}_darwin_arm64.tar.gz"
linux_amd64_name="autopus-adk_${RELEASE_VERSION}_linux_amd64.tar.gz"
linux_arm64_name="autopus-adk_${RELEASE_VERSION}_linux_arm64.tar.gz"
darwin_amd64_sha='' darwin_arm64_sha='' linux_amd64_sha='' linux_arm64_sha=''
checksum_pattern='^([0-9a-f]{64})  ([A-Za-z0-9][A-Za-z0-9._+-]*)$'
seen_names=()
line_count=0
while IFS= read -r line || [[ -n "$line" ]]; do
  line_count=$((line_count + 1))
  [[ "$line" =~ $checksum_pattern ]] \
    || fail "checksums input has a malformed entry at line ${line_count}"
  digest="${BASH_REMATCH[1]}"
  archive="${BASH_REMATCH[2]}"
  for seen_name in "${seen_names[@]-}"; do
    [[ "$archive" != "$seen_name" ]] \
      || fail "checksums input has a duplicate archive at line ${line_count}"
  done
  seen_names+=("$archive")
  case "$archive" in
    "$darwin_amd64_name") darwin_amd64_sha="$digest" ;;
    "$darwin_arm64_name") darwin_arm64_sha="$digest" ;;
    "$linux_amd64_name") linux_amd64_sha="$digest" ;;
    "$linux_arm64_name") linux_arm64_sha="$digest" ;;
    *'_darwin_'*|*'_linux_'*)
      fail "checksums input has an unexpected Darwin or Linux archive at line ${line_count}"
      ;;
  esac
done <"$checksums_path"
for digest in \
  "$darwin_amd64_sha" "$darwin_arm64_sha" "$linux_amd64_sha" "$linux_arm64_sha"
do
  [[ "$digest" =~ ^[0-9a-f]{64}$ ]] \
    || fail 'checksums input is missing an exact bridge archive'
done

temp_dir=''
cleanup() {
  local status=$?
  if [[ -n "$temp_dir" ]]; then
    rm -rf -- "$temp_dir" || status=$?
  fi
  return "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/autopus-homebrew-bridge.XXXXXX") \
  || fail 'cannot create private temporary directory'

git_helper="$script_dir/publish-homebrew-formula-bridge-git.sh"
[[ -f "$git_helper" && ! -L "$git_helper" ]] \
  || fail 'Formula bridge Git helper is not a regular non-symlink file'
# shellcheck source=publish-homebrew-formula-bridge-git.sh
source "$git_helper"
for contract in api_get api_json verify_frozen_formula verify_prior_tap_head \
  decode_api_content sha256_file publish_cask
do
  declare -F "$contract" >/dev/null || fail 'Formula bridge Git contract is incomplete'
done

cask_target="$temp_dir/cask-target.rb"
render_homebrew_cask "$cask_target" "$RELEASE_VERSION" \
  "$darwin_amd64_sha" "$darwin_arm64_sha" "$linux_amd64_sha" "$linux_arm64_sha"
if [[ -n "${COMPANION_CASK_PATH-}" ]]; then
  [[ -f "$COMPANION_CASK_PATH" && ! -L "$COMPANION_CASK_PATH" ]] \
    || fail 'GoReleaser Cask output is not a regular file'
  cmp -s "$cask_target" "$COMPANION_CASK_PATH" \
    || fail 'GoReleaser Cask output differs from the canonical renderer'
fi

verify_frozen_formula
publish_cask cask Cask "$CASK_PATH" "$cask_target" "$PRIOR_CASK_BLOB" \
  'Publish signed Cask for v0.50.122' \
  'published Cask differs from canonical v0.50.122 output and its pinned prior blob'
