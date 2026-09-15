#!/bin/sh
set -e
umask 077

# autopus-adk 설치 스크립트
# 사용법: curl -fsSL https://get.autopus.co | sh
#
# 옵션 (환경변수):
#   INSTALL_DIR   — 설치 경로 (기본: /usr/local/bin)
#   VERSION       — 특정 버전 지정 (기본: 최신)

REPO="autopus-ai/autopus-adk"
BINARY="auto"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
ALIAS="autopus"
SIGNING_FLOOR="0.50.73"
VERIFIER_URL="https://raw.githubusercontent.com/${REPO}/main/scripts/release-signing/verify-checksums-v1.sh"
VERIFIER_SHA256="4c93682b88dad53c547d9d4b180fb5f3ead37f5aa0cf0ab05187fe1107ad38bb"
RUNTIME_HELPER_URL="https://raw.githubusercontent.com/${REPO}/main/scripts/install-runtime-v1.sh"
RUNTIME_HELPER_SHA256="05797a0d62f02687f416f8e1e640543bf48fae74971ff34af0c49650123abf8c"

# 색상 출력
info()  { printf '\033[1;34m%s\033[0m\n' "$1"; }
ok()    { printf '\033[1;32m%s\033[0m\n' "$1"; }
err()   { printf '\033[1;31m%s\033[0m\n' "$1" >&2; exit 1; }

path_contains_dir() {
    target="$1"
    case ":$PATH:" in
        *":$target:"*) return 0 ;;
        *) return 1 ;;
    esac
}

shell_rc_file() {
    case "${SHELL##*/}" in
        zsh) echo "~/.zshrc" ;;
        bash) echo "~/.bashrc" ;;
        *) echo "~/.profile" ;;
    esac
}

print_path_hint() {
    rc_file="$(shell_rc_file)"
    echo "  설치된 명령어:"
    echo "    auto"
    echo "    ${ALIAS}  # auto alias"
    if path_contains_dir "$INSTALL_DIR"; then
        echo "  PATH 확인: ${INSTALL_DIR}"
        return
    fi
    echo ""
    echo "  현재 셸 PATH에 ${INSTALL_DIR} 가 없습니다."
    echo "  새 셸에서 사용하려면 PATH에 추가하세요:"
    echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
    echo "  영구 적용:"
    echo "    echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ${rc_file}"
}

print_next_steps() {
    echo "  다음 단계:"
    echo "    ${BINARY} init"
    echo "      현재 프로젝트를 초기화합니다."
    echo "      설치된 AI 코딩 CLI를 감지해 autopus.yaml과 플랫폼별 하네스 파일을 생성합니다."
    echo ""
    echo "    ${BINARY} update --self"
    echo "      auto CLI 바이너리 자체를 최신 릴리즈로 업데이트합니다."
    echo ""
    echo "    ${BINARY} update"
    echo "      현재 프로젝트의 규칙, 스킬, 에이전트, 설정 파일을 최신 템플릿으로 갱신합니다."
    echo ""
    echo "  권장 순서:"
    echo "    1. 새 프로젝트면 ${BINARY} init"
    echo "    2. 새 릴리즈를 받았으면 ${BINARY} update --self"
    echo "    3. 그 다음 프로젝트 안에서 ${BINARY} update"
}

# OS 감지
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*)
            err "Windows 네이티브 환경은 지원하지 않습니다.
  현재 지원 OS: macOS, Linux
  Windows 사용자는 WSL2를 통해 설치할 수 있습니다:
  https://learn.microsoft.com/windows/wsl/install" ;;
        *)
            err "지원하지 않는 OS입니다: $(uname -s)
  현재 지원 OS: macOS, Linux" ;;
    esac
}

# 아키텍처 감지
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)   echo "amd64" ;;
        arm64|aarch64)  echo "arm64" ;;
        *)
            err "지원하지 않는 아키텍처입니다: $(uname -m)
  현재 지원 아키텍처: x86_64 (amd64), arm64 (aarch64)" ;;
    esac
}

# 최신 버전 조회
get_latest_version() {
    if command -v curl > /dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/'
    elif command -v wget > /dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/'
    else
        err "curl 또는 wget이 필요합니다"
    fi
}

