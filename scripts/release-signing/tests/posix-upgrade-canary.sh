#!/bin/sh

set -eu

TESTS_DIR=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "$TESTS_DIR/../../.." && pwd)
FIXTURE_ROOT="$REPO_ROOT/testdata/upgrade/v0.50.108"
FIXTURE_VERSION=0.50.109-canary
DEFAULT_PREVIOUS=0.50.108
DEFAULT_CANDIDATE=0.50.109

fail() {
    printf 'upgrade-canary: FAIL: %s\n' "$*" >&2
    exit 1
}

require_tool() {
    command -v "$1" >/dev/null 2>&1 || fail "required tool not found: $1"
}

assert_file() {
    [ -f "$1" ] || fail "missing file: $1"
}

assert_absent() {
    [ ! -e "$1" ] && [ ! -L "$1" ] || fail "legacy path remains: $1"
}

assert_contains() {
    grep -F -- "$2" "$1" >/dev/null 2>&1 || fail "$1 does not contain: $2"
}

assert_not_contains() {
    if grep -F -- "$2" "$1" >/dev/null 2>&1; then
        fail "$1 still contains: $2"
    fi
}

assert_version() {
    actual=$("$1" version --short 2>&1) || fail "version command failed: $actual"
    [ "$actual" = "$2" ] || fail "version $actual, expected $2"
}

