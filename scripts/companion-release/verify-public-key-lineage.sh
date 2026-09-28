#!/usr/bin/env bash
set -euo pipefail
umask 077
readonly BUNDLE_NAME='adk-companion-public-key-receipt.bundle' RECEIPT_NAME='public-key-receipt.json' SIGNATURE_NAME='public-key-receipt.sig'
readonly MANIFEST_NAME='adk-companion-manifest.json' MANIFEST_SIGNATURE_NAME='adk-companion-manifest.sig' ARTIFACT_NAME='auto' CHECKSUMS_NAME='checksums.txt'
readonly LOCAL_EVIDENCE_ERROR='fixture_or_local_evidence_forbidden'
fail() { printf 'companion release lineage: %s: %s\n' "$1" "$2" >&2; exit 1; }
require_environment() { local name="$1"; [[ -n "${!name-}" ]] || fail prior_evidence_unverifiable "missing ${name}"; }
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) || fail prior_evidence_unverifiable 'cannot resolve lineage verifier directory'
pins_helper="$script_dir/verify-public-key-lineage-pins.sh"
[[ -f "$pins_helper" && ! -L "$pins_helper" ]] || fail prior_evidence_unverifiable 'lineage pin source is invalid'
# shellcheck source=verify-public-key-lineage-pins.sh
source "$pins_helper"
coordinates_helper="$script_dir/verify-public-key-lineage-coordinates.sh"
[[ -f "$coordinates_helper" && ! -L "$coordinates_helper" ]] || fail prior_evidence_unverifiable 'lineage coordinate source is invalid'
# shellcheck source=verify-public-key-lineage-coordinates.sh
source "$coordinates_helper"
archive_helper="$script_dir/verify-public-key-lineage-archive.sh"
[[ -f "$archive_helper" && ! -L "$archive_helper" ]] || fail prior_evidence_unverifiable 'lineage archive verifier is invalid'
# shellcheck source=verify-public-key-lineage-archive.sh
source "$archive_helper"
assets_helper="$script_dir/verify-public-key-lineage-assets.sh"
[[ -f "$assets_helper" && ! -L "$assets_helper" ]] || fail prior_evidence_unverifiable 'lineage asset verifier is invalid'
# shellcheck source=verify-public-key-lineage-assets.sh
source "$assets_helper"
nonzero_hex "$prior_commit" 40 || fail prior_evidence_unverifiable "${prior_phase} commit pin is not provisioned; ${LOCAL_EVIDENCE_ERROR}"
if [[ "$release_phase" == 'A23' ]]; then
  for pin in "$A0_RECEIPT_SHA256" "$A0_SIGNATURE_SHA256" "$A0_RECORD_SHA256" \
    "$A0_PUBLIC_KEY_SHA256" "$prior_checksums" "$prior_arm64_archive" \
    "$prior_arm64_manifest" "$A22_ARM64_MANIFEST_SIGNATURE_SHA256" \
    "$A22_CHECKSUMS_BUNDLE_SHA256" "$A22_CHECKSUMS_SIGNATURES_SHA256" \
    "$A22_BRIDGE_MANIFEST_SHA256" "$A22_LINEAGE_SHA256" \
    "$A22_LINEAGE_SIGNATURE_SHA256" "$A22_UPSTREAM_SHA256" "$A22_EXECUTABLE_SHA256"; do
    nonzero_hex "$pin" 64 || fail prior_evidence_unverifiable 'A22 bridge trust pins are not provisioned'
  done
else
  for pin in "$A0_RECEIPT_SHA256" "$A0_SIGNATURE_SHA256" "$A0_RECORD_SHA256" \
    "$A0_PUBLIC_KEY_SHA256" "$prior_checksums" "$prior_amd64_manifest" "$prior_arm64_manifest"; do
    nonzero_hex "$pin" 64 || fail prior_evidence_unverifiable 'prior release trust pins are not provisioned'
  done
