#!/usr/bin/env bash
# shellcheck disable=SC2016
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd -- "$tests_dir/.." && pwd)
repo=$(cd -- "$script_dir/../.." && pwd)
producer="$script_dir/produce.sh"
environment_gate="$script_dir/validate-environment.sh"
exec_smoke_package="$script_dir/execsmoke"
uid_isolation="$exec_smoke_package/uid_isolation.go"
artifact_staging="$exec_smoke_package/artifact_staging.go"
workflow="$repo/.github/workflows/release.yaml"
materializer="$script_dir/materialize-omp-release-canary.sh"
remover="$script_dir/remove-omp-release-canary.sh"
goreleaser_config="$repo/.goreleaser.yaml"

fail() {
  printf 'release exec smoke hardening test: %s\n' "$1" >&2
  exit 1
}

contains() {
  grep -Fq -- "$2" "$1" || fail "$1 missing $2"
}

not_contains() {
  ! grep -Fq -- "$2" "$1" || fail "$1 unexpectedly contains $2"
}

[[ -d "$exec_smoke_package" && ! -L "$exec_smoke_package" ]] \
  || fail 'execution smoke helper package is missing or unsafe'
contains "$environment_gate" 'COMPANION_EXEC_SMOKE_GATE'
contains "$environment_gate" "'companion execution smoke gate'"
contains "$producer" '"$COMPANION_EXEC_SMOKE_GATE"'
contains "$producer" '--artifact "$artifact_path"'
contains "$producer" '--expected-version "$COMPANION_VERSION"'
contains "$producer" '--architecture "$COMPANION_ARCHITECTURE"'
contains "$producer" '--timeout 45s'
contains "$producer" 'OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE OMP_CONTEXT_RELEASE_CANARY_ROOT'
contains "$producer" 'OMP_CONTEXT_RELEASE_CANARY_ROOT="$OMP_CONTEXT_RELEASE_CANARY_ROOT"'
contains "$workflow" 'Materialize exact arm64 OMP release canary'
contains "$workflow" '/usr/bin/sudo -n -u nobody /usr/bin/true'
contains "$workflow" 'run: scripts/companion-release/materialize-omp-release-canary.sh'
contains "$materializer" '/usr/bin/install -d -m 0755 -o root -g wheel'
contains "$materializer" '/usr/bin/install -d -m 0700 -o nobody -g nobody'
contains "$materializer" '/usr/bin/install -m 0555 -o root -g wheel'
contains "$workflow" 'Verify release nobody privilege boundary'
contains "$workflow" '[[ "$runner_uid" != "$nobody_uid" ]]'
contains "$workflow" '[[ "$isolated_uid" == "$nobody_uid" ]]'
contains "$workflow" 'companion-ed25519-private-key'
contains "$workflow" 'release-ecdsa-private-key'
contains "$workflow" 'keychain-password'
contains "$workflow" 'protected_modes=(600 600 600 600 600 600 700)'
contains "$workflow" '/bin/test "$permission" "$protected_path"'
contains "$materializer" '/usr/bin/sudo -n -u nobody /bin/test -x "$destination"'
not_contains "$workflow" '/usr/bin/test'
not_contains "$materializer" '/usr/bin/test'
contains "$workflow" '/usr/bin/sudo -n -u root /usr/bin/true >/dev/null 2>&1'
contains "$materializer" 'omp-canary-cleanup-identity'
contains "$remover" '[[ "$(/usr/bin/stat -f '"'"'%d:%i'"'"' /private/tmp)" == "$parent_identity" ]]'
contains "$workflow" 'Remove isolated OMP release canary'
contains "$workflow" 'run: scripts/companion-release/remove-omp-release-canary.sh'
contains "$workflow" 'if: always()'
contains "$materializer" 'https://github.com/can1357/oh-my-pi/releases/download/v17.2.7/omp-darwin-arm64'
contains "$materializer" 'cd2f47545cb3f8eb5e15c91bc9054d73967774652e020b432e294803d1b71ea0'
contains "$workflow" 'OMP_CONTEXT_RELEASE_CANARY_ROOT: /private/tmp/autopus-adk-omp-canary-${{ github.run_id }}-${{ github.run_attempt }}'
contains "$workflow" 'OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE="$OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE"'
contains "$uid_isolation" 'uid == targetUID'
contains "$uid_isolation" 'return validateArtifactAncestorUIDAccess(filepath.Dir(path), targetUID, targetGIDs, trustedOwnerUID)'
contains "$uid_isolation" 'for parent := path; ; parent = filepath.Dir(parent)'
contains "$uid_isolation" 'identityErr != nil || uid == targetUID'
contains "$uid_isolation" 'canaryTargetMode(info, targetUID, targetGIDs, 0o200, 0o020, 0o002)'
contains "$uid_isolation" 'policy.expectedGIDs, policy.runnerOwnerUID'
contains "$uid_isolation" 'uid != trustedOwnerUID || info.Mode()&os.ModeSticky == 0'
contains "$artifact_staging" 'destination := filepath.Join(isolation.root, "auto")'
contains "$artifact_staging" 'productionInstallPath = "/usr/bin/install"'
contains "$artifact_staging" '"-m", "0555", "-o", "root", "-g", "wheel"'
contains "$artifact_staging" 'staged signed artifact changed during execution smoke'
contains "$exec_smoke_package/main.go" 'stageSignedArtifact(artifact, isolation)'
contains "$exec_smoke_package/main.go" 'return staged.finish(smokeErr)'
if grep -Fq 'OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE' "$goreleaser_config"; then
  fail 'release canary executable leaked into the GoReleaser archive configuration'
