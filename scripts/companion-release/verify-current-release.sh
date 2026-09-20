#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'current release evidence: %s\n' "$1" >&2; exit 1; }
readonly RELEASE_REPOSITORY='autopus-ai/autopus-adk'
readonly RELEASE_VERSION='0.50.118'
readonly RELEASE_TAG='v0.50.118'
readonly REPORT_NAME='omp-context-promotion-report.v1.json'
readonly ATTESTATION_NAME='omp-context-promotion-attestation.v2.json'
readonly LINEAGE_NAME='release-lineage-v1.json'
readonly LINEAGE_SIGNATURE_NAME='release-lineage-v1.sig'
readonly ARM64_ARCHIVE_NAME="autopus-adk_${RELEASE_VERSION}_darwin_arm64.tar.gz"
EXPECTED_ARCHIVES=(
  "autopus-adk_${RELEASE_VERSION}_darwin_amd64.tar.gz"
  "autopus-adk_${RELEASE_VERSION}_darwin_arm64.tar.gz"
  "autopus-adk_${RELEASE_VERSION}_linux_amd64.tar.gz"
  "autopus-adk_${RELEASE_VERSION}_linux_arm64.tar.gz"
  "autopus-adk_${RELEASE_VERSION}_windows_amd64.tar.gz"
  "autopus-adk_${RELEASE_VERSION}_windows_amd64.zip"
  "autopus-adk_${RELEASE_VERSION}_windows_arm64.tar.gz"
  "autopus-adk_${RELEASE_VERSION}_windows_arm64.zip"
)
readonly EXPECTED_ARCHIVES
EXPECTED_ASSETS=(
  "${EXPECTED_ARCHIVES[@]}"
  'checksums.txt'
  'checksums.txt.bundle'
  'checksums.txt.signatures'
  "$REPORT_NAME"
  "$ATTESTATION_NAME"
  "$LINEAGE_NAME"
  "$LINEAGE_SIGNATURE_NAME"
)
readonly EXPECTED_ASSETS

