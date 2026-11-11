#!/usr/bin/env bash
# Drive the measured coordinate move against a fixture measurement in a temp
# copy of the tree, one coordinate past whatever the tree pins now.
#
# The assertions are about what must move and what must not: the TO rows
# exist, the FROM rows survive, the incident comments and burn records are
# byte-identical, and every removed line names a coordinate token. The last
# check is what keeps this from passing vacuously: a rule that rewrites an
# unrelated line fails here even if every expected TO row is present.
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source_root=$(cd -- "$tests_dir/../../.." && pwd)
fail() { printf 'release coordinate autopilot: %s\n' "$1" >&2; exit 1; }
temp=$(mktemp -d "${TMPDIR:-/tmp}/coordinate-autopilot.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT

copy="$temp/repo"
mkdir -p "$copy/internal" "$copy/pkg" "$copy/scripts" "$copy/docs" "$copy/.github"
cp -R "$source_root/.github/workflows" "$copy/.github/"
cp -R "$source_root/internal/companionmanifest" "$copy/internal/"
cp -R "$source_root/pkg/companionmanifest" "$copy/pkg/"
cp -R "$source_root/scripts/companion-release" "$source_root/scripts/release-tools" "$copy/scripts/"
cp -R "$source_root/docs/runbooks" "$copy/docs/"
export GIT_AUTHOR_NAME=fixture GIT_AUTHOR_EMAIL=fixture@example.invalid
export GIT_COMMITTER_NAME=fixture GIT_COMMITTER_EMAIL=fixture@example.invalid
git -C "$copy" init --quiet
git -C "$copy" add -A
git -C "$copy" commit --quiet --no-verify -m fixture

# The current coordinate, read from the tree the same way a person would.
validator="$copy/scripts/companion-release/validate-source.sh"
coordinates="$copy/scripts/companion-release/verify-public-key-lineage-coordinates.sh"
bridge="$copy/scripts/companion-release/publish-homebrew-formula-bridge.sh"
from_tag=$(sed -nE "s/^      - '(v[0-9]+\.[0-9]+\.[0-9]+)'\$/\1/p" "$copy/.github/workflows/release.yaml")
[[ "$from_tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]] || fail 'cannot read the release trigger'
from_version=${from_tag#v}
to_tag="v${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
to_version=${to_tag#v}
from_phase=$(sed -nE "s/^  ${from_tag//./\\.}\) release_phase='(A[0-9]+)' ;;\$/\1/p" "$validator")
[[ "$from_phase" =~ ^A([0-9]+)$ ]] || fail "cannot read the ${from_tag} phase"
to_phase="A$((BASH_REMATCH[1] + 1))"
prev_phase=$(sed -nE "s/.*release_phase='${from_phase}' prior_phase='(A[0-9]+)'.*/\1/p" "$coordinates")
prev_commit=$(sed -nE "s/^readonly ${prev_phase}_COMMIT_SHA='([0-9a-f]{40})'\$/\1/p" "$coordinates")
prev_version=$(sed -nE "s/.*${prev_phase}_VERSION='([0-9.]+)'.*/\1/p" "$coordinates" | head -n 1)
prior_tap=$(sed -nE "s/^readonly PRIOR_TAP_COMMIT='([0-9a-f]{40})'\$/\1/p" "$bridge")
frozen=$(sed -nE "s/^readonly FROZEN_FORMULA_BLOB='([0-9a-f]{40})'\$/\1/p" "$bridge")
[[ -n "$prev_commit" && -n "$prior_tap" && -n "$frozen" ]] || fail 'cannot read the predecessor pins'

hex() { printf "$1"'%.0s' $(seq 1 "$2"); }
measurement() {
  jq -n --arg tag "$from_tag" --arg version "$from_version" --arg prev "$prev_commit" \
    --arg parent "${1:-$prior_tap}" --arg frozen "$frozen" \
    --arg commit "$(hex c 40)" --arg tree "$(hex d 40)" --arg object "$(hex e 40)" \
    --arg tap "$(hex b 40)" --arg cask "$(hex f 40)" '
    {schema_version:"autopus.adk_published_release_measurement.v1",
     repository:"autopus-ai/autopus-adk", tag:$tag, version:$version,
     release:{id:987654321, published_at:"2026-01-02T03:04:05Z", asset_count:15, latest:true},
     tag_object:$object, commit:$commit, tree:$tree, predecessor_commit:$prev,
     checksums_sha256:("1" * 64),
     archives:{darwin_amd64:("2" * 64), darwin_arm64:("3" * 64), linux_amd64:("4" * 64), linux_arm64:("5" * 64)},
     manifests:{darwin_amd64:("6" * 64), darwin_arm64:("7" * 64)},
     binaries:{darwin_amd64:("8" * 64), darwin_arm64:("9" * 64)},
     evidence:{tag:("omp-context-evidence-" + $tag), tag_object:("a" * 40), commit:("0" * 39 + "1")},
     homebrew:{tap_repository:"Insajin/homebrew-autopus", tap_head:$tap, tap_head_parent:$parent,
       cask_blob:$cask, formula_blob:$frozen}}'
}
measurement >"$temp/measurement.json"
measurement "$(hex 0 40)" >"$temp/broken-chain.json"

# Only the published check reaches gh; every record value is passed in.
mkdir -p "$temp/bin"
cat >"$temp/bin/gh" <<EOF
#!/usr/bin/env bash
[[ "\$1 \$2" == 'api repos/autopus-ai/autopus-adk/releases/tags/${from_tag}' ]] && exit 0
exit 1
EOF
chmod 0700 "$temp/bin/gh"
run_move() {
  (cd "$copy" && PATH="$temp/bin:$PATH" scripts/release-tools/advance-release-coordinate.sh \
    "$from_tag" "$from_phase" "$to_tag" "$to_phase" --measurements "$1" \
    --release-run 11 --release-conclusion success --canary-run 22 --canary-conclusion success \
    --ruleset-id 33 --ruleset-state sealed --tag-policy-id 44 --skip-line-check)
}

# A measurement that does not extend the pinned tap head is refused untouched.
if run_move "$temp/broken-chain.json" >"$temp/refused.log" 2>&1; then
  fail 'a measurement off the pinned tap chain was accepted'
fi
grep -qF 'does not build on the pinned PRIOR_TAP_COMMIT' "$temp/refused.log" || fail 'chain refusal message differs'
[[ -z "$(git -C "$copy" status --porcelain)" ]] || fail 'a refused move wrote files'

history_lines() {
  grep -hE 'ruleset was the 34th|published at 08:13:08|burned_A[0-9]+_tag|^contains "\$producer_receipt"' \
    "$copy/scripts/companion-release/verify-release-tag-ruleset.sh" \
    "$copy/scripts/companion-release/tests/release-tag-ruleset-pagination-test.sh" \
    "$copy/scripts/companion-release/tests/release-homebrew-hardening-test.sh" \
    "$copy/scripts/companion-release/tests/release-hardening-test.sh" \
    "$copy/internal/companionmanifest/release_lineage_phase_matrix_test.go"
}
history_lines >"$temp/history-before"
grep -E "^readonly ${prev_phase}_" "$coordinates" >"$temp/prev-declaration-before"
run_move "$temp/measurement.json" >"$temp/move.log" 2>&1 || { cat "$temp/move.log" >&2; fail 'the fixture move failed'; }

has() { grep -qF -- "$2" "$copy/$1" || fail "$1 lacks: $2"; }
lacks() { ! grep -qF -- "$2" "$copy/$1" || fail "$1 still has: $2"; }
has scripts/companion-release/validate-source.sh "  ${to_tag}) release_phase='${to_phase}' ;;"
has scripts/companion-release/validate-source.sh "  ${from_tag}) release_phase='${from_phase}' ;;"
has scripts/companion-release/validate-source.sh "readonly ${to_phase}_${from_phase}_ANCESTOR_SHA='$(hex c 40)'"
has scripts/companion-release/validate-source.sh "/${from_phase}/${to_phase} policy'"
has scripts/companion-release/verify-public-key-lineage-coordinates.sh "readonly ${from_phase}_RELEASE_ID='987654321'"
has scripts/companion-release/verify-public-key-lineage-coordinates.sh "${to_phase}_TAG='${to_tag}' ${to_phase}_VERSION='${to_version}'"
has scripts/companion-release/verify-public-key-lineage-coordinates.sh "release_phase='${to_phase}' prior_phase='${from_phase}'"
has scripts/companion-release/verify-public-key-lineage.sh "\"\$release_phase\" == '${to_phase}' ]]"
has scripts/companion-release/produce-public-key-receipt.sh "${to_tag} ${to_version} ${to_phase}"
has scripts/companion-release/produce-public-key-receipt.sh "${from_tag} ${from_version} ${from_phase}"
has .github/workflows/release.yaml "      - '${to_tag}'"
lacks .github/workflows/release.yaml "      - '${from_tag}'"
has .github/workflows/upgrade-canary.yaml "posix-upgrade-canary.sh live ${from_version} ${to_version}"
has .github/workflows/upgrade-canary.yaml "releases/987654321"
has scripts/release-tools/preflight-release.sh "predecessor_tag=\"\${2:-${from_tag}}\""
has scripts/companion-release/publish-homebrew-formula-bridge.sh "readonly PRIOR_TAP_COMMIT='$(hex b 40)'"
has scripts/companion-release/publish-homebrew-formula-bridge.sh "readonly PRIOR_CASK_BLOB='$(hex f 40)'"
has internal/companionmanifest/release_lineage_shipped_phases_test.go "phase: \"${to_phase}\", tag: \"${to_tag}\""
has internal/companionmanifest/release_lineage_phase_matrix_test.go "{name: \"${to_phase}\", tag: \"${to_tag}\""
has "docs/runbooks/release-${to_tag}.md" "# Release ${to_tag} (${to_phase})"
has "docs/runbooks/release-${to_tag}.md" '`987654321`'
has "docs/runbooks/release-${from_tag}.md" 'Release run `11` concluded `success`'
! grep -q '{{' "$copy/docs/runbooks/release-${to_tag}.md" || fail 'the new runbook kept a template placeholder'

# History: byte-identical, plus exactly the one new receipt row.
history_lines | grep -vxF "contains \"\$producer_receipt\" '${to_tag} ${to_version} ${to_phase}'" >"$temp/history-after"
cmp -s "$temp/history-before" "$temp/history-after" || fail 'a history line moved'
grep -E "^readonly ${prev_phase}_" "$coordinates" | cmp -s "$temp/prev-declaration-before" - ||
  fail "the ${prev_phase} declaration moved"

# Every removed line must name a coordinate token; anything else is collateral.
old_pins=$(sed -nE "s/^readonly ${prev_phase}_[A-Z0-9_]+='([0-9a-f]+|[0-9]+)'\$/\1/p" "$temp/prev-declaration-before" | paste -sd '|' -)
token="${from_version//./\\.}|${prev_version//./\\.}|${from_phase}([^0-9]|\$)|${prev_phase}([^0-9]|\$)|${prior_tap}|${old_pins}"
token="$token|$(git -C "$copy" show HEAD:scripts/companion-release/publish-homebrew-formula-bridge.sh |
  sed -nE "s/^readonly PRIOR_CASK_BLOB='([0-9a-f]{40})'\$/\1/p")"
removed=$(git -C "$copy" diff -U0 | grep -E '^-[^-]' || true)
[[ -n "$removed" ]] || fail 'the move removed no line at all'
collateral=$(grep -vE -- "$token" <<<"$removed" || true)
[[ -z "$collateral" ]] || fail "lines without a coordinate token changed:"$'\n'"$collateral"

# A second run is refused: the tree is no longer clean, and once committed the
# TO coordinate already exists.
if run_move "$temp/measurement.json" >/dev/null 2>&1; then fail 'a dirty tree was moved again'; fi
git -C "$copy" add -A
git -C "$copy" commit --quiet --no-verify -m moved
if run_move "$temp/measurement.json" >"$temp/again.log" 2>&1; then fail 'the same move ran twice'; fi
grep -qF 'already' "$temp/again.log" || fail 'the repeat refusal message differs'

printf 'release coordinate autopilot test: PASS (%s %s -> %s %s)\n' "$from_tag" "$from_phase" "$to_tag" "$to_phase"
