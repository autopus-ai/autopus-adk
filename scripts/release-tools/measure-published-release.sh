#!/usr/bin/env bash
# Measure every pin a coordinate move needs from one published release.
#
# A coordinate move pins the predecessor by value: release id, tag object,
# commit, tree, archive and manifest digests, the evidence tag, and the
# Homebrew tap head the next Cask builds on. Each of those was measured by hand
# for every move so far, and each measurement had to be checked the same way:
# bytes against the API digest against checksums.txt, the tag against the R2
# trust root, the commit against its ancestry. This script does exactly that,
# read-only, and prints one JSON document. Any mismatch stops it; nothing is
# guessed and nothing on GitHub is written.
#
# usage: measure-published-release.sh FROM_TAG [--predecessor-commit SHA]
#
# Network reads: the GitHub REST API through gh, and a tree-less fetch of the
# public repository into a temporary bare repository. Nothing touches the
# caller's checkout.
set -euo pipefail
umask 077

fail() { printf 'measure published release: %s\n' "$1" >&2; exit 1; }
usage() {
  printf '%s\n' 'usage: measure-published-release.sh FROM_TAG [--predecessor-commit SHA]' >&2
  exit 64
}
[[ $# -ge 1 ]] || usage
readonly tag=$1
shift
predecessor_commit=''
while (($#)); do
  case "$1" in
    --predecessor-commit) [[ $# -ge 2 ]] || usage; predecessor_commit=$2; shift 2 ;;
    *) usage ;;
  esac
done
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail 'FROM_TAG is not a release tag'
[[ -z "$predecessor_commit" || "$predecessor_commit" =~ ^[0-9a-f]{40}$ ]] ||
  fail '--predecessor-commit is not a 40-hex commit'
readonly version="${tag#v}" hex40='^[0-9a-f]{40}$' hex64='^[0-9a-f]{64}$'
readonly repository="${MEASURE_REPOSITORY:-autopus-ai/autopus-adk}"
readonly release_author_id=204883817
readonly expected_asset_count=15
for tool in gh git jq shasum tar base64 ssh-keygen; do
  command -v "$tool" >/dev/null || fail "${tool} is unavailable"
done

repo_root=$(git rev-parse --show-toplevel) || fail 'not a git repository'
readonly bridge="$repo_root/scripts/companion-release/publish-homebrew-formula-bridge.sh"
readonly trust_root="$repo_root/scripts/companion-release/release-tag-signing-2026-q3-r2.pub"
readonly trust_fingerprint="$repo_root/scripts/companion-release/release-tag-signing-2026-q3-r2.fingerprint"
for file in "$bridge" "$trust_root" "$trust_fingerprint"; do
  [[ -f "$file" && ! -L "$file" ]] || fail "missing or unsafe ${file#"$repo_root"/}"
done
bridge_pin() {
  local value
  value=$(sed -nE "s/^readonly $1='([^']+)'\$/\1/p" "$bridge")
  [[ -n "$value" ]] || fail "cannot read $1 from the Homebrew publisher"
  printf '%s\n' "$value"
}
tap_repository=$(bridge_pin TAP_REPOSITORY)
tap_branch=$(bridge_pin TAP_BRANCH)
cask_path=$(bridge_pin CASK_PATH)
formula_path=$(bridge_pin FORMULA_PATH)
frozen_formula_blob=$(bridge_pin FROZEN_FORMULA_BLOB)

work=$(mktemp -d "${TMPDIR:-/tmp}/measure-release.XXXXXX") || fail 'cannot create workspace'
trap 'rm -rf -- "$work"' EXIT
api() { gh api "$@" </dev/null; }
sha256_of() { shasum -a 256 "$1" | awk '{print $1}'; }

# Release metadata: one immutable, non-draft release with the full asset set.
release_json="$work/release.json"
api "repos/${repository}/releases/tags/${tag}" >"$release_json" ||
  fail "cannot read release ${tag}"
jq -e --arg tag "$tag" --argjson author "$release_author_id" \
  --argjson count "$expected_asset_count" '
  .tag_name == $tag and .draft == false and .prerelease == false and
  .immutable == true and .author.id == $author and
  (.target_commitish | test("^[0-9a-f]{40}$")) and
  (.id | type == "number" and . > 0) and (.assets | length) == $count
' "$release_json" >/dev/null ||
  fail "${tag} is not one immutable published release with ${expected_asset_count} assets"
release_id=$(jq -r '.id' "$release_json")
published_at=$(jq -er '.published_at' "$release_json")
commit=$(jq -r '.target_commitish' "$release_json")
latest_id=$(api "repos/${repository}/releases/latest" --jq '.id') || latest_id=''

# Tag object, commit, and tree, read through the API and again through Git.
tag_object=$(api "repos/${repository}/git/ref/tags/${tag}" \
  --jq 'select(.object.type == "tag") | .object.sha') || fail "cannot read ref ${tag}"
[[ "$tag_object" =~ $hex40 ]] || fail "${tag} is not an annotated tag"
api "repos/${repository}/git/tags/${tag_object}" | jq -e --arg tag "$tag" --arg commit "$commit" '
  .tag == $tag and .object.type == "commit" and .object.sha == $commit
' >/dev/null || fail 'tag object does not name the release commit'
tree=$(api "repos/${repository}/commits/${commit}" --jq '.commit.tree.sha') ||
  fail 'cannot read the release commit'
[[ "$tree" =~ $hex40 ]] || fail 'release tree is malformed'

readonly evidence_tag="omp-context-evidence-${tag}"
git_dir="$work/repo.git"
git init --quiet --bare "$git_dir"
git -C "$git_dir" fetch --quiet --no-tags --filter=tree:0 \
  "https://github.com/${repository}.git" \
  "+refs/tags/${tag}:refs/tags/${tag}" \
  "+refs/tags/${evidence_tag}:refs/tags/${evidence_tag}" \
  '+refs/heads/main:refs/heads/main' </dev/null || fail 'cannot fetch the release refs'
[[ "$(git -C "$git_dir" rev-parse "refs/tags/${tag}")" == "$tag_object" ]] ||
  fail 'Git tag object differs from the API'
tag_header=$(git -C "$git_dir" cat-file tag "$tag_object" | sed -n '1,3p')
[[ "$tag_header" == "$(printf 'object %s\ntype commit\ntag %s' "$commit" "$tag")" ]] ||
  fail 'annotated tag object, type, or name headers differ'
[[ "$(git -C "$git_dir" cat-file commit "$commit" | sed -n 's/^tree //p' | head -n 1)" == "$tree" ]] ||
  fail 'Git tree differs from the API'

# The tag must verify against the committed R2 trust root, as validate-source.sh does.
expected_fingerprint=$(<"$trust_fingerprint")
[[ "$(ssh-keygen -lf "$trust_root" -E sha256 | awk '{print $2}')" == "$expected_fingerprint" ]] ||
  fail 'R2 trust root fingerprint differs'
awk 'NF >= 2 { print "autopus-adk-release-tag " $1 " " $2; exit }' "$trust_root" >"$work/signers"
git -C "$git_dir" -c gpg.format=ssh -c gpg.ssh.allowedSignersFile="$work/signers" \
  verify-tag "refs/tags/${tag}" >/dev/null 2>&1 || fail 'release tag signature does not verify against R2'

git -C "$git_dir" merge-base --is-ancestor "$commit" refs/heads/main ||
  fail 'release commit is not an ancestor of main'
if [[ -n "$predecessor_commit" ]]; then
  git -C "$git_dir" merge-base --is-ancestor "$predecessor_commit" "$commit" ||
    fail 'predecessor commit is not an ancestor of the release commit'
fi

evidence_object=$(git -C "$git_dir" rev-parse "refs/tags/${evidence_tag}")
[[ "$(git -C "$git_dir" cat-file -t "$evidence_object")" == 'tag' ]] ||
  fail 'evidence tag is not annotated'
evidence_commit=$(git -C "$git_dir" rev-parse "refs/tags/${evidence_tag}^{commit}")

# Asset bytes: download into this workspace, hash, compare with the API digest
# and with checksums.txt. A digest that agrees with only one of them stops here.
download() {
  local name=$1 destination="$work/assets/$1" asset_id digest
  asset_id=$(jq -er --arg name "$name" '[.assets[] | select(.name == $name)] |
    if length == 1 then .[0].id else error("asset") end' "$release_json") ||
    fail "${name} is missing or ambiguous"
  digest=$(jq -er --arg name "$name" '.assets[] | select(.name == $name) | .digest' "$release_json")
  [[ ! -e "$destination" ]] || fail "${name} destination already exists"
  api -H 'Accept: application/octet-stream' \
    "repos/${repository}/releases/assets/${asset_id}" >"$destination" || fail "cannot download ${name}"
  [[ -s "$destination" ]] || fail "${name} is empty"
  [[ "sha256:$(sha256_of "$destination")" == "$digest" ]] || fail "${name} bytes differ from the API digest"
}
mkdir -m 0700 "$work/assets"
download checksums.txt
checksums_sha=$(sha256_of "$work/assets/checksums.txt")
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  name="autopus-adk_${version}_${platform}.tar.gz"
  download "$name"
  printf -v "archive_$platform" %s "$(sha256_of "$work/assets/$name")"
  digest_var="archive_$platform"
  listed=$(awk -v name="$name" '$2 == name { print $1 }' "$work/assets/checksums.txt")
  [[ "$listed" == "${!digest_var}" ]] || fail "${name} differs from its checksums.txt entry"
done

# Manifests travel inside the darwin archives; each must describe this release
# and the auto binary beside it.
for arch in amd64 arm64; do
  out="$work/darwin-$arch"; mkdir -m 0700 "$out"
  tar -xzf "$work/assets/autopus-adk_${version}_darwin_${arch}.tar.gz" -C "$out" \
    adk-companion-manifest.json auto || fail "cannot extract the darwin ${arch} manifest"
  [[ -f "$out/adk-companion-manifest.json" && ! -L "$out/adk-companion-manifest.json" &&
     -f "$out/auto" && ! -L "$out/auto" ]] || fail "darwin ${arch} archive members are unsafe"
  printf -v "manifest_$arch" %s "$(sha256_of "$out/adk-companion-manifest.json")"
  binary=$(sha256_of "$out/auto"); printf -v "binary_$arch" %s "$binary"
  jq -e --arg version "$version" --arg arch "$arch" --arg digest "sha256:$binary" \
    --arg provenance "github-actions:${repository}@${commit}" '
    .version == $version and .platform == "darwin" and .architecture == $arch and
    .artifact_digest == $digest and .build_provenance == $provenance
  ' "$out/adk-companion-manifest.json" >/dev/null || fail "darwin ${arch} manifest does not describe ${tag}"
done

# Homebrew: the tap head the next Cask builds on, its Cask blob, and the
# frozen Formula. The Cask has to carry this release's four archive digests.
tap_head=$(api "repos/${tap_repository}/git/ref/heads/${tap_branch}" --jq '.object.sha') ||
  fail 'cannot read the tap head'
[[ "$tap_head" =~ $hex40 ]] || fail 'tap head is malformed'
api "repos/${tap_repository}/git/commits/${tap_head}" >"$work/tap-head.json" || fail 'cannot read the tap commit'
tap_parent=$(jq -er '.parents | if length == 1 then .[0].sha else error("parents") end' "$work/tap-head.json") ||
  fail 'tap head is not a single-parent commit'
tap_message=$(jq -er '.message' "$work/tap-head.json")
[[ "$tap_message" == "Publish signed Cask for ${tag}" ]] || fail "tap head is not the ${tag} Cask commit"
api "repos/${tap_repository}/contents/${cask_path}?ref=${tap_head}" >"$work/cask.json" || fail 'cannot read the Cask'
cask_blob=$(jq -er '.sha' "$work/cask.json")
jq -er '.content' "$work/cask.json" | tr -d '\n' | base64 -d >"$work/cask.rb" 2>/dev/null ||
  fail 'cannot decode the Cask'
[[ "$(git hash-object "$work/cask.rb")" == "$cask_blob" ]] || fail 'Cask bytes differ from their blob'
grep -qF "version \"${version}\"" "$work/cask.rb" || fail "Cask does not name ${version}"
for platform in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
  digest_var="archive_$platform"
  grep -qF "${!digest_var}" "$work/cask.rb" || fail "Cask lacks the ${platform} digest"
done
formula_blob=$(api "repos/${tap_repository}/contents/${formula_path}?ref=${tap_head}" --jq '.sha') ||
  fail 'cannot read the Formula'
[[ "$formula_blob" == "$frozen_formula_blob" ]] || fail 'Formula blob is no longer the frozen one'

jq -n --arg repository "$repository" --arg tag "$tag" --arg version "$version" \
  --argjson release_id "$release_id" --arg published_at "$published_at" \
  --argjson asset_count "$expected_asset_count" --argjson latest "$([[ "$latest_id" == "$release_id" ]] && echo true || echo false)" \
  --arg tag_object "$tag_object" --arg commit "$commit" --arg tree "$tree" \
  --arg predecessor "$predecessor_commit" --arg checksums "$checksums_sha" \
  --arg da "$archive_darwin_amd64" --arg dr "$archive_darwin_arm64" \
  --arg la "$archive_linux_amd64" --arg lr "$archive_linux_arm64" \
  --arg ma "$manifest_amd64" --arg mr "$manifest_arm64" \
  --arg ba "$binary_amd64" --arg br "$binary_arm64" \
  --arg evidence_tag "$evidence_tag" --arg evidence_object "$evidence_object" \
  --arg evidence_commit "$evidence_commit" --arg tap_repository "$tap_repository" \
  --arg tap_head "$tap_head" --arg tap_parent "$tap_parent" --arg cask_blob "$cask_blob" \
  --arg formula_blob "$formula_blob" --arg measured_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" '
  {schema_version:"autopus.adk_published_release_measurement.v1",
   repository:$repository, tag:$tag, version:$version, measured_at:$measured_at,
   release:{id:$release_id, published_at:$published_at, asset_count:$asset_count, latest:$latest},
   tag_object:$tag_object, commit:$commit, tree:$tree,
   predecessor_commit:(if $predecessor == "" then null else $predecessor end),
   checksums_sha256:$checksums,
   archives:{darwin_amd64:$da, darwin_arm64:$dr, linux_amd64:$la, linux_arm64:$lr},
   manifests:{darwin_amd64:$ma, darwin_arm64:$mr},
   binaries:{darwin_amd64:$ba, darwin_arm64:$br},
   evidence:{tag:$evidence_tag, tag_object:$evidence_object, commit:$evidence_commit},
   homebrew:{tap_repository:$tap_repository, tap_head:$tap_head, tap_head_parent:$tap_parent,
     cask_blob:$cask_blob, formula_blob:$formula_blob}}'