[[ $# -eq 1 ]] || fail 'usage: verify-current-release.sh CHECKSUMS_OUTPUT'
readonly checksums_output=$1
[[ -n "${GITHUB_TOKEN:-}" ]] || fail 'missing GITHUB_TOKEN'
[[ "${COMPANION_RELEASE_ID:-}" =~ ^[1-9][0-9]*$ ]] ||
  fail 'exact release id is missing or malformed'
for name in COMPANION_SOURCE_COMMIT COMPANION_SOURCE_TREE; do
  [[ "${!name:-}" =~ ^[0-9a-f]{40}$ ]] || fail "${name} is missing or malformed"
done
[[ "${OMP_CONTEXT_EVIDENCE_REPORT_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] ||
  fail 'exact OMP report digest is missing or malformed'
[[ "${OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] ||
  fail 'exact OMP attestation digest is missing or malformed'
[[ "${OMP_CONTEXT_STATIC_POLICY_B64:-}" =~ ^[A-Za-z0-9_-]+$ &&
   "${#OMP_CONTEXT_STATIC_POLICY_B64}" -le 21846 ]] ||
  fail 'canonical OMP static policy is missing or malformed'
for name in OMP_CONTEXT_EVIDENCE_VERIFIER OMP_CONTEXT_LINEAGE_VERIFIER \
  COMPANION_MANIFEST_VERIFIER
do
  [[ -f "${!name:-}" && ! -L "${!name}" && -x "${!name}" ]] ||
    fail "${name} is missing or unsafe"
done
[[ "${COMPANION_KEY_ID:-}" =~ ^[A-Za-z0-9][A-Za-z0-9._:@/+\-]{0,255}$ &&
   "${COMPANION_HANDOFF:-}" == 'v1' &&
   "${COMPANION_ROLLBACK_FLOOR:-}" =~ ^[1-9][0-9]*$ &&
   "${COMPANION_PUBLIC_KEY_SHA256:-}" =~ ^sha256:[0-9a-f]{64}$ ]] ||
  fail 'current companion release-key policy is missing or malformed'
for tool in awk cmp gh grep install jq mktemp shasum tar tr wc; do
  command -v "$tool" >/dev/null || fail "$tool is unavailable"
done
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) ||
  fail 'cannot resolve verifier directory'
signature_helper="$script_dir/verify-current-release-signatures.sh"
[[ -f "$signature_helper" && ! -L "$signature_helper" && -x "$signature_helper" ]] ||
  fail 'current release signature helper is missing or unsafe'
output_dir=$(dirname -- "$checksums_output")
[[ -d "$output_dir" && ! -L "$output_dir" ]] || fail 'checksums output parent is unsafe'
[[ ! -e "$checksums_output" && ! -L "$checksums_output" ]] ||
  fail 'checksums output already exists'

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/adk-current-release.XXXXXX") ||
  fail 'cannot create release evidence directory'
cleanup() {
  local status=$?
  rm -rf -- "$temp_dir" || status=$?
  if ((status != 0)); then rm -f -- "$checksums_output" || true; fi
  return "$status"
}
trap cleanup EXIT
release_json="$temp_dir/release.json"
checksums="$temp_dir/checksums.txt"
bundle="$temp_dir/checksums.txt.bundle"
envelope="$temp_dir/checksums.txt.signatures"
report="$temp_dir/$REPORT_NAME"
attestation="$temp_dir/$ATTESTATION_NAME"
lineage="$temp_dir/$LINEAGE_NAME"
lineage_signature="$temp_dir/$LINEAGE_SIGNATURE_NAME"
arm64_archive="$temp_dir/$ARM64_ARCHIVE_NAME"

GH_TOKEN="$GITHUB_TOKEN" gh api -H 'Accept: application/vnd.github+json' \
  "repos/${RELEASE_REPOSITORY}/releases/tags/${RELEASE_TAG}" >"$release_json" ||
  fail 'cannot read exact A29 release'
[[ -s "$release_json" && ! -L "$release_json" ]] ||
  fail 'A29 release metadata is empty or unsafe'
expected_assets_json=$(printf '%s\n' "${EXPECTED_ASSETS[@]}" |
  jq -Rsc 'split("\n") | map(select(length > 0))') ||
  fail 'cannot construct expected asset set'
jq -e --arg id "$COMPANION_RELEASE_ID" --arg tag "$RELEASE_TAG" \
  --arg commit "$COMPANION_SOURCE_COMMIT" --argjson expected "$expected_assets_json" '
  type == "object" and .id == ($id | tonumber) and .tag_name == $tag and
  .target_commitish == $commit and .author.id == 204883817 and
  .draft == false and .prerelease == false and .immutable == true and
  (.assets | type) == "array" and (.assets | length) == ($expected | length) and
  ([.assets[].name] | sort) == ($expected | sort) and
  ([.assets[].name] | unique | length) == ($expected | length) and
  all(.assets[]; (.id | type) == "number" and .id > 0 and .state == "uploaded" and
    (.size | type) == "number" and .size > 0 and
    (.digest | type) == "string" and (.digest | test("^sha256:[0-9a-f]{64}$")))
' "$release_json" >/dev/null ||
  fail 'A29 release id, tag, source, author, state, assets, or digests differ'

download_release_asset() {
  local asset_name=$1 destination=$2 metadata asset_id asset_size api_digest
  local downloaded_size downloaded_digest
  metadata=$(jq -er --arg name "$asset_name" '
    .assets[] | select(.name == $name) | [.id, .size, .digest] | @tsv
  ' "$release_json") || fail "${asset_name} metadata is unavailable"
  IFS=$'\t' read -r asset_id asset_size api_digest <<<"$metadata"
  [[ "$asset_id" =~ ^[1-9][0-9]*$ && "$asset_size" =~ ^[1-9][0-9]*$ ]] ||
    fail "${asset_name} identifier or size is malformed"
  [[ ! -e "$destination" && ! -L "$destination" ]] ||
    fail "${asset_name} destination already exists"
  GH_TOKEN="$GITHUB_TOKEN" gh api -H 'Accept: application/octet-stream' \
    "repos/${RELEASE_REPOSITORY}/releases/assets/${asset_id}" >"$destination" ||
    fail "cannot download ${asset_name} from exact A29 release"
  [[ -s "$destination" && ! -L "$destination" ]] ||
    fail "downloaded ${asset_name} is empty or unsafe"
  downloaded_size=$(wc -c <"$destination" | tr -d '[:space:]')
  [[ "$downloaded_size" == "$asset_size" ]] ||
    fail "downloaded ${asset_name} size differs from GitHub metadata"
  downloaded_digest=$(shasum -a 256 "$destination" | awk '{print $1}')
  [[ "sha256:$downloaded_digest" == "$api_digest" ]] ||
    fail "downloaded ${asset_name} differs from GitHub metadata"
}

download_release_asset 'checksums.txt' "$checksums"
download_release_asset 'checksums.txt.bundle' "$bundle"
download_release_asset 'checksums.txt.signatures' "$envelope"
download_release_asset "$REPORT_NAME" "$report"
download_release_asset "$ATTESTATION_NAME" "$attestation"
download_release_asset "$LINEAGE_NAME" "$lineage"
download_release_asset "$LINEAGE_SIGNATURE_NAME" "$lineage_signature"
download_release_asset "$ARM64_ARCHIVE_NAME" "$arm64_archive"
[[ "$(wc -c <"$lineage_signature" | tr -d '[:space:]')" == '64' ]] ||
  fail 'released lineage signature is not raw Ed25519 bytes'
[[ "$(shasum -a 256 "$report" | awk '{print $1}')" == "$OMP_CONTEXT_EVIDENCE_REPORT_SHA256" ]] ||
  fail 'released OMP report digest differs'
[[ "$(shasum -a 256 "$attestation" | awk '{print $1}')" == "$OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256" ]] ||
  fail 'released OMP attestation digest differs'

archive_listing="$temp_dir/arm64-archive-entries"
tar -tzf "$arm64_archive" >"$archive_listing" || fail 'Darwin arm64 archive cannot be listed'
if grep -Eq '(^/|(^|/)\.\.(/|$)|//)' "$archive_listing"; then fail 'archive contains unsafe path'; fi
bundle_dir="$temp_dir/adk-companion-public-key-receipt.bundle"
install -m 0700 -d "$bundle_dir"
extract_archive_entry() {
  local name=$1 output=$2
  [[ "$(grep -Fxc "$name" "$archive_listing")" == '1' ]] ||
    fail "archive entry is absent or duplicate: ${name}"
  tar -xOzf "$arm64_archive" "$name" >"$output" || fail "cannot extract ${name}"
  chmod 0600 "$output"
}
archive_auto="$temp_dir/auto"
archive_manifest="$temp_dir/adk-companion-manifest.json"
archive_manifest_signature="$temp_dir/adk-companion-manifest.sig"
archive_lineage="$temp_dir/archive-$LINEAGE_NAME"
archive_lineage_signature="$temp_dir/archive-$LINEAGE_SIGNATURE_NAME"
extract_archive_entry auto "$archive_auto"
extract_archive_entry adk-companion-manifest.json "$archive_manifest"
extract_archive_entry adk-companion-manifest.sig "$archive_manifest_signature"
extract_archive_entry adk-companion-public-key-receipt.bundle/public-key-receipt.json \
  "$bundle_dir/public-key-receipt.json"
extract_archive_entry adk-companion-public-key-receipt.bundle/public-key-receipt.sig \
  "$bundle_dir/public-key-receipt.sig"
extract_archive_entry "$LINEAGE_NAME" "$archive_lineage"
extract_archive_entry "$LINEAGE_SIGNATURE_NAME" "$archive_lineage_signature"
cmp -s "$lineage" "$archive_lineage" || fail 'standalone and archived lineage bytes differ'
cmp -s "$lineage_signature" "$archive_lineage_signature" ||
  fail 'standalone and archived lineage signatures differ'
chmod 0700 "$archive_auto"
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  "$COMPANION_MANIFEST_VERIFIER" --artifact "$archive_auto" \
  --manifest "$archive_manifest" --signature "$archive_manifest_signature" \
  --receipt "$bundle_dir/public-key-receipt.json" \
  --receipt-signature "$bundle_dir/public-key-receipt.sig" \
  --key-id "$COMPANION_KEY_ID" --version "$RELEASE_VERSION" \
  --platform darwin --architecture arm64 --handoff "$COMPANION_HANDOFF" \
  --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR" \
  --public-key-sha256 "$COMPANION_PUBLIC_KEY_SHA256" ||
  fail 'released Darwin arm64 companion authority is invalid'

expected_archives_json=$(printf '%s\n' "${EXPECTED_ARCHIVES[@]}" |
  jq -Rsc 'split("\n") | map(select(length > 0))')
checksum_entries_json=$(jq -Rsc '
  if endswith("\n") then .[0:-1] else error("missing final newline") end |
  split("\n") | map(capture("^(?<digest>[0-9a-f]{64})  (?<name>[A-Za-z0-9._-]+)$"))
' "$checksums") || fail 'checksums.txt contains malformed lines'
printf '%s' "$checksum_entries_json" | jq -e --argjson expected "$expected_archives_json" '
  length == ($expected | length) and ([.[].name] | sort) == ($expected | sort) and
  ([.[].name] | unique | length) == ($expected | length)
' >/dev/null || fail 'checksums.txt does not describe exactly eight A29 archives'
for archive in "${EXPECTED_ARCHIVES[@]}"; do
  api_digest=$(jq -er --arg name "$archive" '.assets[] | select(.name == $name) | .digest' \
    "$release_json") || fail "API digest is unavailable for ${archive}"
  checksum_digest=$(printf '%s' "$checksum_entries_json" |
    jq -er --arg name "$archive" '.[] | select(.name == $name) | .digest') ||
    fail "checksum entry is unavailable for ${archive}"
  [[ "$api_digest" == "sha256:$checksum_digest" ]] ||
    fail "checksums.txt differs from GitHub API digest for ${archive}"
done
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  "$signature_helper" "$checksums" "$bundle" "$envelope" ||
  fail 'A29 checksum ECDSA/cosign evidence is invalid'

candidate_artifact_sha=$(jq -er '.candidate.artifact_sha256 |
  select(type == "string" and test("^sha256:[0-9a-f]{64}$")) | ltrimstr("sha256:")' "$report") ||
  fail 'released OMP candidate artifact digest is malformed'
distributed_artifact_sha=$(jq -er '.artifact_digest |
  select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$archive_manifest") ||
  fail 'released Darwin arm64 manifest digest is malformed'
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  "$OMP_CONTEXT_LINEAGE_VERIFIER" --lineage "$lineage" --signature "$lineage_signature" \
  --receipt-bundle "$bundle_dir" --key-id "$COMPANION_KEY_ID" \
  --handoff "$COMPANION_HANDOFF" --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR" \
  --upstream-sha256 "sha256:$candidate_artifact_sha" \
  --executable-sha256 "$distributed_artifact_sha" \
  --source-repository "$RELEASE_REPOSITORY" --source-commit "$COMPANION_SOURCE_COMMIT" \
  --source-tree "$COMPANION_SOURCE_TREE" --target darwin-arm64 --version "$RELEASE_VERSION" ||
  fail 'released A29 U-to-D lineage is invalid'

# Active freshness is enforced before publication. This shared immutable gate
# uses historical proof so Homebrew recovery remains valid after the active TTL.
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  "$OMP_CONTEXT_EVIDENCE_VERIFIER" --mode historical --report "$report" \
  --attestation "$attestation" --report-sha256 "$OMP_CONTEXT_EVIDENCE_REPORT_SHA256" \
  --attestation-sha256 "$OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256" \
  --candidate-repository "$RELEASE_REPOSITORY" \
  --candidate-revision "$COMPANION_SOURCE_COMMIT" --candidate-tree "$COMPANION_SOURCE_TREE" \
  --candidate-artifact-sha256 "$candidate_artifact_sha" \
  --static-policy-b64 "$OMP_CONTEXT_STATIC_POLICY_B64" ||
  fail 'A29 historical-recovery OMP evidence is invalid'
install -m 0600 "$checksums" "$checksums_output" || fail 'cannot materialize checksums.txt'
cmp -s "$checksums" "$checksums_output" || fail 'materialized checksums differ'
printf 'current release evidence: exactly fifteen A29 normal release assets verified\n'
