#!/usr/bin/env bash
# shellcheck disable=SC1091,SC2154
set -euo pipefail
umask 077
fail() {
  printf 'companion release: %s\n' "$1" >&2
  exit 1
}
require_environment() {
  local name="$1"
  [[ -n "${!name-}" ]] || fail "required environment variable ${name} is missing"
}
sha256_file() {
  local output digest
  output=$("$shasum_tool" -a 256 "$1") || return 1
  digest="${output%%[[:space:]]*}"
  [[ "$digest" =~ ^[0-9a-f]{64}$ ]] || return 1
  printf 'sha256:%s' "$digest"
}

require_environment COMPANION_PLATFORM
if [[ "$COMPANION_PLATFORM" != 'darwin' ]]; then
  exit 0
fi

for name in COMPANION_ARTIFACT COMPANION_TARGET COMPANION_ARCHITECTURE COMPANION_VERSION; do
  require_environment "$name"
done

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
public_key_receipt_helper="$script_dir/produce-public-key-receipt.sh"
omp_context_lineage_helper="$script_dir/produce-omp-context-lineage.sh"
[[ -f "$public_key_receipt_helper" && ! -L "$public_key_receipt_helper" && \
   -r "$public_key_receipt_helper" ]] \
  || fail 'public key receipt helper is missing or unsafe'
[[ -f "$omp_context_lineage_helper" && ! -L "$omp_context_lineage_helper" && \
   -r "$omp_context_lineage_helper" ]] || fail 'OMP context lineage helper is missing or unsafe'
# shellcheck source=produce-public-key-receipt.sh
source "$public_key_receipt_helper"
# shellcheck source=produce-omp-context-lineage.sh
source "$omp_context_lineage_helper"
for helper_function in resolve_public_key_receipt_release_phase produce_public_key_receipt_bundle \
  prepare_omp_context_release_lineage produce_omp_context_release_lineage cleanup_omp_context_release_lineage; do
  declare -F "$helper_function" >/dev/null 2>&1 \
    || fail 'release helper contract is incomplete'
done
"$script_dir/validate-environment.sh"

[[ "$(uname -s)" == 'Darwin' ]] || fail 'Darwin release requires macOS'
codesign_tool=/usr/bin/codesign ditto_tool=/usr/bin/ditto xcrun_tool=/usr/bin/xcrun plutil_tool=/usr/bin/plutil shasum_tool=/usr/bin/shasum
for tool in "$codesign_tool" "$ditto_tool" "$xcrun_tool" "$plutil_tool" "$shasum_tool"; do
  [[ -f "$tool" && ! -L "$tool" && -x "$tool" ]] || fail 'required Darwin release tool is unavailable'
done

case "$COMPANION_ARCHITECTURE" in
  amd64|arm64) ;;
  *) fail 'COMPANION_ARCHITECTURE is not a shipped Darwin architecture' ;;
esac
[[ "$COMPANION_TARGET" == "darwin_${COMPANION_ARCHITECTURE}"* ]] \
  || fail 'COMPANION_TARGET does not match the Darwin architecture'
[[ "$COMPANION_VERSION" =~ ^[A-Za-z0-9][A-Za-z0-9._+-]{0,255}$ ]] \
  || fail 'COMPANION_VERSION is invalid'

public_key_receipt_enabled=0 release_phase='' omp_context_lineage_enabled=0
if [[ -n "${COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT-}" ]]; then
  public_key_receipt_enabled=1
  require_environment GITHUB_REF_NAME
  resolve_public_key_receipt_release_phase
fi
artifact="$COMPANION_ARTIFACT"
[[ -f "$artifact" && ! -L "$artifact" && -x "$artifact" ]] \
  || fail 'COMPANION_ARTIFACT is not a regular executable'
artifact_dir=$(cd -- "$(dirname -- "$artifact")" && pwd)
artifact_path="$artifact_dir/$(basename -- "$artifact")"
[[ "$(basename -- "$artifact_path")" == 'auto' ]] || fail 'Darwin companion artifact must be named auto'

manifest_path="$artifact_dir/adk-companion-manifest.json"
signature_path="$artifact_dir/adk-companion-manifest.sig"
receipt_path="$artifact_dir/adk-companion-darwin-receipt.json"
public_key_bundle_path="$artifact_dir/adk-companion-public-key-receipt.bundle"
prepare_omp_context_release_lineage "$artifact_dir"
for output in "$manifest_path" "$signature_path" "$receipt_path" "$public_key_bundle_path"; do
  [[ ! -e "$output" && ! -L "$output" ]] || fail 'Darwin release output already exists'
done