fi

credentials_index=$(grep -n 'name: Prepare release credentials' "$workflow" | cut -d: -f1)
boundary_index=$(grep -n 'name: Verify release nobody privilege boundary' "$workflow" | cut -d: -f1)
release_index=$(grep -n 'name: Run GoReleaser' "$workflow" | cut -d: -f1)
(( credentials_index < boundary_index && boundary_index < release_index )) \
  || fail 'nobody privilege boundary probe is not between credential creation and signed canary execution'

identity_index=$(grep -n "Signature=adhoc" "$producer" | tail -1 | cut -d: -f1)
smoke_index=$(grep -n '"$COMPANION_EXEC_SMOKE_GATE"' "$producer" | tail -1 | cut -d: -f1)
manifest_index=$(grep -n 'manifest_sign_args=(' "$producer" | head -1 | cut -d: -f1)
(( identity_index < smoke_index && smoke_index < manifest_index )) \
  || fail 'execution smoke gate is not between final identity verification and manifest creation'

go test -count=1 "$repo/scripts/companion-release/execsmoke"

if [[ "$(uname -s)" == 'Darwin' ]]; then
  temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/release-exec-smoke-test.XXXXXX")
  public_dir=$(mktemp -d "$repo/.release-exec-smoke-artifact.XXXXXX")
  chmod 0700 "$public_dir"
  uid_root=''
  cleanup() {
    local status=$?
    rm -rf -- "$temp_dir" || status=$?
    rm -rf -- "$public_dir" || status=$?
    if [[ -n "$uid_root" && ( -e "$uid_root" || -L "$uid_root" ) ]]; then
      /usr/bin/sudo -n /bin/rm -rf -- "$uid_root" || status=$?
    fi
    return "$status"
  }
  trap cleanup EXIT
  gate="$temp_dir/exec-smoke-gate"
  artifact="$public_dir/auto"
  architecture=$(go env GOARCH)
  case "$architecture" in
    amd64|arm64) ;;
    *) fail "unsupported Darwin test architecture: $architecture" ;;
  esac
  go build -trimpath -o "$gate" "$repo/scripts/companion-release/execsmoke"
  go build -trimpath \
    -ldflags '-X github.com/insajin/autopus-adk/pkg/version.version=0.50.92' \
    -o "$artifact" "$repo/cmd/auto"
  gate_args=(--artifact "$artifact" --expected-version 0.50.92 \
    --architecture "$architecture" --timeout 30s)
  if [[ "$architecture" == 'arm64' ]]; then
    omp_command=$(command -v omp || true)
    if [[ -n "$omp_command" ]]; then
      omp_executable=$(realpath "$omp_command")
    else
      omp_executable=''
    fi
    if [[ -n "$omp_executable" && \
      "$(shasum -a 256 "$omp_executable" | awk '{print $1}')" == \
      'cd2f47545cb3f8eb5e15c91bc9054d73967774652e020b432e294803d1b71ea0' ]]; then
      if /usr/bin/sudo -n -u nobody /usr/bin/true >/dev/null 2>&1; then
        uid_root=$(mktemp -d '/private/tmp/release-exec-smoke-nobody.XXXXXX')
        /usr/bin/sudo -n /usr/sbin/chown root:wheel "$uid_root"
        /usr/bin/sudo -n /bin/chmod 0755 "$uid_root"
        /usr/bin/sudo -n /usr/bin/install -d -m 0700 -o nobody -g nobody \
          "$uid_root/home" "$uid_root/tmp"
        /usr/bin/sudo -n /usr/bin/install -m 0555 -o root -g wheel \
          "$omp_executable" "$uid_root/omp-darwin-arm64"
        OMP_CONTEXT_RELEASE_CANARY_ROOT="$uid_root" \
          OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE="$uid_root/omp-darwin-arm64" \
          "$gate" "${gate_args[@]}"
        [[ ! -e "$uid_root/auto" && ! -L "$uid_root/auto" ]] \
          || fail 'root-owned signed artifact staging was not removed'
        gate_args=()
      else
        if "$gate" "${gate_args[@]}" >/dev/null 2>&1; then
          fail 'arm64 gate passed without nobody UID isolation'
        fi
        gate_args=()
      fi
    else
      if "$gate" "${gate_args[@]}" >/dev/null 2>&1; then
        fail 'arm64 gate passed without the pinned OMP release canary'
      fi
      gate_args=()
    fi
  fi
  if (( ${#gate_args[@]} > 0 )); then
    "$gate" "${gate_args[@]}"
  fi
  if "$gate" --artifact "$artifact" --expected-version 0.50.88 \
    --architecture "$architecture" --timeout 30s >/dev/null 2>&1; then
    fail 'wrong expected version passed the execution smoke gate'
  fi
fi

printf 'release exec smoke hardening test: PASS\n'