fi
if [[ "$release_phase" == 'A23' ]]; then
  nonzero_hex "$prior_tag_object" 40 \
    || fail prior_evidence_unverifiable 'A22 annotated tag pin is not provisioned'
elif [[ "$release_phase" == 'A2' || "$release_phase" == 'A3' || "$release_phase" == 'A4' || "$release_phase" == 'A5' || "$release_phase" == 'A6' || "$release_phase" == 'A7' || "$release_phase" == 'A8' || "$release_phase" == 'A9' || "$release_phase" == 'A10' || "$release_phase" == 'A11' || "$release_phase" == 'A12' || "$release_phase" == 'A13' || "$release_phase" == 'A14' || "$release_phase" == 'A15' || "$release_phase" == 'A16' || "$release_phase" == 'A17' || "$release_phase" == 'A18' || "$release_phase" == 'A19' || "$release_phase" == 'A20' || "$release_phase" == 'A21' || "$release_phase" == 'A22' || "$release_phase" == 'A24' || "$release_phase" == 'A25' || "$release_phase" == 'A26' || "$release_phase" == 'A27' || "$release_phase" == 'A28' || "$release_phase" == 'A29' || "$release_phase" == 'A30' ]]; then
  nonzero_hex "$prior_tag_object" 40 || fail prior_evidence_unverifiable "${prior_phase} annotated tag pin is not provisioned"
  for pin in "$prior_amd64_archive" "$prior_arm64_archive"; do
    nonzero_hex "$pin" 64 || fail prior_evidence_unverifiable "${prior_phase} archive pins are not provisioned"
  done
fi
if [[ "$release_phase" == 'A15' || "$release_phase" == 'A16' || "$release_phase" == 'A17' || "$release_phase" == 'A18' || "$release_phase" == 'A19' || "$release_phase" == 'A20' || "$release_phase" == 'A21' || "$release_phase" == 'A22' || "$release_phase" == 'A24' || "$release_phase" == 'A25' || "$release_phase" == 'A26' || "$release_phase" == 'A27' || "$release_phase" == 'A28' || "$release_phase" == 'A29' || "$release_phase" == 'A30' ]]; then
  for pin in "$prior_linux_amd64_archive" "$prior_linux_arm64_archive"; do
    nonzero_hex "$pin" 64 \
      || fail prior_evidence_unverifiable "${prior_phase} Linux archive pins are not provisioned"
  done
elif [[ -n "$prior_linux_amd64_archive" || -n "$prior_linux_arm64_archive" ]]; then
  fail prior_evidence_unverifiable 'historical Linux archive pins must remain empty'
fi
if [[ -n "$prior_tree" ]]; then
  nonzero_hex "$prior_tree" 40 \
    || fail prior_evidence_unverifiable "${prior_phase} source tree pin is not provisioned"
fi
if [[ -n "$prior_release_id" ]]; then
  [[ "$prior_release_id" =~ ^[1-9][0-9]*$ ]] \
    || fail prior_evidence_unverifiable "${prior_phase} release ID pin is not provisioned"
fi
for name in GITHUB_TOKEN COMPANION_SIGNER COMPANION_RECEIPT_VERIFIER \
  COMPANION_SIGNING_KEY_FILE COMPANION_KEY_ID COMPANION_HANDOFF COMPANION_ROLLBACK_FLOOR \
  COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT; do
  require_environment "$name"
done
case "${COMPANION_LINEAGE_MANIFEST_VERIFICATION_REQUIRED-0}" in
  0) ;;
  1) require_environment COMPANION_MANIFEST_VERIFIER ;;
  *) fail prior_evidence_unverifiable 'COMPANION_LINEAGE_MANIFEST_VERIFICATION_REQUIRED must be 0 or 1' ;;
esac
for tool in gh jq tar cmp shasum xxd awk grep find wc; do
  command -v "$tool" >/dev/null || fail prior_evidence_unverifiable "required tool ${tool} is unavailable"
