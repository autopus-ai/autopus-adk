#!/usr/bin/env bash
set -euo pipefail
umask 077

readonly evidence_tag='omp-context-evidence-v0.50.119'
readonly report_name='omp-context-promotion-report.v1.json'
readonly attestation_name='omp-context-promotion-attestation.v2.json'

fail() { printf 'OMP context evidence tag: %s\n' "$1" >&2; exit 1; }
[[ $# == 1 ]] || fail 'usage: verify-omp-context-evidence-tag.sh OUTPUT_DIR'
readonly output_dir=$1
for name in OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA OMP_CONTEXT_EVIDENCE_COMMIT_SHA \
  OMP_CONTEXT_EVIDENCE_TREE_SHA OMP_CONTEXT_EVIDENCE_REPORT_SHA256 \
  OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256
do
  value=${!name-}
  if [[ "$name" == *_SHA256 ]]; then
    [[ "$value" =~ ^[0-9a-f]{64}$ ]] || fail "exact pin ${name} is missing or malformed"
  else
    [[ "$value" =~ ^[0-9a-f]{40}$ ]] || fail "exact pin ${name} is missing or malformed"
  fi
done
[[ ! -e "$output_dir" && ! -L "$output_dir" ]] || fail 'output directory already exists'
install -m 0700 -d "$output_dir"
readonly tag_ref="refs/tags/${evidence_tag}"
[[ "$(git cat-file -t "$tag_ref")" == 'tag' ]] || fail 'evidence tag is not annotated'
[[ "$(git rev-parse --verify "$tag_ref")" == "$OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA" ]] \
  || fail 'evidence tag object differs from exact pin'
[[ "$(git rev-parse --verify "${tag_ref}^{commit}")" == "$OMP_CONTEXT_EVIDENCE_COMMIT_SHA" ]] \
  || fail 'evidence commit differs from exact pin'
[[ "$(git rev-parse --verify "${tag_ref}^{tree}")" == "$OMP_CONTEXT_EVIDENCE_TREE_SHA" ]] \
  || fail 'evidence tree differs from exact pin'
parents=$(git rev-list --parents -n 1 "$OMP_CONTEXT_EVIDENCE_COMMIT_SHA") \
  || fail 'cannot inspect evidence commit ancestry'
[[ "$parents" == "$OMP_CONTEXT_EVIDENCE_COMMIT_SHA" ]] \
  || fail 'evidence commit is not a zero-parent orphan'
tree_listing=$(git ls-tree "$OMP_CONTEXT_EVIDENCE_TREE_SHA") \
  || fail 'cannot inspect evidence tree'
[[ "$(printf '%s\n' "$tree_listing" | wc -l | tr -d '[:space:]')" == '2' ]] \
  || fail 'evidence tree does not contain exactly two entries'
for name in "$report_name" "$attestation_name"; do
  entry=$(printf '%s\n' "$tree_listing" | awk -F '\t' -v name="$name" '$2 == name {print $1}')
  [[ "$entry" =~ ^100644\ blob\ [0-9a-f]{40}$ ]] \
    || fail "evidence entry ${name} is not a regular 100644 blob"
  git cat-file blob "${entry##* }" >"$output_dir/$name" \
    || fail "cannot materialize evidence entry ${name}"
  chmod 0600 "$output_dir/$name"
done
if command -v shasum >/dev/null 2>&1; then sha=(shasum -a 256); else sha=(sha256sum); fi
report_sha=$("${sha[@]}" "$output_dir/$report_name" | awk '{print $1}')
attestation_sha=$("${sha[@]}" "$output_dir/$attestation_name" | awk '{print $1}')
[[ "$report_sha" == "$OMP_CONTEXT_EVIDENCE_REPORT_SHA256" ]] \
  || fail 'report bytes differ from exact pin'
[[ "$attestation_sha" == "$OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256" ]] \
  || fail 'attestation bytes differ from exact pin'
