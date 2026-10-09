#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'OMP context release binary: %s\n' "$1" >&2; exit 1; }

[[ "${COMPANION_RELEASE_TAG:-}" == 'v0.50.125' ]] || exit 0
[[ "${OMP_CONTEXT_STATIC_POLICY_B64:-}" =~ ^[A-Za-z0-9_-]+$ ]] ||
  fail 'compiled static policy is missing or malformed'
for name in COMPANION_ARTIFACT COMPANION_PLATFORM COMPANION_ARCHITECTURE \
  OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256
do
  [[ -n "${!name-}" ]] || fail "required ${name} is missing"
done
[[ "$OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256" =~ ^[0-9a-f]{64}$ ]] \
  || fail 'expected candidate digest is malformed'
[[ -f "$COMPANION_ARTIFACT" && ! -L "$COMPANION_ARTIFACT" ]] \
  || fail 'GoReleaser artifact is unsafe'
[[ "$COMPANION_PLATFORM" == 'darwin' && "$COMPANION_ARCHITECTURE" == 'arm64' ]] \
  || exit 0
if command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$COMPANION_ARTIFACT" | awk '{print $1}')
elif command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$COMPANION_ARTIFACT" | awk '{print $1}')
else
  fail 'SHA-256 tool is unavailable'
fi
[[ "$actual" == "$OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256" ]] \
  || fail 'GoReleaser Darwin arm64 binary differs from signed candidate'
