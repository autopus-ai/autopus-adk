#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'key rotation publish: %s\n' "$1" >&2; exit 1; }
[[ $# -eq 2 ]] || fail 'usage: publish-key-rotation-sidecar.sh DOCUMENT SIGNATURE'
readonly document=$1 signature=$2
readonly repository='Insajin/autopus-adk'
readonly environment_name='adk-companion-release'
readonly rotation_ref='refs/heads/release-key-rotation-v0.50.109'
readonly document_name='adk-key-rotation-v1.json'
readonly signature_name='adk-key-rotation-v1.sig'
for path in "$document" "$signature"; do
  [[ -f "$path" && ! -L "$path" ]] || fail 'rotation sidecar input is unsafe'
done
[[ "$(wc -c <"$signature" | tr -d ' ')" == '64' ]] || fail 'rotation signature is not raw Ed25519'
for tool in cmp gh git go jq mktemp shasum wc; do
  command -v "$tool" >/dev/null || fail "${tool} is unavailable"
done
[[ "$(gh api user --jq .id)" == '204883817' ]] || fail 'publisher is not the authorized operator'
scripts/companion-release/verify-rotation-ref-ruleset.sh ||
  fail 'immutable rotation ref authority ruleset is unavailable'

repo_root=$(git rev-parse --show-toplevel)
[[ "$(pwd -P)" == "$repo_root" && -z "$(git status --porcelain)" ]] ||
  fail 'rotation publication requires a clean repository root'
[[ "$(git remote get-url origin)" =~ ^(https://github\.com/|git@github\.com:)(autopus-ai|Autopus-AI)/autopus-adk(\.git)?$ ]] ||
  fail 'origin is not the production repository'
source_commit=$(git rev-parse --verify 'HEAD^{commit}')
source_tree=$(git rev-parse --verify 'HEAD^{tree}')
git fetch --no-tags origin main
[[ "$(git rev-parse --verify origin/main)" == "$source_commit" ]] ||
  fail 'source is not exact origin/main'
readonly repo_root source_commit source_tree

repo_channel_key_id=$(gh variable get AUTOPUS_ADK_CHANNEL_KEY_ID --repo "$repository") ||
  fail 'repository channel key ID is unavailable'
env_channel_key_id=$(gh variable get AUTOPUS_ADK_CHANNEL_KEY_ID --repo "$repository" --env "$environment_name") ||
  fail 'environment channel key ID is unavailable'
repo_channel_public=$(gh variable get AUTOPUS_ADK_CHANNEL_PUBLIC_KEY --repo "$repository") ||
  fail 'repository channel public key is unavailable'
env_channel_public=$(gh variable get AUTOPUS_ADK_CHANNEL_PUBLIC_KEY --repo "$repository" --env "$environment_name") ||
  fail 'environment channel public key is unavailable'
[[ "$repo_channel_key_id" == 'adk-channel-2026-q3-a0' &&
   "$repo_channel_key_id" == "$env_channel_key_id" &&
   "$repo_channel_public" == "$env_channel_public" ]] ||
  fail 'repository and environment channel authority differs'
readonly repo_channel_key_id repo_channel_public

cleanup() { rm -rf -- "$temp_dir"; }
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/adk-key-rotation-publish.XXXXXX")
readonly temp_dir
chmod 0700 "$temp_dir"
trap cleanup EXIT
authority_dir="$temp_dir/key-rotation-authority"
authority_receipt=$(scripts/companion-release/materialize-key-rotation-authority.sh "$authority_dir") ||
  fail 'immutable key-rotation authority is unavailable'
jq -e '.authority_commit | test("^[0-9a-f]{40}$")' <<<"$authority_receipt" >/dev/null ||
  fail 'immutable key-rotation authority receipt is malformed'
verifier="$authority_dir/verify-rotation.sh"
verified="$temp_dir/verified.json"
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  "$verifier" verify-rotation \
  --document "$document" --signature "$signature" \
  --source-commit "$source_commit" --source-tree "$source_tree" \
  --next-tag-public-key scripts/companion-release/release-tag-signing-2026-q3-r2.pub \
  --next-tag-fingerprint scripts/companion-release/release-tag-signing-2026-q3-r2.fingerprint \
  --next-promotion-public-key scripts/companion-release/omp-context-promotion-2026-q3-k3.pub \
  >"$verified"
cmp -s "$document" "$verified" || fail 'rotation verifier did not return exact canonical bytes'

publication="$temp_dir/publication"
materialized="$temp_dir/materialized"
mkdir -m 0700 "$publication" "$materialized"
git init -q "$publication"
git -C "$publication" config user.name 'autopus-rotation-authority'
git -C "$publication" config user.email 'noreply@autopus.co'
git -C "$publication" config core.autocrlf false
git -C "$publication" config core.hooksPath /dev/null
git -C "$publication" config commit.gpgSign false
git -C "$publication" remote add origin "$(git remote get-url origin)"
git -C "$publication" checkout -q --orphan release-key-rotation-v0.50.109
cp -- "$document" "$publication/$document_name"
cp -- "$signature" "$publication/$signature_name"
git -C "$publication" add -- "$document_name" "$signature_name"
tracked=$(git -C "$publication" ls-files)
[[ "$tracked" == "$document_name"$'\n'"$signature_name" ]] ||
  fail 'rotation publication index contains unexpected files'
git -C "$publication" commit -q -m 'adk-channel: publish authorized v0.50.109 key rotation'
commit=$(git -C "$publication" rev-parse --verify HEAD)
[[ "$(git -C "$publication" rev-list --parents -n 1 HEAD | wc -w | tr -d ' ')" == '1' ]] ||
  fail 'rotation publication commit is not orphaned'
for name in "$document_name" "$signature_name"; do
  git -C "$publication" cat-file blob "HEAD:${name}" >"$materialized/$name"
done
cmp -s "$document" "$materialized/$document_name" || fail 'committed rotation document differs'
cmp -s "$signature" "$materialized/$signature_name" || fail 'committed rotation signature differs'

remote=$(git ls-remote --refs origin "$rotation_ref") || fail 'cannot inspect rotation ref'
if [[ -z "$remote" ]]; then
  push_status=0
  git -C "$publication" push origin "HEAD:${rotation_ref}" || push_status=$?
  remote=$(git ls-remote --refs origin "$rotation_ref") || fail 'cannot inspect rotation push outcome'
  if [[ "$remote" != "$commit"$'\t'"$rotation_ref" ]]; then
    [[ "$push_status" -ne 0 ]] || fail 'rotation push returned success without exact ref'
    fail 'rotation push outcome is inconsistent'
  fi
else
  remote_commit=${remote%%$'\t'*}
  [[ "$remote" == "$remote_commit"$'\t'"$rotation_ref" &&
     "$remote_commit" =~ ^[0-9a-f]{40}$ ]] ||
    fail 'immutable rotation ref is malformed'
  git fetch --no-tags origin "$rotation_ref" >/dev/null
  [[ "$(git rev-parse --verify FETCH_HEAD)" == "$remote_commit" &&
     "$(git rev-list --parents -n 1 FETCH_HEAD | wc -w | tr -d ' ')" == '1' &&
     "$(git ls-tree -r --name-only FETCH_HEAD)" == "$document_name"$'\n'"$signature_name" ]] ||
    fail 'existing rotation ref is not the canonical orphan publication'
  git cat-file blob "FETCH_HEAD:${document_name}" >"$materialized/remote-$document_name"
  git cat-file blob "FETCH_HEAD:${signature_name}" >"$materialized/remote-$signature_name"
  cmp -s "$document" "$materialized/remote-$document_name" ||
    fail 'existing rotation ref contains another document'
  cmp -s "$signature" "$materialized/remote-$signature_name" ||
    fail 'existing rotation ref contains another signature'
  commit=$remote_commit
fi

git fetch --no-tags origin "$rotation_ref" >/dev/null
[[ "$(git rev-parse --verify FETCH_HEAD)" == "$commit" ]] || fail 'published rotation ref differs'
rotation_document_sha256=$(shasum -a 256 "$document" | awk '{print $1}')
jq -cn --arg rotation_ref "$rotation_ref" --arg rotation_ref_commit "$commit" \
  --arg rotation_document_sha256 "$rotation_document_sha256" \
  '{rotation_ref:$rotation_ref,rotation_ref_commit:$rotation_ref_commit,
    rotation_document_sha256:$rotation_document_sha256}'
