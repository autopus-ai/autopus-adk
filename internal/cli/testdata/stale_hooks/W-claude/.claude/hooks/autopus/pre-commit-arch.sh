#!/bin/sh
# pre-commit-arch.sh — 아키텍처 규칙 검사
# Autopus-ADK가 자동 생성한 파일입니다.
set -e

# Check staged source files only. Full-tree checks are too noisy for legacy
# repositories with existing large files.
CHANGED_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep -E "\.(go|rs|ts|tsx|js|jsx|mjs|cjs|css|scss|sass|less|py|rb|php|java|kt|kts|swift|c|cc|cpp|cxx|h|hpp|sh|vue)$" || true)

if [ -z "$CHANGED_FILES" ]; then
    echo "✅ 아키텍처 규칙 검사 완료 (변경된 소스 파일 없음)"
    exit 0
fi

# Run the canonical CLI check when available.
if command -v auto > /dev/null 2>&1; then
    echo "🔍 아키텍처 규칙 검사 중..."
    if auto check --arch --quiet --staged; then
        echo "✅ 아키텍처 규칙 검사 통과"
    else
        echo "❌ 아키텍처 규칙 위반이 발견되었습니다."
        echo "   'auto check --arch --staged' 명령어로 상세 내용을 확인하세요."
        exit 1
    fi
else
    echo "✅ 아키텍처 규칙 검사 완료 (auto CLI 없음, 기본 검사)"
fi

exit 0