# 다운로드
download() {
    url="$1"
    dest="$2"
    if command -v curl > /dev/null 2>&1; then
        curl -fsSL "$url" -o "$dest"
    elif command -v wget > /dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        return 1
    fi
}

version_supports_signing() {
    printf '%s\n' "$1" | awk -F. '
        NF != 3 { exit 1 }
        {
            for (i=1; i<=3; i++) if ($i !~ /^[0-9][0-9]*$/) exit 1
            major=$1+0; minor=$2+0; patch=$3+0
            if (major > 0 || (major == 0 && (minor > 50 || (minor == 50 && patch >= 73)))) exit 0
            exit 1
        }'
}

# SHA256 체크섬 검증
verify_checksum() {
    archive="$1"
    expected_checksum="$2"
    failure_code="${3:-release_checksum_mismatch}"

    if command -v sha256sum > /dev/null 2>&1; then
        actual=$(sha256sum "$archive" | awk '{print $1}')
    elif command -v shasum > /dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive" | awk '{print $1}')
    else
        err "release_checksum_tool_unavailable: SHA-256 검증 도구가 없어 설치를 중단합니다."
    fi

    if [ "$actual" != "$expected_checksum" ]; then
        err "${failure_code}: 다운로드 파일의 SHA-256이 일치하지 않습니다."
    fi
}