done
[[ -f "$COMPANION_SIGNER" && ! -L "$COMPANION_SIGNER" && -x "$COMPANION_SIGNER" ]] || fail prior_evidence_unverifiable 'companion signer is invalid'
[[ -f "$COMPANION_RECEIPT_VERIFIER" && ! -L "$COMPANION_RECEIPT_VERIFIER" &&
   -x "$COMPANION_RECEIPT_VERIFIER" ]] \
  || fail prior_evidence_unverifiable 'companion receipt verifier is invalid'
if [[ -n "${COMPANION_MANIFEST_VERIFIER-}" ]]; then
  [[ -f "$COMPANION_MANIFEST_VERIFIER" && ! -L "$COMPANION_MANIFEST_VERIFIER" &&
     -x "$COMPANION_MANIFEST_VERIFIER" ]] \
    || fail prior_evidence_unverifiable 'companion manifest verifier is invalid'
fi
if [[ "$release_phase" == 'A23' || "$release_phase" == 'A24' || "$release_phase" == 'A25' || "$release_phase" == 'A26' || "$release_phase" == 'A27' || "$release_phase" == 'A28' || "$release_phase" == 'A29' || "$release_phase" == 'A30' ]]; then
  [[ -f "${OMP_CONTEXT_LINEAGE_VERIFIER-}" &&
     ! -L "$OMP_CONTEXT_LINEAGE_VERIFIER" && -x "$OMP_CONTEXT_LINEAGE_VERIFIER" ]] \
    || fail prior_evidence_unverifiable 'OMP context lineage verifier is invalid'
fi
[[ -f "$COMPANION_SIGNING_KEY_FILE" && ! -L "$COMPANION_SIGNING_KEY_FILE" ]] \
  || fail prior_evidence_unverifiable 'companion signing key file is invalid'
temp_dir=''
cleanup() {
  local status=$? cleanup_status=0
  [[ -z "$temp_dir" ]] || rm -rf -- "$temp_dir" || cleanup_status=$?
  if [[ "$cleanup_status" != '0' ]]; then
    printf 'companion release lineage: cleanup failed\n' >&2
    return "$cleanup_status"
  fi
  return "$status"
}
trap cleanup EXIT; trap 'exit 1' HUP INT TERM
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/adk-public-key-lineage.XXXXXX") \
  || fail prior_evidence_unverifiable 'cannot allocate verification workspace'
release_json="$temp_dir/prior-release.json" tag_ref_json="$temp_dir/prior-tag-ref.json" commit_json="$temp_dir/prior-commit.json"
env -i PATH="$PATH" HOME="${HOME-}" GITHUB_TOKEN="$GITHUB_TOKEN" \
  gh api "repos/$prior_repository/releases/tags/$prior_tag" >"$release_json" \
  || fail prior_evidence_absent "cannot obtain ${prior_evidence_source} metadata"
env -i PATH="$PATH" HOME="${HOME-}" GITHUB_TOKEN="$GITHUB_TOKEN" \
  gh api "repos/$prior_repository/git/ref/tags/$prior_tag" >"$tag_ref_json" \
  || fail prior_evidence_absent "cannot obtain immutable ${prior_phase} tag metadata"
env -i PATH="$PATH" HOME="${HOME-}" GITHUB_TOKEN="$GITHUB_TOKEN" \
  gh api "repos/$prior_repository/commits/$prior_commit" >"$commit_json" \
  || fail prior_evidence_absent "cannot obtain immutable ${prior_phase} commit metadata"
tag_name=$(jq -er '.tag_name | select(type == "string")' "$release_json") \
  || fail prior_evidence_malformed 'release tag_name is malformed'
target_commitish=$(jq -er '.target_commitish | select(type == "string")' "$release_json") \
  || fail prior_evidence_malformed 'release target_commitish is malformed'
jq -e '.draft == false and .prerelease == false and .immutable == true' \
  "$release_json" >/dev/null \
  || fail prior_evidence_unverifiable "${prior_phase} release is not immutable and final"
