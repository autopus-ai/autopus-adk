#!/bin/sh
# react-review.sh — PR 리뷰 이벤트 자동 대응 훅
# Autopus-ADK가 자동 생성한 파일입니다.
# 이 스크립트는 PR 리뷰 이벤트를 처리하고 에이전트에게 전달합니다.

set -e

# 리뷰 정보 파라미터
PR_NUMBER="${PR_NUMBER:-}"
REVIEW_ACTION="${REVIEW_ACTION:-submitted}"
REVIEW_STATE="${REVIEW_STATE:-}"
REVIEWER="${REVIEWER:-}"
PR_BRANCH="${PR_BRANCH:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')}"

echo "📋 PR 리뷰 이벤트 감지"
echo "   PR: #$PR_NUMBER"
echo "   액션: $REVIEW_ACTION"
echo "   상태: $REVIEW_STATE"
echo "   리뷰어: $REVIEWER"

# 변경 요청인 경우에만 에이전트 대응
if [ "$REVIEW_STATE" = "changes_requested" ]; then
    echo "🔧 변경 요청 감지 — 에이전트 대응 시작"

    # 리뷰 정보를 파일로 기록
    REVIEW_FILE=".auto-review-request.json"
    cat > "$REVIEW_FILE" << EOF
{
  "pr_number": "$PR_NUMBER",
  "action": "$REVIEW_ACTION",
  "state": "$REVIEW_STATE",
  "reviewer": "$REVIEWER",
  "branch": "$PR_BRANCH",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

    # autopus CLI로 에이전트 알림 (설치된 경우)
    if command -v auto > /dev/null 2>&1; then
        echo "🤖 리뷰 에이전트에게 변경 요청 전달 중..."
        auto react check --quiet || true
    fi

    echo "✅ 리뷰 대응 완료"
elif [ "$REVIEW_STATE" = "approved" ]; then
    echo "✅ 리뷰 승인됨 — 대응 불필요"
fi

exit 0
