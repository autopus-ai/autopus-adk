#!/usr/bin/env bash
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd -- "$tests_dir/.." && pwd)
helper="$script_dir/verify-current-release-signatures.sh"

readonly EXPECTED_K1_FINGERPRINT='e1fdfe066484c7eae8ff16fa4b1ee6237b8d06299c2b66ced485f029af77837f'
readonly OFFLINE_K2_FINGERPRINT='93d9f681d829f2d0bdba7e1853e6acf9ae2ffd2c760355853218e920c35cc5ff'

fail() {
  printf 'release current signature hardening test: %s\n' "$1" >&2
  exit 1
}

contains() {
  grep -Fq -- "$2" "$1" || fail "missing static contract: $2"
}

not_contains() {
  if grep -Fq -- "$2" "$1"; then
    fail "forbidden static contract: $2"
  fi
}

[[ -f "$helper" && ! -L "$helper" && -x "$helper" ]] \
  || fail 'signature verification helper is missing or unsafe'

contains "$helper" 'set -euo pipefail'
contains "$helper" 'umask 077'
contains "$helper" '[[ $# -eq 3 ]]'
contains "$helper" "readonly EXPECTED_K1_FINGERPRINT='$EXPECTED_K1_FINGERPRINT'"
contains "$helper" 'scripts/release-signing/verify-checksums-v1.sh'
contains "$helper" 'verify_release_checksums_v1'
contains "$helper" 'cosign verify-blob'
contains "$helper" '--certificate-identity "$COSIGN_IDENTITY"'
contains "$helper" '--certificate-oidc-issuer "$COSIGN_ISSUER"'
contains "$helper" 'https://github.com/autopus-ai/autopus-adk/.github/workflows/release.yaml@refs/tags/v0.50.118'
contains "$helper" 'https://token.actions.githubusercontent.com'
not_contains "$helper" "$OFFLINE_K2_FINGERPRINT"
not_contains "$helper" '--offline'
not_contains "$helper" '--insecure-ignore-tlog'
not_contains "$helper" '--insecure-ignore-sct'
not_contains "$helper" 'set -x'

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/release-current-signature-test.XXXXXX")
cleanup() {
  local status=$?
  rm -rf -- "$temp_dir" || status=$?
  return "$status"
}
trap cleanup EXIT

mock_bin="$temp_dir/bin"
install -m 0700 -d "$mock_bin"
for tool_name in cosign openssl; do
  tool_path="$mock_bin/$tool_name"
  printf '%s\n' \
    '#!/usr/bin/env bash' \
    'set -euo pipefail' \
    'printf "%s\\n" "${0##*/}" >> "$INVOCATION_LOG"' \
    'exit 99' >"$tool_path"
  chmod 0700 "$tool_path"
done

invocation_log="$temp_dir/invocations.log"
valid_checksums="$temp_dir/checksums.txt"
valid_bundle="$temp_dir/checksums.txt.bundle"
valid_envelope="$temp_dir/checksums.txt.signatures"
printf '%064d  autopus-adk_0.50.118_darwin_arm64.tar.gz\n' 1 >"$valid_checksums"
printf '%s\n' 'SECRET_BUNDLE_SENTINEL' >"$valid_bundle"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%s\tAA==\n' \
  "$EXPECTED_K1_FINGERPRINT" >"$valid_envelope"

expect_precrypto_rejection() {
  local case_name=$1
  shift
  local output=''
  : >"$invocation_log"
  if output=$(PATH="$mock_bin:$PATH" INVOCATION_LOG="$invocation_log" \
    bash "$helper" "$@" 2>&1); then
    fail "$case_name unexpectedly succeeded"
  fi
  [[ ! -s "$invocation_log" ]] \
    || fail "$case_name invoked a cryptographic or network tool"
  [[ "$output" != *'SECRET_BUNDLE_SENTINEL'* ]] \
    || fail "$case_name disclosed input bytes"
}

expect_precrypto_rejection wrong_arg_count
expect_precrypto_rejection wrong_arg_count_two "$valid_checksums" "$valid_bundle"
expect_precrypto_rejection wrong_arg_count_four \
  "$valid_checksums" "$valid_bundle" "$valid_envelope" extra

empty_checksums="$temp_dir/empty-checksums"
empty_bundle="$temp_dir/empty-bundle"
empty_envelope="$temp_dir/empty-envelope"
: >"$empty_checksums"
: >"$empty_bundle"
: >"$empty_envelope"
expect_precrypto_rejection empty_checksums \
  "$empty_checksums" "$valid_bundle" "$valid_envelope"
expect_precrypto_rejection empty_bundle \
  "$valid_checksums" "$empty_bundle" "$valid_envelope"
expect_precrypto_rejection empty_envelope \
  "$valid_checksums" "$valid_bundle" "$empty_envelope"

checksums_link="$temp_dir/checksums-link"
bundle_link="$temp_dir/bundle-link"
envelope_link="$temp_dir/envelope-link"
ln -s "$valid_checksums" "$checksums_link"
ln -s "$valid_bundle" "$bundle_link"
ln -s "$valid_envelope" "$envelope_link"
expect_precrypto_rejection symlink_checksums \
  "$checksums_link" "$valid_bundle" "$valid_envelope"
expect_precrypto_rejection symlink_bundle \
  "$valid_checksums" "$bundle_link" "$valid_envelope"
expect_precrypto_rejection symlink_envelope \
  "$valid_checksums" "$valid_bundle" "$envelope_link"

wrong_header="$temp_dir/wrong-header"
unknown_fingerprint="$temp_dir/unknown-fingerprint"
k2_only="$temp_dir/k2-only"
k1_plus_k2="$temp_dir/k1-plus-k2"
duplicate_k1="$temp_dir/duplicate-k1"
extra_tab="$temp_dir/extra-tab"
no_final_lf="$temp_dir/no-final-lf"
printf 'WRONG\n%s\tAA==\n' "$EXPECTED_K1_FINGERPRINT" >"$wrong_header"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%064d\tAA==\n' 7 >"$unknown_fingerprint"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%s\tAA==\n' \
  "$OFFLINE_K2_FINGERPRINT" >"$k2_only"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%s\tAA==\n%s\tAA==\n' \
  "$EXPECTED_K1_FINGERPRINT" "$OFFLINE_K2_FINGERPRINT" >"$k1_plus_k2"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%s\tAA==\n%s\tAA==\n' \
  "$EXPECTED_K1_FINGERPRINT" "$EXPECTED_K1_FINGERPRINT" >"$duplicate_k1"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%s\tAA==\textra\n' \
  "$EXPECTED_K1_FINGERPRINT" >"$extra_tab"
printf 'AUTOPUS-RELEASE-SIGNATURE-V1\n%s\tAA==' \
  "$EXPECTED_K1_FINGERPRINT" >"$no_final_lf"

for malformed_case in \
  wrong_header unknown_fingerprint k2_only k1_plus_k2 duplicate_k1 extra_tab no_final_lf
do
  malformed_path="$temp_dir/${malformed_case//_/-}"
  expect_precrypto_rejection "$malformed_case" \
    "$valid_checksums" "$valid_bundle" "$malformed_path"
done

printf 'release current signature hardening test: PASS\n'