if [[ -n "$prior_release_id" ]]; then
  [[ "$(jq -er '.id' "$release_json")" == "$prior_release_id" ]] \
    || fail prior_release_identity_mismatch "${prior_phase} release ID differs from its pin"
fi
[[ "$tag_name" == "$prior_tag" && "$target_commitish" == "$prior_commit" ]] \
  || fail prior_release_identity_mismatch 'release/tag/commit coordinates differ'
[[ "$(jq -er '.sha' "$commit_json")" == "$prior_commit" ]] \
  || fail prior_release_identity_mismatch "commit endpoint differs from the ${prior_phase} pin"
if [[ -n "$prior_tree" ]]; then
  [[ "$(jq -er '.commit.tree.sha' "$commit_json")" == "$prior_tree" ]] \
    || fail prior_release_identity_mismatch "${prior_phase} source tree differs from its pin"
fi
tag_type=$(jq -er '.object.type' "$tag_ref_json") \
  || fail prior_evidence_malformed 'tag object type is malformed'
tag_object_sha=$(jq -er '.object.sha' "$tag_ref_json") \
  || fail prior_evidence_malformed 'tag object SHA is malformed'
if [[ "$tag_type" == 'tag' ]]; then
  [[ -z "$prior_tag_object" || "$tag_object_sha" == "$prior_tag_object" ]] \
    || fail prior_release_identity_mismatch "${prior_phase} annotated tag object differs from its pin"
  annotated_tag_json="$temp_dir/prior-annotated-tag.json"
  env -i PATH="$PATH" HOME="${HOME-}" GITHUB_TOKEN="$GITHUB_TOKEN" \
    gh api "repos/$prior_repository/git/tags/$tag_object_sha" >"$annotated_tag_json" \
    || fail prior_evidence_absent "cannot obtain annotated ${prior_phase} tag metadata"
  [[ "$(jq -er '.tag' "$annotated_tag_json")" == "$prior_tag" && \
     "$(jq -er '.object.type' "$annotated_tag_json")" == 'commit' ]] \
    || fail prior_release_identity_mismatch 'annotated tag identity differs'
  tag_commit_sha=$(jq -er '.object.sha' "$annotated_tag_json") \
    || fail prior_evidence_malformed 'annotated tag commit is malformed'
elif [[ "$tag_type" == 'commit' ]]; then
  [[ -z "$prior_tag_object" ]] \
    || fail prior_release_identity_mismatch "${prior_phase} tag must be the pinned annotated object"
  tag_commit_sha="$tag_object_sha"
else
  fail prior_evidence_malformed "${prior_phase} tag does not resolve to a commit"
fi
[[ "$tag_commit_sha" == "$prior_commit" ]] \
  || fail prior_release_identity_mismatch "${prior_phase} tag commit differs from the source pin"
jq -e --arg bundle "$BUNDLE_NAME" --arg receipt "$RECEIPT_NAME" --arg signature "$SIGNATURE_NAME" \
  '[.assets[].name | select(. == $bundle or . == $receipt or . == $signature)] | length == 0' \
  "$release_json" >/dev/null \
  || fail prior_evidence_malformed 'independent receipt assets are forbidden'
if [[ "$release_phase" == 'A23' ]]; then
  verify_a22_bridge_lineage_assets
else
  verify_public_key_lineage_assets
fi
receipt_sha256=$(sha256_file "$prior_receipt") \
  || fail prior_evidence_unverifiable 'cannot digest prior receipt'
signature_sha256=$(sha256_file "$prior_signature") \
  || fail prior_evidence_unverifiable 'cannot digest prior signature'
[[ "$receipt_sha256" == "sha256:$A0_RECEIPT_SHA256" ]] \
  || fail prior_receipt_bytes_mismatch 'prior receipt differs from its A0 pin'
[[ "$signature_sha256" == "sha256:$A0_SIGNATURE_SHA256" ]] \
  || fail prior_signature_bytes_mismatch 'prior signature differs from its A0 pin'