validate_release_version() {
    value=$1
    case "$value" in
        ''|*[!0-9.]*|.*|*.|*..*) fail "invalid release version: $value" ;;
    esac
    after_major=${value#*.}
    after_minor=${after_major#*.}
    [ "$after_major" != "$value" ] &&
        [ "$after_minor" != "$after_major" ] &&
        [ "${after_minor#*.}" = "$after_minor" ] ||
        fail "release version must have three numeric parts: $value"
}

assert_legacy_paths() {
    assert_file "$PROJECT/.claude/skills/autopus/auto-go.md"
    assert_file "$PROJECT/.codex/prompts/auto.md"
    assert_file "$PROJECT/.codex/rules/autopus/context7-docs.md"
    assert_file "$PROJECT/.agents/skills/auto/SKILL.md"
}

assert_legacy_fixture() {
    assert_legacy_paths
    assert_contains "$PROJECT/.omp/config.yml" 'customDirectories:'
    assert_contains "$PROJECT/.omp/config.yml" '.agents/skills'
}

assert_public_previous_fixture() {
    assert_file "$PROJECT/.claude/skills/auto-go/SKILL.md"
    assert_file "$PROJECT/.codex/skills/codex-auto/SKILL.md"
    assert_contains "$PROJECT/.codex/config.toml" '[features.multi_agent_v2]'
    assert_file "$PROJECT/.omp/skills/auto/SKILL.md"
    assert_file "$PROJECT/.omp/commands/auto.md"
    assert_file "$PROJECT/.omp/agents/executor.md"
    assert_absent "$PROJECT/.omp/config.yml"
    assert_file "$PROJECT/.autopus/omp-manifest.json"
    assert_not_contains "$PROJECT/.autopus/omp-manifest.json" '".omp/config.yml"'
    assert_not_contains "$PROJECT/.autopus/omp-manifest.json" '.agents/skills/'
}

assert_current_surface() {
    assert_file "$PROJECT/.claude/skills/auto-go/SKILL.md"
    assert_file "$PROJECT/.codex/skills/codex-auto/SKILL.md"
    assert_contains "$PROJECT/.codex/config.toml" '[features.multi_agent_v2]'
    assert_file "$PROJECT/.omp/skills/auto/SKILL.md"
    assert_file "$PROJECT/.omp/commands/auto.md"
    assert_absent "$PROJECT/.omp/agents/executor.md"

    assert_absent "$PROJECT/.claude/skills/autopus/auto-go.md"
    assert_absent "$PROJECT/.codex/prompts/auto.md"
    assert_absent "$PROJECT/.codex/rules/autopus/context7-docs.md"
    assert_absent "$PROJECT/.agents/skills/auto/SKILL.md"

    assert_file "$PROJECT/.omp/config.yml"
    assert_contains "$PROJECT/.omp/config.yml" 'dark: canary-user'
    assert_not_contains "$PROJECT/.omp/config.yml" 'customDirectories:'
    assert_not_contains "$PROJECT/.omp/config.yml" '.agents/skills'
    assert_not_contains "$PROJECT/.omp/config.yml" '# AUTOPUS:BEGIN'
    assert_file "$PROJECT/.autopus/omp-manifest.json"
    assert_not_contains "$PROJECT/.autopus/omp-manifest.json" '".omp/config.yml"'
    assert_not_contains "$PROJECT/.autopus/omp-manifest.json" '.agents/skills/'
    cmp -s "$PROJECT/user-sentinel.txt" "$SENTINEL_EXPECTED" ||
        fail 'user sentinel changed during upgrade'
}

initialize_git_repo() {
    git -C "$PROJECT" init -q
    git -C "$PROJECT" config user.name 'Upgrade Canary'
    git -C "$PROJECT" config user.email 'upgrade-canary@example.invalid'
}

assert_real_commit_hooks() {
    assert_file "$PROJECT/.git/hooks/commit-msg"
    [ -x "$PROJECT/.git/hooks/commit-msg" ] || fail 'commit-msg hook is not executable'
    printf 'upgrade canary commit payload\n' > "$PROJECT/upgrade-canary-change.txt"
    git -C "$PROJECT" add .

    before=$(git -C "$PROJECT" rev-parse HEAD 2>/dev/null || printf 'unborn')
    if git -C "$PROJECT" commit -m 'test(upgrade): reject missing Lore trailers' \
        >"$RECEIPT_DIR/invalid-commit.log" 2>&1; then
        fail 'invalid Lore commit unexpectedly succeeded'
    fi
    after=$(git -C "$PROJECT" rev-parse HEAD 2>/dev/null || printf 'unborn')
    [ "$before" = "$after" ] || fail 'rejected commit changed HEAD'
    assert_contains "$RECEIPT_DIR/invalid-commit.log" 'Constraint'

    git -C "$PROJECT" commit \
        -m 'test(upgrade): admit release upgrade canary' \
        -m 'Constraint: preserve upgrade compatibility evidence' \
        -m '🐙 Autopus <noreply@autopus.co>' \
        >"$RECEIPT_DIR/valid-commit.log" 2>&1 || {
            cat "$RECEIPT_DIR/valid-commit.log" >&2
            fail 'valid Lore commit was rejected'
        }
    git -C "$PROJECT" log -1 --pretty=%B > "$RECEIPT_DIR/accepted-message.txt"
    assert_contains "$RECEIPT_DIR/accepted-message.txt" \
        'Constraint: preserve upgrade compatibility evidence'
    assert_contains "$RECEIPT_DIR/accepted-message.txt" \
        '🐙 Autopus <noreply@autopus.co>'
}

assert_doctor_json() {
    doctor_exit=0
    "$AUTO" doctor --dir "$PROJECT" --json > "$RECEIPT_DIR/doctor.json" 2> "$RECEIPT_DIR/doctor.stderr" ||
        doctor_exit=$?
    [ -s "$RECEIPT_DIR/doctor.json" ] || {
        cat "$RECEIPT_DIR/doctor.stderr" >&2
        fail 'doctor --json produced no report'
    }
    printf '%s\n' "$doctor_exit" > "$RECEIPT_DIR/doctor.exit"
    python3 - "$RECEIPT_DIR/doctor.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as stream:
    report = json.load(stream)
if report.get("schema_version") != "1.0.0":
    raise SystemExit("doctor schema_version is not 1.0.0")
if report.get("command") != "auto doctor":
    raise SystemExit("doctor command identity is missing")
config = report.get("data", {}).get("config", {})
if config.get("loaded") is not True:
    raise SystemExit("doctor config is not loaded")
checks = report.get("checks", [])
by_id = {item.get("id"): item for item in checks}
if by_id.get("doctor.config.autopus_yaml", {}).get("status") != "pass":
    raise SystemExit("doctor config check did not pass")
if by_id.get("doctor.hooks.configured", {}).get("status") != "pass":
    raise SystemExit("doctor hooks check did not pass")
blocked = ("doctor.config", "doctor.hooks", "doctor.platform.omp.validation")
for item in checks:
    check_id = item.get("id", "")
    if check_id.startswith(blocked) and item.get("status") == "fail":
        raise SystemExit("doctor upgrade compatibility failure: " + check_id)
PY
}

write_provenance() {
    receipt_status=${1:-attempt}
    executable_verified=false
    if [ "$receipt_status" = admitted ]; then executable_verified=true; fi
    os_name=$(uname -s)
    arch_name=$(uname -m)
    if [ "$MODE" = fixture ]; then
        cat > "$RECEIPT_DIR/provenance.json" <<EOF
{"mode":"fixture","status":"$receipt_status","proof":"candidate-source compatibility only","previous_fixture":"v0.50.108","candidate_source_version":"$NEW_VERSION","candidate_source_executable_verified":$executable_verified,"actual_previous_executable":false,"public_release_signatures":false,"os":"$os_name","arch":"$arch_name"}
EOF
    else
        cat > "$RECEIPT_DIR/provenance.json" <<EOF
{"mode":"live","status":"$receipt_status","proof":"public asset admission","previous_public_version":"$PREVIOUS_VERSION","candidate_public_version":"$NEW_VERSION","actual_release_executables_verified":$executable_verified,"public_signed_installers_verified":$executable_verified,"os":"$os_name","arch":"$arch_name"}
EOF
    fi
}

run_update_and_assertions() {
    AUTOPUS_GITHUB_TOKEN=${UPGRADE_CANARY_GITHUB_TOKEN:-} "$AUTO" update --dir "$PROJECT" --local --yes \
        > "$RECEIPT_DIR/update.log" 2>&1 || {
            cat "$RECEIPT_DIR/update.log" >&2
            fail 'candidate update failed'
        }
    assert_current_surface
    assert_real_commit_hooks
    assert_doctor_json
    write_provenance admitted
}

[ "$#" -ge 1 ] || fail 'usage: posix-upgrade-canary.sh fixture CANDIDATE | live [PREVIOUS CANDIDATE]'
MODE=$1
shift

for tool in cp git grep mktemp python3 uname; do
    require_tool "$tool"
done
unset AUTOPUS_GITHUB_TOKEN GITHUB_TOKEN GH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_TOKEN \
    OPENAI_API_KEY GEMINI_API_KEY GOOGLE_API_KEY OMP_ACCESS_TOKEN PI_PROVIDER_TOKEN \
    CLAUDE_CODE_OAUTH_TOKEN 2>/dev/null || true

WORK=$(mktemp -d "${TMPDIR:-/tmp}/autopus-upgrade-canary.XXXXXX")
trap 'rm -rf -- "$WORK"' EXIT HUP INT TERM
HOME_DIR="$WORK/home"
INSTALL_DIR="$WORK/install"
PROJECT="$WORK/project"
SENTINEL_EXPECTED="$WORK/user-sentinel.expected"
RECEIPT_DIR=${UPGRADE_CANARY_RECEIPT_DIR:-"$WORK/receipt"}
mkdir -p "$HOME_DIR" "$INSTALL_DIR" "$PROJECT" "$RECEIPT_DIR"
printf 'AUTOPUS-UPGRADE-CANARY-USER-SENTINEL\n' > "$SENTINEL_EXPECTED"
PATH="$INSTALL_DIR:/usr/bin:/bin"
HOME=$HOME_DIR
SHELL=/bin/sh
export HOME INSTALL_DIR PATH SHELL

case "$MODE" in
    fixture)
        [ "$#" -eq 1 ] || fail 'fixture mode requires exactly one candidate binary path'
        [ -x "$1" ] || fail "candidate binary is not executable: $1"
        cp "$1" "$INSTALL_DIR/auto"
        chmod 0755 "$INSTALL_DIR/auto"
        AUTO="$INSTALL_DIR/auto"
        NEW_VERSION=$FIXTURE_VERSION
        write_provenance
        assert_version "$AUTO" "$NEW_VERSION"
        cp -R "$FIXTURE_ROOT/." "$PROJECT/"
        initialize_git_repo
        assert_legacy_fixture
        run_update_and_assertions
        ;;
    live)
        [ "$#" -le 2 ] || fail 'live mode accepts only previous and candidate versions'
        PREVIOUS_VERSION=${1:-$DEFAULT_PREVIOUS}
        NEW_VERSION=${2:-$DEFAULT_CANDIDATE}
        validate_release_version "$PREVIOUS_VERSION"
        validate_release_version "$NEW_VERSION"
        write_provenance
        AUTO="$INSTALL_DIR/auto"
        initialize_git_repo
        # The production installer remains authoritative for signature verification and runtime timeout admission.
        (
            cd "$REPO_ROOT"
            VERSION=$PREVIOUS_VERSION sh ./install.sh
        ) > "$RECEIPT_DIR/install-previous.log" 2>&1 || {
            cat "$RECEIPT_DIR/install-previous.log" >&2
            fail 'previous public signed installer failed'
        }
        assert_version "$AUTO" "$PREVIOUS_VERSION"
        "$AUTO" init --dir "$PROJECT" --project upgrade-canary-live \
            --platforms claude-code,codex,omp --no-review-gate --yes \
            > "$RECEIPT_DIR/old-init.log" 2>&1 || {
                cat "$RECEIPT_DIR/old-init.log" >&2
                fail 'previous release init failed'
            }
        assert_public_previous_fixture
        printf 'AUTOPUS-UPGRADE-CANARY-USER-SENTINEL\n' > "$PROJECT/user-sentinel.txt"
        printf 'theme:\n  dark: canary-user\n' > "$PROJECT/.omp/config.yml"
        (
            cd "$REPO_ROOT"
            VERSION=$NEW_VERSION sh ./install.sh
        ) > "$RECEIPT_DIR/install-candidate.log" 2>&1 || {
            cat "$RECEIPT_DIR/install-candidate.log" >&2
            fail 'candidate public signed installer failed'
        }
        assert_version "$AUTO" "$NEW_VERSION"
        run_update_and_assertions
        ;;
    *) fail "unknown mode: $MODE" ;;
esac

printf 'upgrade-canary: PASS mode=%s receipt=%s\n' "$MODE" "$RECEIPT_DIR"
