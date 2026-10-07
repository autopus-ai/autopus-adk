#!/bin/sh
# pre-commit-lore.sh — Lore 커밋 메시지 형식 검사
# Autopus-ADK가 자동 생성한 파일입니다.
set -e

# 커밋 메시지 파일 경로
COMMIT_MSG_FILE="$1"
if [ -z "$COMMIT_MSG_FILE" ]; then
    COMMIT_MSG_FILE=".git/COMMIT_EDITMSG"
fi

# 커밋 메시지 읽기
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE" 2>/dev/null || echo "")

# 병합 커밋은 검사 생략
if echo "$COMMIT_MSG" | grep -q "^Merge"; then
    exit 0
fi

# 자동 커밋은 검사 생략 (rebase, fixup 등)
if echo "$COMMIT_MSG" | grep -qE "^(fixup|squash)!"; then
    exit 0
fi

# Conventional Commits 형식 검사
# 형식: <type>(<scope>): <subject>
PATTERN="^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\(.+\))?: .{1,72}$"
FIRST_LINE=$(echo "$COMMIT_MSG" | head -1)

if ! echo "$FIRST_LINE" | grep -qE "$PATTERN"; then
    echo "❌ Lore 커밋 형식 위반"
    echo ""
    echo "현재: $FIRST_LINE"
    echo ""
    echo "올바른 형식:"
    echo "  <type>(<scope>): <subject>"
    echo ""
    echo "허용된 타입: feat, fix, docs, style, refactor, test, chore, perf, ci, build, revert"
    echo "예시: feat(auth): JWT 기반 인증 구현"
    echo ""
    echo "자동 검사를 건너뛰려면: git commit --no-verify"
    exit 1
fi

# Subject 길이 검사 (72자 이하)
SUBJECT_LENGTH=$(echo "$FIRST_LINE" | wc -c)
if [ "$SUBJECT_LENGTH" -gt 73 ]; then
    echo "⚠️  커밋 제목이 72자를 초과합니다 (현재: ${SUBJECT_LENGTH}자)"
    echo "간결하게 작성해주세요."
    exit 1
fi

echo "✅ Lore 커밋 형식 검사 통과"
exit 0