temp_dir=''
receipt_temp=''
succeeded=0
cleanup() {
  local status=$?
  local rollback_status=0
  if [[ -n "$receipt_temp" ]]; then
    if rm -f -- "$receipt_temp"; then :; else rollback_status=$?; fi
  fi
  if [[ -n "$temp_dir" ]]; then
    if rm -rf -- "$temp_dir"; then :; else rollback_status=$?; fi
  fi
  if [[ "$succeeded" != '1' ]]; then
    if rm -f -- "$manifest_path" "$signature_path" "$receipt_path" &&
      cleanup_omp_context_release_lineage; then
      :
    else
      rollback_status=$?
    fi
    if [[ -e "$public_key_bundle_path" || -L "$public_key_bundle_path" ]]; then
      if rm -rf -- "$public_key_bundle_path"; then :; else rollback_status=$?; fi
    fi
  fi
  if [[ "$rollback_status" != '0' ]]; then
    printf 'companion release: partial release publication rollback failed\n' >&2
    return "$rollback_status"
  fi
  return "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/adk-companion-release.XXXXXX")
codesign_error="$temp_dir/codesign-error.txt"
report_codesign_diagnostic() {
  local label=$1 path=$2 line line_count=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    printf 'companion release: %s: %.1024s\n' "$label" "$line" >&2
    line_count=$((line_count + 1))
    [[ "$line_count" -lt 8 ]] || break
  done <"$path"
}

if [[ -n "${APPLE_SIGNING_KEYCHAIN-}" ]]; then
  "$codesign_tool" --force --sign "$APPLE_SIGNING_IDENTITY" \
    --keychain "$APPLE_SIGNING_KEYCHAIN" \
    --identifier co.autopus.adk --options runtime --timestamp "$artifact_path" \
    >/dev/null 2>"$codesign_error" \
    || { report_codesign_diagnostic 'codesign' "$codesign_error"; fail 'Developer ID signing failed'; }
else
  "$codesign_tool" --force --sign "$APPLE_SIGNING_IDENTITY" \
    --identifier co.autopus.adk --options runtime --timestamp "$artifact_path" \
    >/dev/null 2>"$codesign_error" \
    || { report_codesign_diagnostic 'codesign' "$codesign_error"; fail 'Developer ID signing failed'; }
fi

notary_container="$temp_dir/auto.zip"
notary_response="$temp_dir/notarytool.json"
notary_error="$temp_dir/notarytool-error.txt"
identity_details="$temp_dir/codesign-details.txt"
"$ditto_tool" -c -k --sequesterRsrc --keepParent "$artifact_path" "$notary_container" \
  >/dev/null 2>&1 || fail 'notarizable container creation failed'
# notary_fail prints notarytool's stderr and JSON response before failing.
notary_fail() {
  report_codesign_diagnostic 'notarytool' "$notary_error"
  report_codesign_diagnostic 'notarytool response' "$notary_response"; fail "$1"
}
# notarytool names the rejection (an unaccepted developer agreement, a revoked
# key, an Invalid verdict) only on stderr and in its JSON response. Both were
# discarded, so two failed v0.50.121 attempts left nothing but "submission
# failed". Neither carries the API key material, only its ID and the response.
"$xcrun_tool" notarytool submit "$notary_container" \
  --key "$APPLE_API_KEY_PATH" --key-id "$APPLE_API_KEY" --issuer "$APPLE_API_ISSUER" \
  --wait --output-format json >"$notary_response" 2>"$notary_error" \
  || notary_fail 'notarytool submission failed'

if ! notary_status=$("$plutil_tool" -extract status raw -o - "$notary_response" 2>/dev/null); then
  fail 'notarytool response is missing status'
fi
if ! notary_id=$("$plutil_tool" -extract id raw -o - "$notary_response" 2>/dev/null); then
  fail 'notarytool response is missing submission UUID'
fi
uuid_pattern='^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[1-5][0-9A-Fa-f]{3}-[89ABab][0-9A-Fa-f]{3}-[0-9A-Fa-f]{12}$'
[[ "$notary_status" == 'Accepted' && "$notary_id" =~ $uuid_pattern ]] \
  || notary_fail 'notarization was not Accepted with a valid submission UUID'

designated_requirement='identifier "co.autopus.adk" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "GP2PFA2PUV" and notarized'
if ! "$codesign_tool" --verify --strict --all-architectures --verbose=2 \
  --check-notarization "-R=$designated_requirement" "$artifact_path" \
  >"$identity_details" 2>&1; then
  report_codesign_diagnostic 'codesign verify' "$identity_details"
  fail 'code-sign designated requirement verification failed'
fi
"$codesign_tool" -dv --verbose=4 "$artifact_path" >"$identity_details" 2>&1 \
  || fail 'code-sign identity inspection failed'
grep -Fqx 'Identifier=co.autopus.adk' "$identity_details" \
  || fail 'signed binary identifier is invalid'
grep -Fqx 'TeamIdentifier=GP2PFA2PUV' "$identity_details" \
  || fail 'signed binary Team ID is invalid'
grep -Eq '^Timestamp=.+$' "$identity_details" \
  || fail 'signed binary secure timestamp is missing'
grep -Eq '^CodeDirectory .*flags=.*\(.*runtime.*\)' "$identity_details" \
  || fail 'signed binary hardened runtime is missing'