main() {
    OS="$(detect_os)"
    ARCH="$(detect_arch)"
    VERSION="${VERSION:-$(get_latest_version)}"

    if [ -z "$VERSION" ]; then
        err "최신 버전을 가져올 수 없습니다. GitHub API 한도를 확인하세요."
    fi
    version_supports_signing "$VERSION" || \
        err "unsigned_release_not_supported: v${SIGNING_FLOOR} 이상만 안전하게 설치할 수 있습니다."

    info "autopus-adk v${VERSION} 설치 중... (${OS}/${ARCH})"

    ARCHIVE="autopus-adk_${VERSION}_${OS}_${ARCH}.tar.gz"
    BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"
    URL="${BASE_URL}/${ARCHIVE}"
    CHECKSUMS_URL="${BASE_URL}/checksums.txt"
    SIGNATURES_URL="${BASE_URL}/checksums.txt.signatures"

    TMPDIR="$(mktemp -d)"
    trap 'rm -rf "$TMPDIR"' EXIT

    VERIFIER_PATH="${TMPDIR}/verify-checksums-v1.sh"
    download "$VERIFIER_URL" "$VERIFIER_PATH" || \
        err "installer_verifier_download_failed: 서명 검증기를 받을 수 없습니다."
    verify_checksum "$VERIFIER_PATH" "$VERIFIER_SHA256" installer_verifier_integrity_failed
    . "$VERIFIER_PATH"

    RUNTIME_HELPER_PATH="${TMPDIR}/install-runtime-v1.sh"
    download "$RUNTIME_HELPER_URL" "$RUNTIME_HELPER_PATH" || \
        err "installer_runtime_helper_download_failed: 실행 검증기를 받을 수 없습니다."
    [ -f "$RUNTIME_HELPER_PATH" ] && [ ! -L "$RUNTIME_HELPER_PATH" ] || \
        err "installer_runtime_helper_invalid: 실행 검증기는 regular file이어야 합니다."
    verify_checksum "$RUNTIME_HELPER_PATH" "$RUNTIME_HELPER_SHA256" installer_runtime_helper_integrity_failed
    . "$RUNTIME_HELPER_PATH"

    download "$CHECKSUMS_URL" "${TMPDIR}/checksums.txt" || \
        err "release_checksum_download_failed: checksums.txt를 받을 수 없습니다."
    download "$SIGNATURES_URL" "${TMPDIR}/checksums.txt.signatures" || \
        err "release_signature_download_failed: 서명 envelope를 받을 수 없습니다."
    mkdir -m 700 "${TMPDIR}/release-v1"
    if ! verify_release_checksums_v1 "${TMPDIR}/checksums.txt" \
        "${TMPDIR}/checksums.txt.signatures" "${TMPDIR}/release-v1"; then
        err "릴리스 서명 검증에 실패하여 설치를 중단합니다."
    fi
    ok "릴리스 서명 검증 통과 ✓"

    info "다운로드: ${URL}"
    download "$URL" "${TMPDIR}/${ARCHIVE}" || \
        err "release_archive_download_failed: 릴리스 아카이브를 받을 수 없습니다."
    if ! EXPECTED=$(awk -v name="$ARCHIVE" '
        $2 == name {
            if (NF != 2 || length($1) != 64 || $1 ~ /[^0-9a-f]/ || found) exit 2
            found=1; hash=$1
        }
        END { if (!found) exit 1; print hash }
    ' "${TMPDIR}/checksums.txt"); then
        err "release_checksum_manifest_invalid: 아카이브의 exact checksum 1건이 필요합니다."
    fi
    info "체크섬 검증 중..."
    verify_checksum "${TMPDIR}/${ARCHIVE}" "$EXPECTED"
    ok "체크섬 검증 통과 ✓"

    info "압축 해제 중..."
    tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

    info "${INSTALL_DIR}/${BINARY} 에 설치 중..."
    if [ -w "$INSTALL_DIR" ] || { [ ! -e "$INSTALL_DIR" ] && mkdir -m 755 -p "$INSTALL_DIR" 2>/dev/null; }; then
        cp "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
        chmod 755 "${INSTALL_DIR}/${BINARY}"
        ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/${ALIAS}"
    else
        echo ""
        echo "  시스템 폴더(${INSTALL_DIR})에 설치하기 위해 관리자 비밀번호가 필요합니다."
        sudo mkdir -m 755 -p "$INSTALL_DIR"
        sudo cp "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
        sudo chmod 755 "${INSTALL_DIR}/${BINARY}"
        sudo ln -sf "${INSTALL_DIR}/${BINARY}" "${INSTALL_DIR}/${ALIAS}"
    fi

    ok "autopus-adk v${VERSION} 설치 완료!"
    echo ""
    print_path_hint
    echo ""

    # Verify execution admission before invoking the more expensive doctor flow.
    info "설치된 auto 실행 확인 중..."
    version_smoke_output="${TMPDIR}/version-smoke.out"
    if run_version_smoke "${INSTALL_DIR}/${BINARY}" "$version_smoke_output"; then
        if ! version_smoke_output_matches "$VERSION" "$version_smoke_output"; then
            rm -f "$version_smoke_output"
            err "post_install_version_mismatch: auto version --short 출력이 설치 요청 버전 v${VERSION}과 일치하지 않습니다."
        fi
        rm -f "$version_smoke_output"
        ok "auto 실행 확인 완료!"
    else
        version_smoke_status=$?
        rm -f "$version_smoke_output"
        if [ "$version_smoke_status" -eq 124 ] && [ "$OS" = "darwin" ]; then
            print_macos_execution_admission_recovery
            err "post_install_execution_admission_timeout: 설치 파일은 보존했지만 실행 확인에 실패했습니다."
        fi
        err "post_install_version_smoke_failed: 설치 파일은 보존했지만 auto version --short 실행에 실패했습니다."
    fi
    echo ""

    # Post-install: check and auto-install required tools only.
    info "필수 도구 확인 중... (이미 설치된 것은 건너뜀)"
    if run_bounded_command "$DOCTOR_TIMEOUT_SECONDS" "${INSTALL_DIR}/${BINARY}" doctor --fix --yes --required-only 2>/dev/null; then
        ok "필수 도구 점검 완료!"
    else
        doctor_status=$?
        if [ "$doctor_status" -eq 124 ]; then
            echo "  필수 도구 점검이 ${DOCTOR_TIMEOUT_SECONDS}초를 초과해 관련 프로세스를 종료했습니다."
        else
            echo "  일부 필수 도구를 자동 설치하지 못했습니다."
        fi
        echo "  수동 확인: ${BINARY} doctor"
    fi
    echo ""

    ok "🐙 Autopus-ADK 준비 완료!"
    echo ""
    print_next_steps
    echo ""
}

if [ "${AUTOPUS_INSTALLER_TEST_SOURCE:-}" != "1" ]; then
    main
fi
