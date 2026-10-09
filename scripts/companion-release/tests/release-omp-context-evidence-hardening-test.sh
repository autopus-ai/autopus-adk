#!/usr/bin/env bash
set -euo pipefail
umask 077

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
helper="$root/scripts/companion-release/verify-omp-context-evidence-tag.sh"
binary_helper="$root/scripts/companion-release/verify-omp-context-release-binary.sh"
verifier_source="$root/scripts/companion-release/ompcontextverify/main.go"
workflow="$root/.github/workflows/release.yaml"
config="$root/.goreleaser.yaml"
current="$root/scripts/companion-release/verify-current-release.sh"
temp=$(mktemp -d "${TMPDIR:-/tmp}/omp-context-evidence-test.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT
fail() { printf 'OMP context evidence hardening: %s\n' "$1" >&2; exit 1; }

for required in \
  'omp-context-evidence-v0.50.124' \
  'OMP_CONTEXT_STATIC_POLICY_B64' \
  'omp-context-promotion-report.v1.json' \
  'omp-context-promotion-attestation.v2.json' \
  '--mode active' '--mode historical'
do
  grep -Fq -- "$required" "$workflow" "$config" "$current" "$helper" ||
    fail "normal evidence lane is missing $required"
done
# The burned v0.50.110 evidence coordinate is failure history, never active authority.
for forbidden in \
  'omp-context-evidence-v0.50.110' \
  'omp-context-bridge-release.v1.json' \
  'adk-key-rotation-v1.json' \
  'adk-key-rotation-v1.sig' \
  '--expected-signing-key-id'
do
  if grep -Fq -- "$forbidden" "$workflow" "$config" "$current" "$verifier_source"; then
    fail "active A23 lane contains forbidden authority $forbidden"
  fi
done
grep -Fq 'policy.PromotionSigningKeyID' "$verifier_source" ||
  fail 'evidence verifier does not consume the policy-owned signing key'
grep -Fq 'ValidateOMPContextPromotionActiveStaticPolicyV3' "$verifier_source" ||
  fail 'active evidence verifier does not delegate exact K3 validation to core policy'
grep -Fq 'OMPContextPromotionProviderAuthorityDigestV1(report)' "$verifier_source" ||
  fail 'evidence verifier does not bind report-derived provider authority to static policy'

repo="$temp/repo"
git init -q "$repo"
git -C "$repo" config user.name 'OMP Evidence Fixture'
git -C "$repo" config user.email 'omp-evidence@example.invalid'
report="$temp/omp-context-promotion-report.v1.json"
attestation="$temp/omp-context-promotion-attestation.v2.json"
printf '{"fixture":"fresh-v0.50.124-report"}' >"$report"
printf '{"fixture":"fresh-v0.50.124-attestation"}' >"$attestation"
report_blob=$(git -C "$repo" hash-object -w "$report")
attestation_blob=$(git -C "$repo" hash-object -w "$attestation")
tree=$(printf '100644 blob %s\t%s\n100644 blob %s\t%s\n' \
  "$attestation_blob" 'omp-context-promotion-attestation.v2.json' \
  "$report_blob" 'omp-context-promotion-report.v1.json' | git -C "$repo" mktree)
commit=$(printf 'fresh A23 orphan evidence\n' | \
  GIT_AUTHOR_DATE='2026-08-31T00:00:00Z' GIT_COMMITTER_DATE='2026-08-31T00:00:00Z' \
  git -C "$repo" commit-tree "$tree")
GIT_COMMITTER_DATE='2026-08-31T00:00:01Z' \
  git -C "$repo" tag -am 'fresh A24 evidence' omp-context-evidence-v0.50.124 "$commit"
tag_object=$(git -C "$repo" rev-parse refs/tags/omp-context-evidence-v0.50.124)
report_sha=$(shasum -a 256 "$report" | awk '{print $1}')
attestation_sha=$(shasum -a 256 "$attestation" | awk '{print $1}')

run_helper() {
  local output=$1
  shift
  env OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA="$tag_object" \
    OMP_CONTEXT_EVIDENCE_COMMIT_SHA="$commit" \
    OMP_CONTEXT_EVIDENCE_TREE_SHA="$tree" \
    OMP_CONTEXT_EVIDENCE_REPORT_SHA256="$report_sha" \
    OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256="$attestation_sha" \
    "$@" bash "$helper" "$output"
}

(cd "$repo" && run_helper "$temp/valid")
cmp "$report" "$temp/valid/omp-context-promotion-report.v1.json"
cmp "$attestation" "$temp/valid/omp-context-promotion-attestation.v2.json"
if (cd "$repo" && run_helper "$temp/bad-digest" \
  OMP_CONTEXT_EVIDENCE_REPORT_SHA256="$(printf 'f%.0s' {1..64})"); then
  fail 'tampered report digest passed'
fi

GIT_COMMITTER_DATE='2026-08-31T00:00:02Z' \
  git -C "$repo" tag -am 'stale A22 evidence' omp-context-evidence-v0.50.108 "$commit"
git -C "$repo" update-ref -d refs/tags/omp-context-evidence-v0.50.124
if (cd "$repo" && run_helper "$temp/stale-ref"); then
  fail 'stale v0.50.108 evidence ref passed as A23'
fi
git -C "$repo" update-ref refs/tags/omp-context-evidence-v0.50.124 "$tag_object"

git -C "$repo" update-ref refs/tags/omp-context-evidence-v0.50.124 "$commit"
if (cd "$repo" && run_helper "$temp/lightweight" \
  OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA="$commit"); then
  fail 'lightweight evidence tag passed'
fi
git -C "$repo" update-ref refs/tags/omp-context-evidence-v0.50.124 "$tag_object"

extra_blob=$(printf 'drift' | git -C "$repo" hash-object -w --stdin)
extra_tree=$(printf '100644 blob %s\t%s\n100644 blob %s\t%s\n100644 blob %s\t%s\n' \
  "$attestation_blob" 'omp-context-promotion-attestation.v2.json' \
  "$report_blob" 'omp-context-promotion-report.v1.json' "$extra_blob" 'stale-policy.json' |
  git -C "$repo" mktree)
extra_commit=$(printf 'drifted orphan evidence\n' | git -C "$repo" commit-tree "$extra_tree")
git -C "$repo" tag -fam 'drifted evidence' omp-context-evidence-v0.50.124 "$extra_commit"
extra_tag=$(git -C "$repo" rev-parse refs/tags/omp-context-evidence-v0.50.124)
if (cd "$repo" && run_helper "$temp/tree-drift" \
  OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA="$extra_tag" \
  OMP_CONTEXT_EVIDENCE_COMMIT_SHA="$extra_commit" \
  OMP_CONTEXT_EVIDENCE_TREE_SHA="$extra_tree"); then
  fail 'evidence tree with replayed policy bytes passed'
fi

parent=$(printf 'parent\n' | git -C "$repo" commit-tree "$tree")
child=$(printf 'child\n' | git -C "$repo" commit-tree "$tree" -p "$parent")
git -C "$repo" tag -fam 'parented evidence' omp-context-evidence-v0.50.124 "$child"
parented_tag=$(git -C "$repo" rev-parse refs/tags/omp-context-evidence-v0.50.124)
if (cd "$repo" && run_helper "$temp/parented" \
  OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA="$parented_tag" \
  OMP_CONTEXT_EVIDENCE_COMMIT_SHA="$child"); then
  fail 'parented evidence commit passed'
fi

artifact="$temp/auto"
printf 'canonical A23 candidate bytes' >"$artifact"
artifact_sha=$(shasum -a 256 "$artifact" | awk '{print $1}')
env COMPANION_RELEASE_TAG='v0.50.124' COMPANION_ARTIFACT="$artifact" \
  COMPANION_PLATFORM='darwin' COMPANION_ARCHITECTURE='arm64' OMP_CONTEXT_STATIC_POLICY_B64='e30' \
  OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256="$artifact_sha" bash "$binary_helper"
if env COMPANION_RELEASE_TAG='v0.50.124' COMPANION_ARTIFACT="$artifact" \
  COMPANION_PLATFORM='darwin' COMPANION_ARCHITECTURE='arm64' OMP_CONTEXT_STATIC_POLICY_B64='e30' \
  OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256="$(printf '0%.0s' {1..64})" \
  bash "$binary_helper"; then
  fail 'mismatched GoReleaser candidate digest passed'
fi

printf 'OMP context evidence hardening: PASS\n'