if grep -Fqx 'Signature=adhoc' "$identity_details"; then
  fail 'ad hoc signature is forbidden'
fi

execution_smoke_digest=$(sha256_file "$artifact_path") \
  || fail 'cannot digest final signed companion before execution smoke'
execution_smoke_env=(PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}")
if [[ "$COMPANION_ARCHITECTURE" == 'arm64' ]]; then
  for name in OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE OMP_CONTEXT_RELEASE_CANARY_ROOT; do require_environment "$name"; done
  execution_smoke_env+=(OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE="$OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE" OMP_CONTEXT_RELEASE_CANARY_ROOT="$OMP_CONTEXT_RELEASE_CANARY_ROOT")
fi
# Liveness bound: macOS first-run assessment of a freshly signed binary under nobody isolation exceeded 15s in v0.50.104.
env -i "${execution_smoke_env[@]}" "$COMPANION_EXEC_SMOKE_GATE" \
  --artifact "$artifact_path" --expected-version "$COMPANION_VERSION" \
  --architecture "$COMPANION_ARCHITECTURE" --timeout 45s \
  || fail 'final signed companion execution smoke failed'
[[ "$(sha256_file "$artifact_path")" == "$execution_smoke_digest" ]] \
  || fail 'final signed companion changed during execution smoke'

if [[ "$public_key_receipt_enabled" == '1' || "$omp_context_lineage_enabled" == '1' ]]; then
  signing_key_digest_before=$(sha256_file "$COMPANION_SIGNING_KEY_FILE") \
    || fail 'cannot digest companion signing key'
fi

manifest_sign_args=(companion-manifest sign \
  --artifact "$artifact_path" \
  --manifest-output "$manifest_path" \
  --signature-output "$signature_path" \
  --version "$COMPANION_VERSION" \
  --platform "$COMPANION_PLATFORM" \
  --architecture "$COMPANION_ARCHITECTURE" \
  --build-provenance "$COMPANION_BUILD_PROVENANCE" \
  --handoff "$COMPANION_HANDOFF" \
  --rollback-floor "$COMPANION_ROLLBACK_FLOOR" \
  --issued-at "$COMPANION_ISSUED_AT" \
  --expires-at "$COMPANION_EXPIRES_AT" \
  --key-id "$COMPANION_KEY_ID")
env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
  "$COMPANION_SIGNER" "${manifest_sign_args[@]}" \
  <"$COMPANION_SIGNING_KEY_FILE" >/dev/null \
  || fail 'companion manifest signing failed'

[[ -f "$manifest_path" && ! -L "$manifest_path" ]] || fail 'companion manifest was not produced'
[[ -f "$signature_path" && ! -L "$signature_path" ]] || fail 'companion signature was not produced'
[[ "$(wc -c <"$signature_path" | tr -d '[:space:]')" == '64' ]] \
  || fail 'companion signature is not raw Ed25519 bytes'
if ! artifact_digest=$("$plutil_tool" -extract artifact_digest raw -o - "$manifest_path" 2>/dev/null); then
  fail 'companion manifest artifact digest is missing'
fi
actual_digest=$(sha256_file "$artifact_path") || fail 'cannot digest signed companion artifact'
[[ "$artifact_digest" == "$execution_smoke_digest" ]] \
  || fail 'companion manifest does not bind the execution-smoked artifact'
[[ "$artifact_digest" == "$actual_digest" ]] || fail 'artifact changed after companion manifest digest'
manifest_digest=$(sha256_file "$manifest_path") || fail 'cannot digest companion manifest'
signature_digest=$(sha256_file "$signature_path") || fail 'cannot digest companion signature'

receipt_temp=$(mktemp "$artifact_dir/.adk-companion-receipt.XXXXXX")
printf '%s' \
  "{\"schema_version\":\"adk-companion-darwin-receipt.v1\",\"artifact_digest\":\"$artifact_digest\",\"manifest_digest\":\"$manifest_digest\",\"signature_digest\":\"$signature_digest\",\"version\":\"$COMPANION_VERSION\",\"platform\":\"darwin\",\"architecture\":\"$COMPANION_ARCHITECTURE\",\"code_identity\":{\"identifier\":\"co.autopus.adk\",\"team_id\":\"GP2PFA2PUV\",\"developer_id\":true,\"hardened_runtime\":true,\"secure_timestamp\":true,\"designated_requirement_verified\":true},\"notarization\":{\"status\":\"Accepted\",\"submission_id\":\"$notary_id\"}}" \
  >"$receipt_temp"
chmod 0600 "$receipt_temp"
mv -f -- "$receipt_temp" "$receipt_path"
receipt_temp=''

if [[ "$public_key_receipt_enabled" == '1' ]]; then
  produce_public_key_receipt_bundle \
    "$artifact_path" "$manifest_path" "$signature_path" \
    "$public_key_bundle_path" "$signing_key_digest_before" "$release_phase"
fi
produce_omp_context_release_lineage "$actual_digest" "${signing_key_digest_before-}"
succeeded=1
