#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'OMP context candidate build: %s\n' "$1" >&2; exit 1; }

[[ $# == 1 ]] || fail 'usage: build-omp-context-candidate.sh OUTPUT'
readonly output=$1
readonly expected_tag='v0.50.125'
readonly expected_version='0.50.125'
readonly expected_go_toolchain='go1.26.9'
[[ "${COMPANION_RELEASE_TAG:-}" == "$expected_tag" ]] || fail 'release tag is not exact A35'
[[ "${GITHUB_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'source commit is malformed'
[[ "${COMPANION_SOURCE_TREE:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'source tree is malformed'
[[ "${OMP_CONTEXT_STATIC_POLICY_B64:-}" =~ ^[A-Za-z0-9_-]+$ &&
   "${#OMP_CONTEXT_STATIC_POLICY_B64}" -le 21846 ]] \
  || fail 'compiled static policy is missing or malformed'
[[ ! -e "$output" && ! -L "$output" ]] || fail 'output already exists'
for tool in git go; do command -v "$tool" >/dev/null || fail "${tool} is unavailable"; done
go_version=$(GOENV=off GOTOOLCHAIN="$expected_go_toolchain" go env GOVERSION) ||
  fail 'exact Go toolchain is unavailable'
[[ "$go_version" == "$expected_go_toolchain" ]] || fail 'exact Go toolchain identity differs'
[[ "$(git rev-parse --verify 'HEAD^{commit}')" == "$GITHUB_SHA" ]] \
  || fail 'checked-out commit differs from release source'
[[ "$(git rev-parse --verify 'HEAD^{tree}')" == "$COMPANION_SOURCE_TREE" ]] \
  || fail 'checked-out tree differs from release source'
commit_date=$(TZ=UTC git show -s --date='format-local:%Y-%m-%dT%H:%M:%SZ' \
  --format=%cd "$GITHUB_SHA") || fail 'cannot resolve source commit date'
[[ "$commit_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]] \
  || fail 'commit date is malformed'
short_commit=${GITHUB_SHA:0:8}
ldflags="-s -w -X github.com/insajin/autopus-adk/pkg/version.version=${expected_version}"
ldflags+=" -X github.com/insajin/autopus-adk/pkg/version.commit=${short_commit}"
ldflags+=" -X github.com/insajin/autopus-adk/pkg/version.date=${commit_date}"
ldflags+=" -X github.com/insajin/autopus-adk/pkg/version.sourceCommit=${GITHUB_SHA}"
ldflags+=" -X github.com/insajin/autopus-adk/pkg/version.sourceTree=${COMPANION_SOURCE_TREE}"
ldflags+=" -X github.com/insajin/autopus-adk/internal/cli.pipelineOMPActiveStaticPolicyB64=${OMP_CONTEXT_STATIC_POLICY_B64}"
env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}" \
  GOTOOLCHAIN="$expected_go_toolchain" GOENV=off \
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOARM64=v8.0 \
  go build -trimpath -buildvcs=false -ldflags "$ldflags" -o "$output" ./cmd/auto \
  || fail 'canonical candidate build failed'
[[ -f "$output" && ! -L "$output" && -s "$output" ]] || fail 'candidate output is unsafe'
chmod 0700 "$output"
