#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'rotation authority test: %s\n' "$1" >&2; exit 1; }
repo=$(cd -- "${BASH_SOURCE[0]%/*}/../../.." && pwd -P)
release="$repo/scripts/companion-release"
source_authority="$release/rotation-authority"
materializer="$release/materialize-key-rotation-authority.sh"
rotation_ruleset="$release/verify-rotation-ref-ruleset.sh"
for tool in cmp dd git jq openssl sed ssh-keygen stat; do
  command -v "$tool" >/dev/null || fail "$tool is unavailable"
done
temp=$(mktemp -d "${TMPDIR:-/tmp}/rotation-authority-test.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT
mkdir -m 0700 "$temp/authority" "$temp/pins" "$temp/state" "$temp/bin"
cat >"$temp/bin/go" <<EOF
#!/usr/bin/env bash
printf called >'$temp/state/go-called'
exit 99
EOF
chmod 0700 "$temp/bin/go"
PATH="$temp/bin:$PATH"
cp "$source_authority/verify-rotation.sh" "$temp/authority/verify-rotation.sh"
cp "$source_authority/adk-key-rotation-authority.v1.json" "$temp/authority/adk-key-rotation-authority.v1.json"
cp "$release/release-tag-signing-2026-q3-r2.pub" "$temp/pins/tag.pub"
cp "$release/release-tag-signing-2026-q3-r2.fingerprint" "$temp/pins/tag.fingerprint"
cp "$release/omp-context-promotion-2026-q3-k3.pub" "$temp/pins/promotion.pub"

openssl genpkey -algorithm ED25519 -out "$temp/a0.pem" >/dev/null 2>&1
openssl pkey -in "$temp/a0.pem" -pubout -outform DER -out "$temp/a0.der"
dd if="$temp/a0.der" of="$temp/a0.raw" bs=1 skip=12 count=32 2>/dev/null
test_public=$(openssl base64 -A -in "$temp/a0.raw")
production_public='1IqFilCntaMPUxg7ndOZnyy6Lj1NBQXkXBJp3rEu6kI='
[[ ${#test_public} -eq 44 && "$test_public" == *= ]] || fail 'ephemeral A0 public key is malformed'
sed "s|$production_public|$test_public|g" "$temp/authority/verify-rotation.sh" >"$temp/verifier.patched"
mv "$temp/verifier.patched" "$temp/authority/verify-rotation.sh"
test_verifier_digest=$(openssl dgst -sha256 "$temp/authority/verify-rotation.sh")
test_verifier_sha=${test_verifier_digest##* }
printf '%s' "$(jq -c --arg key "$test_public" --arg sha "$test_verifier_sha" \
  '.channel_public_key=$key | .verifier_sha256=$sha' \
  "$temp/authority/adk-key-rotation-authority.v1.json")" >"$temp/policy.patched"
mv "$temp/policy.patched" "$temp/authority/adk-key-rotation-authority.v1.json"
chmod 0700 "$temp/authority/verify-rotation.sh"
verifier="$temp/authority/verify-rotation.sh"
source_commit='c2a50831c925a4b08d55991039ed3b1113392888'
source_tree='f1c26be5d6f126997f728a9a20d47768f536b3f1'
issued=$(jq -nr 'now - 60 | strftime("%Y-%m-%dT%H:%M:%SZ")')
expires=$(jq -nr 'now + 3600 | strftime("%Y-%m-%dT%H:%M:%SZ")')

render_document() {
  local from=$1 until=$2 destination=$3
  printf '%s' "$(jq -cn --arg issued "$from" --arg expires "$until" \
    --arg commit "$source_commit" --arg tree "$source_tree" \
    '{schema_version:"adk-key-rotation.v1",channel:"stable",repository:"Insajin/autopus-adk",
      bridge_tag:"v0.50.109",release_mode:"canonical-full-bridge",source_commit:$commit,
      source_tree:$tree,issued_at:$issued,expires_at:$expires,channel_key_id:"adk-channel-2026-q3-a0",
      previous_tag_fingerprint:"SHA256:bhW+YA+FZ6G4d9Z8BM/eBss6l0I/fcVmV7k986GupK0",
      next_tag_public_key:"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPKdXtl0E+TcLmC94idkTgtM5XUA5UqP9An0vNFp0FlY",
      next_tag_fingerprint:"SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ",
      next_promotion_key_id:"omp-context-promotion-2026-q3-k3",
      next_promotion_public_key:"YkTuNcfWGTLgTglPmZq/Dj4OXwcoUwnkM2ExIGIz+jM=",
      next_promotion_public_key_sha256:"2a9b41dec1330f65937d9b25b20967cb29fd9209c722ce5fe1a9afd6ca45b937"}')" >"$destination"
}
sign_document() {
  local document=$1 signature=$2
  { printf 'autopus.adk-channel.key-rotation.v1\000'; openssl base64 -A -in "$document" | openssl base64 -d -A; } >"$temp/message"
  openssl pkeyutl -sign -inkey "$temp/a0.pem" -rawin -in "$temp/message" -out "$signature"
}
run_verifier() {
  local command=$1 document=$2 signature=$3
  "$verifier" "$command" --document "$document" --signature "$signature" \
    --source-commit "$source_commit" --source-tree "$source_tree" \
    --next-tag-public-key "$temp/pins/tag.pub" --next-tag-fingerprint "$temp/pins/tag.fingerprint" \
    --next-promotion-public-key "$temp/pins/promotion.pub"
}
reject() {
  local label=$1
  shift
  if "$@" >"$temp/rejected.out" 2>/dev/null; then fail "$label was accepted"; fi
}
reject_signed_filter() {
  local label=$1 filter=$2
  printf '%s' "$(jq -c "$filter" "$temp/valid.json")" >"$temp/rejected.json"
  sign_document "$temp/rejected.json" "$temp/rejected.sig"
  reject "$label" run_verifier verify-rotation-historical "$temp/rejected.json" "$temp/rejected.sig"
}
check_historical_case() {
  local label=$1 from=$2 until=$3 historical_result=$4
  render_document "$from" "$until" "$temp/time.json"
  sign_document "$temp/time.json" "$temp/time.sig"
  reject "$label active" run_verifier verify-rotation "$temp/time.json" "$temp/time.sig"
  if [[ "$historical_result" == pass ]]; then
    run_verifier verify-rotation-historical "$temp/time.json" "$temp/time.sig" >"$temp/time.out"
    cmp -s "$temp/time.json" "$temp/time.out" || fail "$label historical output differs"
  else
    reject "$label historical" run_verifier verify-rotation-historical "$temp/time.json" "$temp/time.sig"
  fi
}

render_document "$issued" "$expires" "$temp/valid.json"
sign_document "$temp/valid.json" "$temp/valid.sig"
run_verifier verify-rotation "$temp/valid.json" "$temp/valid.sig" >"$temp/verified.json"
cmp -s "$temp/valid.json" "$temp/verified.json" || fail 'success did not emit exact document bytes'
for mutation in \
  'schema|.schema_version="wrong"' 'channel|.channel="beta"' 'repository|.repository="other/repo"' \
  'bridge tag|.bridge_tag="v0.50.108"' 'release mode|.release_mode="other"' \
  'source commit|.source_commit="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"' \
  'source tree|.source_tree="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' \
  'A0 key ID|.channel_key_id="other-key"' 'old fingerprint|.previous_tag_fingerprint="SHA256:wrong"' \
  'R2 public|.next_tag_public_key="ssh-ed25519 AAAA"' 'R2 fingerprint|.next_tag_fingerprint="SHA256:wrong"' \
  'K3 ID|.next_promotion_key_id="other-key"' 'K3 public|.next_promotion_public_key="AAAA"' \
  'K3 SHA|.next_promotion_public_key_sha256="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
do reject_signed_filter "${mutation%%|*}" "${mutation#*|}"; done

expired_from=$(jq -nr 'now - 7200 | strftime("%Y-%m-%dT%H:%M:%SZ")')
expired_until=$(jq -nr 'now - 3600 | strftime("%Y-%m-%dT%H:%M:%SZ")')
future_from=$(jq -nr 'now + 3600 | strftime("%Y-%m-%dT%H:%M:%SZ")')
future_until=$(jq -nr 'now + 7200 | strftime("%Y-%m-%dT%H:%M:%SZ")')
long_until=$(jq -nr 'now + 90000 | strftime("%Y-%m-%dT%H:%M:%SZ")')
check_historical_case expired "$expired_from" "$expired_until" pass
check_historical_case future "$future_from" "$future_until" pass
check_historical_case overlong "$issued" "$long_until" fail
check_historical_case reversed "$expires" "$issued" fail
check_historical_case malformed '2026-01-01T00:00:00+00:00' "$expires" fail

printf '%s\n' "$(<"$temp/valid.json")" >"$temp/noncanonical.json"
sign_document "$temp/noncanonical.json" "$temp/noncanonical.sig"
reject whitespace run_verifier verify-rotation-historical "$temp/noncanonical.json" "$temp/noncanonical.sig"
raw=$(<"$temp/valid.json")
printf '%s' '{"schema_version":"adk-key-rotation.v1",'"${raw#\{}" >"$temp/noncanonical.json"
sign_document "$temp/noncanonical.json" "$temp/noncanonical.sig"
reject duplicate run_verifier verify-rotation-historical "$temp/noncanonical.json" "$temp/noncanonical.sig"
reject_signed_filter unknown '.unknown="value"'
reject_signed_filter missing 'del(.channel)'
printf '%s' "$(jq -c '{channel:.channel,schema_version:.schema_version} + .' "$temp/valid.json")" >"$temp/noncanonical.json"
sign_document "$temp/noncanonical.json" "$temp/noncanonical.sig"
reject reorder run_verifier verify-rotation-historical "$temp/noncanonical.json" "$temp/noncanonical.sig"
cp "$temp/valid.sig" "$temp/bad.sig"; printf x >>"$temp/bad.sig"
reject signature run_verifier verify-rotation-historical "$temp/valid.json" "$temp/bad.sig"
openssl base64 -A -in "$temp/valid.json" | openssl base64 -d -A >"$temp/message"
openssl pkeyutl -sign -inkey "$temp/a0.pem" -rawin -in "$temp/message" -out "$temp/bad.sig"
reject domain run_verifier verify-rotation-historical "$temp/valid.json" "$temp/bad.sig"
for pin in tag.pub tag.fingerprint promotion.pub; do
  cp "$temp/pins/$pin" "$temp/pins/$pin.good"; printf x >>"$temp/pins/$pin"
  reject "$pin tamper" run_verifier verify-rotation-historical "$temp/valid.json" "$temp/valid.sig"
  mv "$temp/pins/$pin.good" "$temp/pins/$pin"
done
cp "$temp/authority/adk-key-rotation-authority.v1.json" "$temp/policy.good"
printf '%s' "$(jq -c '.channel_public_key="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="' "$temp/policy.good")" \
  >"$temp/authority/adk-key-rotation-authority.v1.json"
reject 'A0 public policy tamper' run_verifier verify-rotation-historical "$temp/valid.json" "$temp/valid.sig"
mv "$temp/policy.good" "$temp/authority/adk-key-rotation-authority.v1.json"
reject duplicate-option "$verifier" verify-rotation --document "$temp/valid.json" --document "$temp/valid.json"
reject candidate-coordinate "$verifier" verify-rotation-historical --document "$temp/valid.json" \
  --signature "$temp/valid.sig" --source-commit aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --source-tree "$source_tree" --next-tag-public-key "$temp/pins/tag.pub" \
  --next-tag-fingerprint "$temp/pins/tag.fingerprint" \
  --next-promotion-public-key "$temp/pins/promotion.pub"

real_git=$(command -v git)
git -C "$temp" init -q authority-repo
git -C "$temp/authority-repo" config user.name authority-test
git -C "$temp/authority-repo" config user.email authority-test@example.invalid
cp "$source_authority/"* "$temp/authority-repo/"
chmod 0644 "$temp/authority-repo/adk-key-rotation-authority.v1.json"
chmod 0755 "$temp/authority-repo/verify-rotation.sh"
git -C "$temp/authority-repo" add .
canonical_tree=$(git -C "$temp/authority-repo" write-tree)
canonical_commit=$(printf canonical | git -C "$temp/authority-repo" commit-tree "$canonical_tree")
authority_ref='refs/heads/release-key-rotation-authority-v2'
git -C "$temp/authority-repo" update-ref "$authority_ref" "$canonical_commit"
printf '%s' "$canonical_commit" >"$temp/state/repository"; cp "$temp/state/repository" "$temp/state/environment"; cp "$temp/state/repository" "$temp/state/protected"
printf enabled >"$temp/state/ruleset"

cat >"$temp/bin/git" <<EOF
#!/usr/bin/env bash
[[ -z "\${GH_TOKEN-}" && -z "\${GITHUB_TOKEN-}" &&
   "\${GIT_TERMINAL_PROMPT-}" == 0 && "\${GIT_ASKPASS-}" == /usr/bin/false ]] || exit 90
args=(); for arg in "\$@"; do
  [[ "\$arg" != 'https://github.com/autopus-ai/autopus-adk.git' ]] || arg='$temp/authority-repo'
  args+=("\$arg")
done
exec '$real_git' "\${args[@]}"
EOF
cat >"$temp/bin/gh" <<EOF
#!/usr/bin/env bash
state='$temp/state'
printf '%s\n' "\$*" >>"\$state/gh-calls"
[[ ! -f "\$state/force-403" ]] || exit 1
if [[ "\$1 \$2" == 'variable get' ]]; then
  if [[ " \$* " == *' --env '* ]]; then if [[ "\$3" == ADK_PROTECTED_KEY_ROTATION_AUTHORITY_COMMIT ]]; then cat "\$state/protected"; else cat "\$state/environment"; fi; else cat "\$state/repository"; fi
elif [[ "\$1" == api && "\$*" == *'/rulesets/991'* ]]; then
  printf '%s\n' '{"name":"autopus-key-rotation-authority-v2","target":"branch","enforcement":"active","conditions":{"ref_name":{"exclude":[],"include":["refs/heads/release-key-rotation-authority-v2"]}},"bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],"rules":[{"type":"creation"},{"type":"deletion"},{"type":"update"}]}'
elif [[ "\$1" == api && "\$*" == *'/rulesets/992'* ]]; then
  printf '%s\n' '{"name":"autopus-v0.50.109-rotation-ref-authority","target":"branch","enforcement":"active","conditions":{"ref_name":{"exclude":[],"include":["refs/heads/release-key-rotation-v0.50.109"]}},"bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],"rules":[{"type":"creation"},{"type":"deletion"},{"type":"update"}]}'
elif [[ "\$1" == api && "\$*" == *'--paginate'* && -f "\$state/ruleset" ]]; then
  printf '%s\n' '[[{"id":991,"name":"autopus-key-rotation-authority-v2","target":"branch"}]]'
elif [[ "\$1" == api ]]; then
  printf '%s\n' '[{"id":992,"name":"autopus-v0.50.109-rotation-ref-authority","target":"branch"}]'
else exit 1
fi
EOF
cat >"$temp/bin/curl" <<EOF
#!/usr/bin/env bash
state='$temp/state'
[[ -z "\${GH_TOKEN-}" && -z "\${GITHUB_TOKEN-}" && "\${1-}" == '--disable' &&
   " \$* " != *' Authorization:'* ]] || exit 91
printf '%s\n' "\$*" >>"\$state/curl-calls"
url=\${!#}
source='autopus-ai/autopus-adk'; [[ ! -f "\$state/public-bad" ]] || source='Other/repository'
if [[ "\$url" == */rulesets/991 ]]; then
  printf '{"source_type":"Repository","source":"%s","name":"autopus-key-rotation-authority-v2","target":"branch","enforcement":"active","conditions":{"ref_name":{"exclude":[],"include":["refs/heads/release-key-rotation-authority-v2"]}},"rules":[{"type":"creation"},{"type":"deletion"},{"type":"update"}]}\n' "\$source"
elif [[ "\$url" == */rulesets/992 ]]; then
  printf '{"source_type":"Repository","source":"%s","name":"autopus-v0.50.109-rotation-ref-authority","target":"branch","enforcement":"active","conditions":{"ref_name":{"exclude":[],"include":["refs/heads/release-key-rotation-v0.50.109"]}},"rules":[{"type":"creation"},{"type":"deletion"},{"type":"update"}]}\n' "\$source"
else
  printf '%s\n' '[{"id":991,"name":"autopus-key-rotation-authority-v2","target":"branch"},{"id":992,"name":"autopus-v0.50.109-rotation-ref-authority","target":"branch"}]'
fi
EOF
chmod 0700 "$temp/bin/"*
mock_path="$temp/bin:$PATH"
receipt=$(GH_TOKEN=operator-token PATH="$mock_path" "$materializer" "$temp/output")
jq -e --arg commit "$canonical_commit" '.assertion_mode=="strict" and
  .authority_ref=="refs/heads/release-key-rotation-authority-v2" and
  .authority_commit==$commit and (.verifier_sha256|test("^[0-9a-f]{64}$")) and
  (.policy_sha256|test("^[0-9a-f]{64}$"))' <<<"$receipt" >/dev/null || fail 'strict receipt differs'
mode() { local value; if value=$(stat -f '%Lp' "$1" 2>/dev/null); then printf '%s' "$value"; else stat -c '%a' "$1"; fi; }
[[ "$(mode "$temp/output/verify-rotation.sh")" == 700 &&
   "$(mode "$temp/output/adk-key-rotation-authority.v1.json")" == 600 ]] || fail 'output modes differ'
output_entries=("$temp/output/"*)
[[ ${#output_entries[@]} -eq 2 && -f "$temp/output/verify-rotation.sh" &&
   -f "$temp/output/adk-key-rotation-authority.v1.json" ]] || fail 'output pair differs'
[[ ! -e "$temp/state/go-called" ]] || fail 'candidate Go verifier was called'
rm -f "$temp/state/gh-calls" "$temp/state/curl-calls"
public_receipt=$(GH_TOKEN=must-not-authenticate GITHUB_TOKEN=must-not-authenticate \
  PATH="$mock_path" "$materializer" --public "$canonical_commit" "$temp/output-public")
jq -e --arg commit "$canonical_commit" '.assertion_mode=="public" and .authority_commit==$commit' \
  <<<"$public_receipt" >/dev/null || fail 'public receipt differs'
[[ ! -e "$temp/state/gh-calls" && "$(wc -l <"$temp/state/curl-calls" | tr -d ' ')" == 6 ]] ||
  fail 'public materialization did not remain on the anonymous API path'
rm -f "$temp/state/curl-calls"
GH_TOKEN=must-not-authenticate GITHUB_TOKEN=must-not-authenticate PATH="$mock_path" \
  "$rotation_ruleset" --public
[[ ! -e "$temp/state/gh-calls" && "$(wc -l <"$temp/state/curl-calls" | tr -d ' ')" == 2 ]] ||
  fail 'public rotation ruleset verification used an authenticated path'

materializer_rejects() { local label=$1; reject "$label" env PATH="$mock_path" "$materializer" "$temp/out-$RANDOM"; }
touch "$temp/state/force-403"; rm -f "$temp/state/curl-calls"
materializer_rejects 'strict 403 without fallback'
[[ ! -e "$temp/state/curl-calls" ]] || fail 'strict 403 fell back to public assertions'
rm -f "$temp/state/gh-calls"
reject 'strict rotation ruleset 403 without fallback' env PATH="$mock_path" "$rotation_ruleset"
[[ -e "$temp/state/gh-calls" && ! -e "$temp/state/curl-calls" ]] ||
  fail 'strict rotation ruleset 403 fell back to public assertions'
rm "$temp/state/force-403"
touch "$temp/state/public-bad"
reject 'wrong public repository source' env PATH="$mock_path" "$materializer" \
  --public "$canonical_commit" "$temp/output-public-bad"
rm "$temp/state/public-bad"
reject 'malformed public authority commit' env PATH="$mock_path" "$materializer" \
  --public not-a-commit "$temp/output-public-malformed"
printf '%040d' 0 >"$temp/state/environment"; materializer_rejects 'environment mismatch'; cp "$temp/state/repository" "$temp/state/environment"
printf '%040d' 1 >"$temp/state/protected"; materializer_rejects 'distinct protected mismatch'; cp "$temp/state/repository" "$temp/state/protected"
rm "$temp/state/ruleset"; materializer_rejects 'missing ruleset'; printf enabled >"$temp/state/ruleset"
publish_tree() {
  local message=$1 parent=${2-} tree commit
  git -C "$temp/authority-repo" add -A
  tree=$(git -C "$temp/authority-repo" write-tree)
  if [[ -n "$parent" ]]; then commit=$(printf '%s' "$message" | git -C "$temp/authority-repo" commit-tree "$tree" -p "$parent")
  else commit=$(printf '%s' "$message" | git -C "$temp/authority-repo" commit-tree "$tree"); fi
  git -C "$temp/authority-repo" update-ref "$authority_ref" "$commit"
  printf '%s' "$commit" >"$temp/state/repository"; cp "$temp/state/repository" "$temp/state/environment"; cp "$temp/state/repository" "$temp/state/protected"
}
restore_tree() { git -C "$temp/authority-repo" checkout -q "$canonical_tree" -- .; rm -f "$temp/authority-repo/extra"; }
printf extra >"$temp/authority-repo/extra"; publish_tree extra; materializer_rejects 'extra file'; restore_tree
chmod 0644 "$temp/authority-repo/verify-rotation.sh"; publish_tree verifier-mode; materializer_rejects 'wrong verifier mode'; restore_tree
chmod 0755 "$temp/authority-repo/adk-key-rotation-authority.v1.json"; publish_tree policy-mode; materializer_rejects 'wrong policy mode'; restore_tree
rm "$temp/authority-repo/adk-key-rotation-authority.v1.json"; ln -s verify-rotation.sh "$temp/authority-repo/adk-key-rotation-authority.v1.json"
publish_tree symlink; materializer_rejects symlink; restore_tree
printf '\n' >>"$temp/authority-repo/adk-key-rotation-authority.v1.json"; publish_tree policy-bytes
materializer_rejects 'noncanonical policy'; restore_tree
printf '\n' >>"$temp/authority-repo/verify-rotation.sh"; publish_tree verifier-bytes
materializer_rejects 'noncanonical verifier'; restore_tree
publish_tree parent "$canonical_commit"; materializer_rejects 'non-orphan commit'; restore_tree
publish_tree canonical-restored
repacked=$(<"$temp/state/repository"); printf '%s' "$canonical_commit" >"$temp/state/repository"; cp "$temp/state/repository" "$temp/state/environment"; cp "$temp/state/repository" "$temp/state/protected"
materializer_rejects repack
printf '%s' "$repacked" >"$temp/state/repository"; cp "$temp/state/repository" "$temp/state/environment"; cp "$temp/state/repository" "$temp/state/protected"
git -C "$temp/authority-repo" update-ref "$authority_ref" "$canonical_commit"
materializer_rejects 'ref swap'
printf '%s\n' 'rotation authority tests: PASS'
