#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'key rotation sidecar: %s\n' "$1" >&2; exit 1; }
usage() {
  printf '%s\n' 'usage: verify-key-rotation-sidecar.sh [--public-ruleset] [--historical] VERIFIER SOURCE_COMMIT SOURCE_TREE OUTPUT_DIR [DOCUMENT SIGNATURE]' >&2
  exit 64
}
public_ruleset=0
historical=0
while [[ "${1-}" == --* ]]; do
  case "$1" in
    --public-ruleset)
      [[ "$public_ruleset" -eq 0 ]] || usage
      public_ruleset=1
      ;;
    --historical)
      [[ "$historical" -eq 0 ]] || usage
      historical=1
      ;;
    *) usage ;;
  esac
  shift
done
[[ $# -eq 4 || $# -eq 6 ]] || usage
readonly verifier=$1 source_commit=$2 source_tree=$3 output_dir=$4
readonly supplied_document=${5-} supplied_signature=${6-}
verify_command='verify-rotation'
[[ "$historical" -eq 0 ]] || verify_command='verify-rotation-historical'
readonly public_ruleset historical verify_command
readonly rotation_ref='refs/heads/release-key-rotation-v0.50.109'
readonly document_name='adk-key-rotation-v1.json'
readonly signature_name='adk-key-rotation-v1.sig'
for tool in awk cmp git install jq shasum tr wc; do
  command -v "$tool" >/dev/null || fail "$tool is unavailable"
done
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) ||
  fail 'cannot resolve rotation verifier directory'
channel_key_id_pin="$script_dir/adk-channel-2026-q3-a0.key-id"
channel_public_key_pin="$script_dir/adk-channel-2026-q3-a0.public.b64"
for pin in "$channel_key_id_pin" "$channel_public_key_pin"; do
  [[ -f "$pin" && ! -L "$pin" ]] || fail 'historical channel authority pin is unsafe'
done
IFS= read -r pinned_channel_key_id <"$channel_key_id_pin" ||
  fail 'cannot read historical channel key ID'
IFS= read -r pinned_channel_public_key <"$channel_public_key_pin" ||
  fail 'cannot read historical channel public key'
[[ "$pinned_channel_key_id" == 'adk-channel-2026-q3-a0' &&
   "$pinned_channel_public_key" =~ ^[A-Za-z0-9+/]+={0,2}$ &&
   "$(wc -l <"$channel_key_id_pin" | tr -d ' ')" == '1' &&
   "$(wc -l <"$channel_public_key_pin" | tr -d ' ')" == '1' ]] ||
  fail 'historical channel authority pin differs'
readonly script_dir channel_key_id_pin channel_public_key_pin
readonly pinned_channel_key_id pinned_channel_public_key
[[ "$source_commit" =~ ^[0-9a-f]{40}$ && "$source_tree" =~ ^[0-9a-f]{40}$ ]] ||
  fail 'source coordinates are malformed'
[[ -f "$verifier" && ! -L "$verifier" && -x "$verifier" ]] || fail 'rotation verifier is unsafe'
if [[ "$historical" -eq 0 ]]; then
  [[ "${AUTOPUS_ADK_CHANNEL_KEY_ID:-}" == "$pinned_channel_key_id" &&
     "${AUTOPUS_ADK_CHANNEL_PUBLIC_KEY:-}" == "$pinned_channel_public_key" ]] ||
    fail 'current ADK channel authority differs from bridge authority'
fi
[[ ! -e "$output_dir" && ! -L "$output_dir" ]] || fail 'output directory already exists'
for supplied in "$supplied_document" "$supplied_signature"; do
  [[ -z "$supplied" || (-f "$supplied" && ! -L "$supplied") ]] ||
    fail 'supplied sidecar input is unsafe'
done
[[ "$(git rev-parse --show-toplevel)" == "$(pwd -P)" ]] ||
  fail 'rotation verification must run at repository root'
[[ "$(git remote get-url origin)" =~ ^(https://github\.com/|git@github\.com:)(autopus-ai|Autopus-AI)/autopus-adk(\.git)?$ ]] ||
  fail 'origin is not the production repository'
if [[ "$public_ruleset" -eq 1 ]]; then
  scripts/companion-release/verify-rotation-ref-ruleset.sh --public ||
    fail 'public immutable rotation ref authority ruleset is unavailable'
else
  scripts/companion-release/verify-rotation-ref-ruleset.sh ||
    fail 'immutable rotation ref authority ruleset is unavailable'
fi

remote_line=$(git ls-remote --refs origin "$rotation_ref") || fail 'cannot inspect rotation distribution ref'
rotation_ref_commit=${remote_line%%$'\t'*}
[[ "$rotation_ref_commit" =~ ^[0-9a-f]{40}$ &&
   "$remote_line" == "$rotation_ref_commit"$'\t'"$rotation_ref" ]] ||
  fail 'rotation distribution ref is missing or ambiguous'
git fetch --no-tags origin "$rotation_ref" >/dev/null || fail 'cannot fetch rotation distribution ref'
[[ "$(git rev-parse --verify FETCH_HEAD)" == "$rotation_ref_commit" &&
   "$(git cat-file -t "$rotation_ref_commit")" == 'commit' ]] ||
  fail 'fetched rotation distribution commit differs'
[[ "$(git rev-list --parents -n 1 "$rotation_ref_commit")" == "$rotation_ref_commit" ]] ||
  fail 'rotation distribution commit is not orphaned'
entries=$(git ls-tree -r "$rotation_ref_commit") || fail 'cannot inspect rotation distribution tree'
names=$(git ls-tree -r --name-only "$rotation_ref_commit") || fail 'cannot inspect rotation distribution names'
[[ "$names" == "$document_name"$'\n'"$signature_name" ]] ||
  fail 'rotation distribution must contain exactly the canonical sidecar pair'
while IFS= read -r entry; do
  [[ "${entry%% *}" == '100644' ]] || fail 'rotation distribution file mode is unsafe'
done <<<"$entries"
install -d -m 0700 "$output_dir"
document="$output_dir/$document_name"
signature="$output_dir/$signature_name"
git cat-file blob "${rotation_ref_commit}:${document_name}" >"$document" ||
  fail 'cannot materialize rotation document'
git cat-file blob "${rotation_ref_commit}:${signature_name}" >"$signature" ||
  fail 'cannot materialize rotation signature'
chmod 0600 "$document" "$signature"
[[ -s "$document" && "$(wc -c <"$signature" | tr -d ' ')" == '64' ]] ||
  fail 'rotation sidecar bytes are malformed'
if [[ -n "$supplied_document" ]]; then
  cmp -s "$supplied_document" "$document" || fail 'supplied rotation document differs from fixed ref'
  cmp -s "$supplied_signature" "$signature" || fail 'supplied rotation signature differs from fixed ref'
fi
verified_document="$output_dir/verified-document"
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  AUTOPUS_ADK_CHANNEL_KEY_ID="$pinned_channel_key_id" \
  AUTOPUS_ADK_CHANNEL_PUBLIC_KEY="$pinned_channel_public_key" \
  "$verifier" "$verify_command" \
  --document "$document" --signature "$signature" \
  --source-commit "$source_commit" --source-tree "$source_tree" \
  --next-tag-public-key scripts/companion-release/release-tag-signing-2026-q3-r2.pub \
  --next-tag-fingerprint scripts/companion-release/release-tag-signing-2026-q3-r2.fingerprint \
  --next-promotion-public-key scripts/companion-release/omp-context-promotion-2026-q3-k3.pub \
  >"$verified_document" || fail 'ADK channel authority rejected the rotation sidecar'
cmp -s "$document" "$verified_document" || fail 'rotation verifier did not return exact canonical bytes'
rm -f -- "$verified_document"
final_remote=$(git ls-remote --refs origin "$rotation_ref")
[[ "$final_remote" == "$rotation_ref_commit"$'\t'"$rotation_ref" ]] ||
  fail 'rotation distribution ref changed during verification'
rotation_document_sha256=$(shasum -a 256 "$document" | awk '{print $1}')
[[ "$rotation_document_sha256" =~ ^[0-9a-f]{64}$ ]] || fail 'rotation document digest is malformed'
assertion_mode='strict'
[[ "$public_ruleset" -eq 0 ]] || assertion_mode='public'
readonly assertion_mode
jq -cn --arg assertion_mode "$assertion_mode" --arg rotation_ref "$rotation_ref" \
  --arg rotation_ref_commit "$rotation_ref_commit" \
  --arg rotation_document_sha256 "$rotation_document_sha256" \
  '{assertion_mode:$assertion_mode,rotation_ref:$rotation_ref,rotation_ref_commit:$rotation_ref_commit,
    rotation_document_sha256:$rotation_document_sha256}'
