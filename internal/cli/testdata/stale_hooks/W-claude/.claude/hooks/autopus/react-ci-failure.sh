#!/bin/sh
# react-ci-failure.sh — CI 실패 자동 대응 훅
# Autopus-ADK가 자동 생성한 파일입니다.
# 이 스크립트는 CI 실패 이벤트를 감지하고 에이전트에게 알립니다.

set -e

# CI 실패 정보 파라미터
CI_STATUS="${CI_STATUS:-unknown}"
CI_RUN_ID="${CI_RUN_ID:-}"
CI_WORKFLOW="${CI_WORKFLOW:-}"
CI_BRANCH="${CI_BRANCH:-$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')}"
CI_COMMIT="${CI_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo 'unknown')}"

echo "🚨 CI 실패 감지"
echo "   브랜치: $CI_BRANCH"
echo "   커밋: $CI_COMMIT"
echo "   워크플로우: $CI_WORKFLOW"

# CI 실패 정보를 파일로 기록 (에이전트가 참조)
CI_FAILURE_FILE=".auto-ci-failure.json"
cat > "$CI_FAILURE_FILE" << EOF
{
  "status": "$CI_STATUS",
  "run_id": "$CI_RUN_ID",
  "workflow": "$CI_WORKFLOW",
  "branch": "$CI_BRANCH",
  "commit": "$CI_COMMIT",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "   실패 정보: $CI_FAILURE_FILE"

# autopus CLI로 에이전트 알림 (설치된 경우)
if command -v auto > /dev/null 2>&1; then
    echo "🤖 에이전트에게 CI 실패 전달 중..."
    auto react check --quiet || true
fi

echo "✅ CI 실패 대응 완료"
exit 0
