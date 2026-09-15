#!/usr/bin/env bash
# Advance every pinned release coordinate from one tag to the next.
#
# The coordinate is not one constant. It is pinned in workflow triggers, ref
# guards, asset name literals, a ruleset gate, the current-release verifier, the
# Homebrew bridge, and a phase resolver. release-hardening-test.sh guards them
# together, so missing one turns into a red test rather than a broken release —
# but only after someone notices. Doing them by hand across seven files is how a
# coordinate drifts.
#
# Two kinds of pin, and telling them apart is the whole job:
#   replace  - names the release currently being shipped
#   append   - accumulates every shipped release, never rewritten
set -euo pipefail
umask 077

fail() { printf 'advance coordinate: %s\n' "$1" >&2; exit 1; }
usage() {
  printf '%s\n' 'usage: advance-release-coordinate.sh FROM_TAG FROM_PHASE TO_TAG TO_PHASE' >&2
  exit 64
}
[[ $# -eq 4 ]] || usage
readonly from_tag=$1 from_phase=$2 to_tag=$3 to_phase=$4
[[ "$from_phase" =~ ^A[0-9]+$ ]] || fail 'FROM_PHASE is not a phase label'
readonly from_version="${from_tag#v}" to_version="${to_tag#v}"

[[ "$from_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'FROM_TAG is not a release tag'
[[ "$to_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'TO_TAG is not a release tag'
[[ "$to_phase" =~ ^A[0-9]+$ ]] || fail 'TO_PHASE is not a phase label'
[[ "$from_tag" != "$to_tag" ]] || fail 'FROM_TAG and TO_TAG are equal'

repo_root=$(git rev-parse --show-toplevel) || fail 'not a git repository'
cd "$repo_root"
readonly repository='autopus-ai/autopus-adk'

# Replace: each of these names the release being shipped, so exactly one value
# is correct at a time.
# The list was built by enumerating every v-tag reference under scripts/ and
# .github/ and dropping the ones that name a predecessor or a historical pin. An
# earlier draft held six entries and missed six more; the guard tests caught it,
# but only after the coordinate was half moved. If a release fails on "not exact
# <tag>", the missing file belongs here.
readonly -a replace_targets=(
  '.github/workflows/release.yaml'
  '.github/workflows/homebrew-formula-bridge-recovery.yaml'
  '.github/workflows/upgrade-canary.yaml'
  'scripts/companion-release/verify-release-tag-ruleset.sh'
  'scripts/companion-release/verify-current-release.sh'
  'scripts/companion-release/publish-homebrew-formula-bridge.sh'
  'scripts/companion-release/prepare-release.sh'
  'scripts/companion-release/publish-release-coordinates.sh'
  'scripts/companion-release/verify-omp-context-release-binary.sh'
  'scripts/companion-release/verify-omp-context-evidence-tag.sh'
  'scripts/companion-release/verify-release-prep-lock.sh'
  'scripts/companion-release/build-omp-context-candidate.sh'
  'scripts/companion-release/tests/testdata/mock-release-prep-gh.sh'
  'scripts/companion-release/tests/testdata/mock-tap-gh.sh'
  # These carry the coordinate as data rather than as a trigger, and both were
  # missed on the v0.50.112 -> v0.50.113 move: the phase case arm in
  # validate-source.sh and the cosign identity. A missed file here is silent
  # drift that only the Go contract tests catch, which is far too late.
  'scripts/companion-release/verify-current-release-signatures.sh'
  'scripts/release-tools/preflight-release.sh'
)

# Append-only history, deliberately NOT substituted. The lineage coordinates
# keep one declaration per phase whose predecessor pins are measured from the
# immutable release, so a new phase cannot be derived from a version string.
#
# Substituting this file is safe only while the FROM coordinate was never
# published. It was safe for the burned v0.50.112 and it destroyed history the
# first time it ran against the published v0.50.113: A24_TAG became v0.50.114
# and the shipped coordinate disappeared. So the published state decides, and it
# is measured rather than assumed.
#
# produce-public-key-receipt.sh is also append-only but needs no measurement,
# so its own block below appends the triple and refuses out-of-order rows.
readonly -a history_files=(
  'scripts/companion-release/validate-source.sh'
  'scripts/companion-release/verify-public-key-lineage-coordinates.sh'
)

# Phase-keyed accumulators: these files list every phase a rule applies to
# ('A27' || 'A28' ...), so they carry no version string and the history check
# above cannot see them. A published FROM keeps its entry and gains TO; an
# unpublished FROM moves in place. A28 was appended by hand once because this
# file was in neither list.
readonly -a phase_list_files=(
  'scripts/companion-release/verify-public-key-lineage.sh'
)

# Test files are deliberately absent from the list above. Each of them mixes
# shipped-release pins with predecessor references in the same file — the
# accumulate assertion for the previous phase, the predecessor Cask digests, a
# fixture that needs a tag which already exists in git — so a whole-file
# substitution corrupts as much as it fixes. That was observed, not theorised:
# an earlier run rewrote `'v0.50.111 0.50.111 A23'` into the A24 row and
# silently removed the guard that the older row survived.
readonly -a review_targets=(
  'scripts/companion-release/tests/release-prep-hardening-test.sh'
  'scripts/companion-release/tests/release-homebrew-hardening-test.sh'
  'scripts/companion-release/tests/release-hardening-test.sh'
  'scripts/companion-release/tests/release-omp-context-evidence-hardening-test.sh'
  'scripts/companion-release/tests/release-prep-environment-inheritance-test.sh'
)
for target in "${replace_targets[@]}"; do
  [[ -f "$target" && ! -L "$target" ]] || fail "missing or unsafe target $target"
done
for target in "${history_files[@]}" "${phase_list_files[@]}"; do
  [[ -f "$target" && ! -L "$target" ]] || fail "missing or unsafe target $target"
done

# Measure, do not assume. A published FROM means its rows are proof someone can
# still verify, so they stay and the new phase is added beside them. An
# unpublished FROM was a burned attempt and its row is a placeholder to move.
from_published=0
if gh api "repos/${repository}/releases/tags/${from_tag}" \
  --jq 'select(.draft == false and (.assets | length) > 0) | .id' >/dev/null 2>&1; then
  from_published=1
fi
if [[ "$from_published" -eq 1 ]]; then
  printf '  history  %s is published; its rows stay and %s is added beside them\n' \
    "$from_tag" "$to_tag"
  for target in "${history_files[@]}"; do
    if grep -qF -- "$to_version" "$target"; then
      printf '  present  %s already declares %s\n' "$target" "$to_version"
      continue
    fi
    fail "$(printf '%s\n' \
      "${target} has no ${to_version} declaration and ${from_tag} is published." \
      "  Add the new phase by hand: its predecessor pins are measured from the" \
      "  immutable release, not derived from a version string. See" \
      "  docs/runbooks/omp-pin-advance.md for the measurement pattern.")"
  done
  for target in "${phase_list_files[@]}"; do
    if grep -qF -- "'${to_phase}'" "$target"; then
      printf '  present  %s already lists %s\n' "$target" "$to_phase"
      continue
    fi
    fail "$(printf '%s\n' \
      "${target} does not list ${to_phase} and ${from_tag} is published." \
      "  Append '${to_phase}' beside '${from_phase}' in each accumulating phase list;" \
      "  the ${from_phase} entry stays because its release is still verifiable.")"
  done
else
  printf '  history  %s was never published; moving its rows in place\n' "$from_tag"
  for target in "${history_files[@]}"; do
    perl -pi -e "s/\Q${from_tag}\E/${to_tag}/g; s/\Q${from_version}\E/${to_version}/g" "$target"
    printf '  updated  %s\n' "$target"
  done
  for target in "${phase_list_files[@]}"; do
    perl -pi -e "s/'\Q${from_phase}\E'/'${to_phase}'/g" "$target"
    printf '  updated  %s\n' "$target"
  done
fi

changed=0
for target in "${replace_targets[@]}"; do
  before=$(shasum -a 256 "$target" | awk '{print $1}')
  # Two forms are safe to rewrite: the tag in refs and triggers, and the bare
  # version in asset name literals and RELEASE_VERSION constants.
  #
  # Phase labels are deliberately not rewritten. A blunt A23-to-A24 pass looks
  # right and is wrong: it also hits predecessor references, historical comments,
  # and accumulate assertions that must keep naming the older phase. Those are
  # reported below for review instead.
  perl -pi -e "s/\Q${from_tag}\E/${to_tag}/g; s/\Q${from_version}\E/${to_version}/g" "$target"
  after=$(shasum -a 256 "$target" | awk '{print $1}')
  if [[ "$before" != "$after" ]]; then
    printf '  updated  %s\n' "$target"
    changed=$((changed + 1))
  else
    printf '  no pin   %s\n' "$target"
  fi
done

# Append: the receipt resolver keeps every shipped triple. Rewriting an old row
# would make a published receipt unverifiable.
readonly receipt='scripts/companion-release/produce-public-key-receipt.sh'
[[ -f "$receipt" && ! -L "$receipt" ]] || fail "missing or unsafe target $receipt"
readonly receipt_row="${to_tag} ${to_version} ${to_phase}"
if grep -qF -- "$receipt_row" "$receipt"; then
  printf '  present  %s already lists %s\n' "$receipt" "$receipt_row"
else
  grep -qF -- "${from_tag} ${from_version}" "$receipt" ||
    fail "receipt resolver does not list ${from_tag}; refusing to append out of order"
  perl -pi -e "s/^(\Q${from_tag} ${from_version}\E .*)\$/\$1\n${receipt_row}/" "$receipt"
  grep -qF -- "$receipt_row" "$receipt" || fail 'receipt row append failed'
  printf '  appended %s to %s\n' "$receipt_row" "$receipt"
  changed=$((changed + 1))
fi

# The phase map and its ancestor pin are deliberately not automated. The
# ancestor is the predecessor's source commit, which has to be read from the
# published tag rather than derived from a version string, and getting it wrong
# would let a release ship without containing its predecessor.
readonly validator='scripts/companion-release/validate-source.sh'
if grep -qF -- "${to_tag}) release_phase='${to_phase}'" "$validator"; then
  printf '  present  %s maps %s to %s\n' "$validator" "$to_tag" "$to_phase"
else
  printf '\n%s\n' "MANUAL: ${validator} does not map ${to_tag} to ${to_phase}."
  printf '%s\n' "  Add the phase row, an ancestor constant set to"
  printf '%s\n' "  \$(git rev-parse ${from_tag}^{commit}), and an ancestor branch."
  printf '%s\n' "  The tag-to-phase map fails closed, so the release is refused until then."
fi

for target in "${replace_targets[@]}" "$receipt"; do
  case "$target" in
    *.sh) bash -n "$target" || fail "$target no longer parses" ;;
  esac
done

# Phase labels need eyes, not sed. Each hit is either the shipped release (bump
# it), a predecessor reference (leave it), or a historical statement (leave it).
printf '\n%s\n' "REVIEW: test files naming ${from_tag} or ${from_phase}, classify each hit"
for target in "${review_targets[@]}"; do
  while IFS= read -r hit; do
    printf '  %s:%s\n' "$target" "$hit"
  done < <(grep -n -e "$from_tag" -e "$from_phase" "$target" 2>/dev/null || true)
done

printf '\n%s\n' "REVIEW: sources still naming ${from_phase}, classify each hit"
phase_hits=0
for target in "${replace_targets[@]}" "$receipt" "$validator"; do
  if grep -n -- "$from_phase" "$target" >/dev/null 2>&1; then
    while IFS= read -r hit; do
      printf '  %s:%s\n' "$target" "$hit"
      phase_hits=$((phase_hits + 1))
    done < <(grep -n -- "$from_phase" "$target")
  fi
done
[[ "$phase_hits" -gt 0 ]] || printf '  none\n'

printf '\n%s\n' "advanced ${changed} file(s) from ${from_tag} to ${to_tag} (${to_phase})"
printf '%s\n' 'Run scripts/companion-release/tests/release-hardening-test.sh to confirm the'
printf '%s\n' 'coordinate moved together, then preflight-release.sh for the rest.'
