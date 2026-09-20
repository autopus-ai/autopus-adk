#!/usr/bin/env bash
# Everything about a release that can be checked without the promotion signing
# key or a live provider, in one command.
#
# The release is a long procedure whose expensive parts come last, so a wrong
# constant used to surface only after the tag was burned. Every check here is
# read-only and cheap; none of them mutate the repository or GitHub.
#
# Exit 0 means: nothing left to verify locally. What remains needs the K3
# promotion key and a live provider, which by design do not live in CI.
set -euo pipefail
umask 077

readonly repository='autopus-ai/autopus-adk'
readonly release_tag="${1:-v0.50.118}"
readonly predecessor_tag="${2:-v0.50.117}"
readonly release_ref="refs/tags/${release_tag}"
readonly version="${release_tag#v}"

failures=0
pass() { printf '  ok    %s\n' "$1"; }
warn() { printf '  warn  %s\n' "$1"; }
bad() { printf '  FAIL  %s\n' "$1"; failures=$((failures + 1)); }
section() { printf '\n%s\n' "$1"; }
check() { if eval "$2" >/dev/null 2>&1; then pass "$1"; else bad "$1"; fi; }
preflight_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
. "$preflight_dir/preflight-prep-inputs-lib.sh"

section "tooling"
for tool in git gh jq go gofmt cosign shasum; do
  check "$tool available" "command -v $tool"
done
actor_id=$(gh api user --jq .id 2>/dev/null || printf 'unknown')
if [[ "$actor_id" == '204883817' ]]; then
  pass "authenticated as release actor 204883817"
else
  bad "authenticated as $actor_id, expected release actor 204883817"
fi

section "source"
if [[ -z "$(git status --porcelain)" ]]; then pass 'working tree clean'; else bad 'working tree dirty'; fi
git fetch --no-tags -q origin main 2>/dev/null || true
head_commit=$(git rev-parse --verify HEAD)
if [[ "$(git rev-parse --verify origin/main)" == "$head_commit" ]]; then
  pass "HEAD equals origin/main ($head_commit)"
else
  bad 'HEAD differs from origin/main'
fi
if git rev-parse --verify --quiet "$release_ref" >/dev/null; then
  bad "$release_tag already exists locally"
else
  pass "$release_tag absent locally"
fi
if [[ -n "$(git ls-remote --refs origin "$release_ref" 2>/dev/null)" ]]; then
  bad "$release_tag already exists on origin"
else
  pass "$release_tag absent on origin"
fi

section "content gates"
if [[ -z "$(gofmt -l pkg internal cmd templates)" ]]; then pass 'gofmt clean'; else bad 'gofmt reports files'; fi
if go mod tidy -diff >/dev/null 2>&1; then pass 'go.mod and go.sum are tidy'; else bad 'go mod tidy would change go.mod or go.sum'; fi
oversized=$(go run ./cmd/source-lines --max 300 --ext .go pkg internal cmd 2>&1 | grep -v '^source-lines:' || true)
if [[ -z "$oversized" ]]; then pass 'no source file over 300 code lines'; else bad "over 300 code lines: $oversized"; fi
before_generate=$(git status --porcelain)
if go run ./cmd/generate-templates >/dev/null 2>&1 &&
  [[ "$(git status --porcelain)" == "$before_generate" ]]; then
  pass 'generated surfaces converge'
else
  bad 'generate-templates changes tracked files'
fi

section "release wiring for $release_tag"
check "workflow triggers on $release_tag" \
  "grep -qF -- \"'${release_tag}'\" .github/workflows/release.yaml"
check "prepare-release pins $release_tag" \
  "grep -qF \"readonly release_tag='${release_tag}'\" scripts/companion-release/prepare-release.sh"
phase=$(awk -v tag="$release_tag" '$1 == tag")" { gsub(/[^A-Z0-9]/, "", $2); print $2 }' \
  scripts/companion-release/validate-source.sh | head -1)
if [[ -n "$phase" ]]; then
  pass "validate-source maps $release_tag to phase ${phase#RELEASEPHASE}"
else
  bad "validate-source does not map $release_tag; the tag would be refused as outside policy"
fi
predecessor_commit=$(git rev-parse --verify "${predecessor_tag}^{commit}" 2>/dev/null || printf '')
if [[ -n "$predecessor_commit" ]] &&
  grep -qF "$predecessor_commit" scripts/companion-release/validate-source.sh; then
  pass "ancestor constant matches $predecessor_tag source commit"
else
  bad "no ancestor constant equals $predecessor_tag source commit $predecessor_commit"
fi
if [[ -n "$predecessor_commit" ]] &&
  git merge-base --is-ancestor "$predecessor_commit" HEAD >/dev/null 2>&1; then
  pass "HEAD contains $predecessor_tag"
else
  bad "HEAD does not contain $predecessor_tag"
fi

section "tag authority"
ruleset=$(gh api "repos/${repository}/rulesets" --jq \
  ".[] | select(.name == \"autopus-${release_tag}-release-authority\") | .id" 2>/dev/null || printf '')