claimed_public_key_sha256=$(jq -er '.public_key_sha256' "$prior_receipt") \
  || fail prior_evidence_malformed 'prior public_key_sha256 claim is absent'
[[ "$claimed_public_key_sha256" == "sha256:$A0_PUBLIC_KEY_SHA256" ]] \
  || fail prior_public_key_digest_mismatch 'prior public key digest differs'
env -i PATH="$PATH" HOME="${HOME-}" \
  "$COMPANION_RECEIPT_VERIFIER" \
  --receipt "$prior_receipt" --signature "$prior_signature" \
  --signing-key "$COMPANION_SIGNING_KEY_FILE" \
  --key-id "$COMPANION_KEY_ID" \
  --issued-at "$COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT" \
  --expires-at "$COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT" \
  --handoff "$COMPANION_HANDOFF" \
  --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR" \
  --public-key-sha256 "sha256:$A0_PUBLIC_KEY_SHA256" \
  || fail prior_evidence_unverifiable 'prior receipt claims or signature are invalid'
record_sha256=$(
  {
    printf 'autopus.public-key-receipt.a0-record.v1\0'
    printf '%s' "${receipt_sha256#sha256:}" | xxd -r -p
    printf '%s' "${signature_sha256#sha256:}" | xxd -r -p
  } | shasum -a 256 | awk '{print "sha256:" $1}'
) || fail prior_evidence_unverifiable 'cannot digest prior key record'
[[ "$record_sha256" == "sha256:$A0_RECORD_SHA256" ]] \
  || fail prior_record_digest_mismatch 'prior key record differs from its A0 pin'
current_bundle="$temp_dir/current/$BUNDLE_NAME"
install -m 0700 -d "$temp_dir/current"
env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="$temp_dir" \
  "$COMPANION_SIGNER" companion-manifest public-key-receipt \
  --key-file "$COMPANION_SIGNING_KEY_FILE" \
  --bundle-output "$current_bundle" \
  --key-id "$COMPANION_KEY_ID" \
  --issued-at "$COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT" \
  --expires-at "$COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT" \
  --handoff "$COMPANION_HANDOFF" \
  --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR" >/dev/null \
  || fail prior_evidence_unverifiable "cannot produce current ${release_phase} receipt"
[[ "$(find "$current_bundle" -mindepth 1 -maxdepth 1 -print | wc -l | tr -d '[:space:]')" == '2' ]] \
  || fail prior_evidence_malformed "current ${release_phase} bundle is malformed"
current_receipt="$current_bundle/$RECEIPT_NAME"
current_signature="$current_bundle/$SIGNATURE_NAME"
cmp -- "$prior_receipt" "$current_receipt" \
  || fail prior_receipt_bytes_mismatch "${release_phase} does not republish exact ${prior_phase} receipt bytes"
cmp -- "$prior_signature" "$current_signature" \
  || fail prior_signature_bytes_mismatch "${release_phase} does not republish exact ${prior_phase} signature bytes"
current_receipt_sha256=$(sha256_file "$current_receipt")
current_signature_sha256=$(sha256_file "$current_signature")
current_record_sha256=$({
  printf 'autopus.public-key-receipt.a0-record.v1\0'
  printf '%s' "${current_receipt_sha256#sha256:}" | xxd -r -p
  printf '%s' "${current_signature_sha256#sha256:}" | xxd -r -p
} | shasum -a 256 | awk '{print "sha256:" $1}') \
  || fail prior_evidence_unverifiable 'cannot digest current key record'
[[ "$current_record_sha256" == "sha256:$A0_RECORD_SHA256" ]] \
  || fail prior_record_digest_mismatch 'current key record differs from its A0 pin'
[[ "$(jq -er '.public_key_sha256' "$current_receipt")" == "sha256:$A0_PUBLIC_KEY_SHA256" ]] \
  || fail prior_public_key_digest_mismatch 'current public key digest differs from its A0 pin'
printf 'companion release lineage: %s exact %s key record verified\n' "$release_phase" "$prior_phase"
