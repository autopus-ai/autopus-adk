#!/usr/bin/env bash
set -euo pipefail
umask 077

readonly EXPECTED_K1_FINGERPRINT='e1fdfe066484c7eae8ff16fa4b1ee6237b8d06299c2b66ced485f029af77837f'
readonly COSIGN_IDENTITY='https://github.com/autopus-ai/autopus-adk/.github/workflows/release.yaml@refs/tags/v0.50.118'
readonly COSIGN_ISSUER='https://token.actions.githubusercontent.com'

fail() {
  printf 'current release signatures: %s\n' "$1" >&2
  exit 1
}

require_input() {
  local candidate_file=$1
  local label=$2
  [[ -f "$candidate_file" && ! -L "$candidate_file" && \
     -r "$candidate_file" && -s "$candidate_file" ]] \
    || fail "${label} input is empty or unsafe"
}

[[ $# -eq 3 ]] \
  || fail 'usage: verify-current-release-signatures.sh CHECKSUMS BUNDLE ENVELOPE'
readonly checksums_path=$1
readonly bundle_path=$2
readonly envelope_path=$3

# Defense in depth for direct callers that do not use verify-current-release.sh.
unset GITHUB_TOKEN GH_TOKEN

require_input "$checksums_path" 'checksums'
require_input "$bundle_path" 'cosign bundle'
require_input "$envelope_path" 'release signature envelope'

for tool in awk cosign mktemp openssl; do
  command -v "$tool" >/dev/null 2>&1 \
    || fail "required signature verification tool is unavailable: ${tool}"
done

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) \
  || fail 'cannot resolve signature verification helper directory'
repo_root=$(cd -- "$script_dir/../.." && pwd) \
  || fail 'cannot resolve repository root'
release_verifier="$repo_root/scripts/release-signing/verify-checksums-v1.sh"
[[ -f "$release_verifier" && ! -L "$release_verifier" && \
   -r "$release_verifier" ]] \
  || fail 'checked-in release signature verifier is missing or unsafe'

# Reject every envelope except one exact K1 record before any cryptographic or
# network-facing verifier runs. The checked-in verifier remains authoritative
# for the complete wire-format and ECDSA validation contract.
if ! LC_ALL=C awk -v expected="$EXPECTED_K1_FINGERPRINT" '
  BEGIN { valid = 1 }
  NR == 1 {
    if ($0 != "AUTOPUS-RELEASE-SIGNATURE-V1") valid = 0
    next
  }
  NR == 2 {
    if (length($0) <= 65 || substr($0, 1, 64) != expected ||
        substr($0, 65, 1) != "\t" || index(substr($0, 66), "\t") != 0) {
      valid = 0
    }
    next
  }
  { valid = 0 }
  END { if (NR != 2 || valid != 1) exit 1 }
' "$envelope_path"; then
  fail 'release signature envelope must contain exactly one K1 record'
fi

# shellcheck source=../release-signing/verify-checksums-v1.sh
source "$release_verifier"
declare -F verify_release_checksums_v1 >/dev/null 2>&1 \
  || fail 'checked-in release signature verifier contract is incomplete'

verification_dir=''
cleanup() {
  local status=$?
  local cleanup_status=0
  if [[ -n "$verification_dir" ]]; then
    rm -rf -- "$verification_dir" || cleanup_status=$?
  fi
  if ((cleanup_status != 0)); then
    printf 'current release signatures: cleanup failed\n' >&2
    return "$cleanup_status"
  fi
  return "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

verification_dir=$(mktemp -d \
  "${TMPDIR:-/tmp}/adk-current-release-signatures.XXXXXX") \
  || fail 'cannot allocate signature verification workspace'
[[ -d "$verification_dir" && ! -L "$verification_dir" ]] \
  || fail 'signature verification workspace is unsafe'

# @AX:ANCHOR [AUTO]: This helper is the cryptographic trust boundary between an immutable GitHub release and Homebrew publication.
# @AX:REASON [AUTO]: Release and recovery workflows must verify both the exact K1 envelope and the A23 GitHub Actions Sigstore identity before creating a tap write token.
if ! verify_release_checksums_v1 \
  "$checksums_path" "$envelope_path" "$verification_dir" \
  >/dev/null 2>&1; then
  fail 'release ECDSA signature verification failed'
fi

if ! cosign verify-blob \
  --bundle "$bundle_path" \
  --certificate-identity "$COSIGN_IDENTITY" \
  --certificate-oidc-issuer "$COSIGN_ISSUER" \
  "$checksums_path" >/dev/null 2>&1; then
  fail 'release keyless signature verification failed'
fi

printf 'current release signatures: exact A23 ECDSA and keyless signatures verified\n'