if [[ -n "$ruleset" ]]; then
  pass "ruleset autopus-${release_tag}-release-authority exists (id $ruleset)"
  bypass=$(gh api "repos/${repository}/rulesets/${ruleset}" --jq '[.bypass_actors[].actor_id] | join(",")' 2>/dev/null || printf '')
  if [[ "$bypass" == '204883817' ]]; then
    pass 'ruleset is open for the release actor; seal it right after the tag exists'
  elif [[ -z "$bypass" ]]; then
    bad 'ruleset is already sealed; the tag push would be blocked'
  else
    bad "ruleset bypass actors are unexpected: $bypass"
  fi
else
  bad "ruleset autopus-${release_tag}-release-authority is missing"
fi

section "companion validity window"
issued=$(gh variable get ADK_COMPANION_ISSUED_AT --repo "$repository" 2>/dev/null || printf '')
expires=$(gh variable get ADK_COMPANION_EXPIRES_AT --repo "$repository" 2>/dev/null || printf '')
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
if [[ -n "$issued" && -n "$expires" && "$issued" < "$now" && "$now" < "$expires" ]]; then
  pass "companion window covers now ($issued .. $expires)"
else
  bad "companion window does not cover $now ($issued .. $expires)"
fi
floor=$(gh variable get ADK_COMPANION_ROLLBACK_FLOOR --repo "$repository" 2>/dev/null || printf '')
if [[ "$floor" =~ ^[1-9][0-9]*$ ]]; then
  pass "rollback floor is $floor; pass it to the lineage verifier"
else
  bad 'rollback floor variable is missing or malformed'
fi

section "predecessor $predecessor_tag"
release_json=$(gh api "repos/${repository}/releases/tags/${predecessor_tag}" 2>/dev/null || printf '')
if [[ -n "$release_json" ]] && jq -e '.draft == false' <<<"$release_json" >/dev/null; then
  pass "published, release id $(jq -r .id <<<"$release_json")"
else
  bad 'predecessor release is missing or still a draft'
fi
expected_assets=$(jq -r '[.assets[].name] | sort | .[]' <<<"${release_json:-{\}}" 2>/dev/null |
  sed "s/${predecessor_tag#v}/${version}/g" || printf '')
asset_count=$(printf '%s\n' "$expected_assets" | grep -c . || true)
if [[ "$asset_count" -eq 15 ]]; then
  pass "expected asset shape resolves to 15 names for $version"
else
  bad "expected asset shape resolved to $asset_count names, want 15"
fi

section "artifact trust anchors"
work=$(mktemp -d "${TMPDIR:-/tmp}/release-preflight.XXXXXX")
trap 'rm -rf -- "$work"' EXIT
if gh release download "$predecessor_tag" -p 'checksums.txt' -p 'checksums.txt.bundle' \
  -D "$work" --clobber >/dev/null 2>&1; then
  # The predecessor was signed before the repository transfer; certificate identity is immutable.
  identity="https://github.com/Insajin/autopus-adk/.github/workflows/release.yaml@refs/tags/${predecessor_tag}"
  if (cd "$work" && cosign verify-blob checksums.txt --bundle checksums.txt.bundle \
    --certificate-identity "$identity" \
    --certificate-oidc-issuer 'https://token.actions.githubusercontent.com') >/dev/null 2>&1; then
    pass 'sigstore keyless anchor verifies on the predecessor'
  else
    bad 'sigstore verification of the predecessor failed'
  fi
else
  bad 'cannot download predecessor checksums and bundle'
fi
if gh release download "$predecessor_tag" -p 'checksums.txt.signatures' -D "$work" --clobber >/dev/null 2>&1; then
  k1='e1fdfe066484c7eae8ff16fa4b1ee6237b8d06299c2b66ced485f029af77837f'
  if grep -qF "$k1" "$work/checksums.txt.signatures"; then
    pass 'predecessor envelope carries the compiled K1 anchor'
  else
    bad 'predecessor envelope does not carry K1; self-update trust would break'
  fi
else
  bad 'cannot download the predecessor signature envelope'
fi

check_release_prep_inputs

section "release script contracts"
# Both trees are checked: companion-release holds the evidence-producing scripts,
# release-tools holds the operator tooling. The split exists because a security
# test treats everything under companion-release as production evidence code and
# forbids fixture references there, which operator tooling legitimately needs.
for script in scripts/companion-release/*.sh scripts/release-tools/*.sh; do
  bash -n "$script" || bad "$script does not parse"
done
pass 'release scripts parse'
for test_script in scripts/companion-release/tests/*.sh; do
  if bash "$test_script" >/dev/null 2>&1; then
    pass "$(basename "$test_script")"
  else
    bad "$(basename "$test_script")"
  fi
done

section "verdict"
if [[ "$failures" -eq 0 ]]; then
  cat <<EOF
  All local checks passed. What remains cannot be done from here:

    1. Generate K3-signed production evidence with the promotion signing key
       and a live provider, then push the evidence tag and prep lock:
         scripts/companion-release/prepare-release.sh --preflight ... then --apply
    2. Push ${release_tag}. The workflow builds, signs, and publishes.
    3. Seal ruleset bypass immediately after the tag exists.
    4. Compare the published asset set against the expected 15 names.
    5. Say in the release description that the tag is R2-signed and how to
       verify it (git verify-tag against release-tag-signing-2026-q3-r2.pub).
EOF
  exit 0
fi
printf '  %d check(s) failed; fix them before starting the release.\n' "$failures"
exit 1
