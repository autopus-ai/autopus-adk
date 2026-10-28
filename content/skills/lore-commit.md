---
name: lore-commit
description: Lore 커밋 메시지 작성 및 의사결정 기록 스킬
triggers:
  - lore
  - commit message
  - 커밋 메시지
  - 의사결정
  - decision record
category: workflow
level1_metadata: "Lore 커밋 형식, 의사결정 기록, 트레일러 태그"
---

# Lore Commit Skill

Lore 형식으로 의사결정을 커밋 메시지에 기록하는 스킬입니다.

## Lore 커밋 형식

### 기본 구조
```
<type>(<scope>): <subject>

<body>

<lore-trailers>
🐙 Autopus <noreply@autopus.co>
```

### 타입 분류
| 타입 | 설명 |
|------|------|
| `feat` | 새로운 기능 추가 |
| `fix` | 버그 수정 |
| `refactor` | 기능 변경 없는 코드 개선 |
| `test` | 테스트 추가/수정 |
| `docs` | 문서 수정 |
| `chore` | 빌드, 설정 변경 |
| `perf` | 성능 개선 |
| `merge` | 병합 또는 릴리스 통합 커밋 |
| `revert` | 이전 변경 되돌리기 |

## Lore 트레일러 태그

### 현재 프로토콜
```
Constraint: [지켜야 하는 제약 또는 경계]
Rejected: [버린 대안]
Confidence: [low|medium|high]
Scope-risk: [local|module|system]
Reversibility: [trivial|moderate|difficult]
Directive: [후속 작업 지침]
Tested: [검증한 항목]
Not-tested: [아직 검증하지 못한 항목]
Related: [관련 이슈/PR/SPEC]
```

### 기본 요구사항
```
auto check --lore      -> 타입 프리픽스 + Autopus 사인오프 검사
auto lore validate     -> 설정된 required_trailers 기반 Lore 트레일러 검사
```

## 예시

```
feat(auth): JWT 기반 인증 구현

사용자 세션 관리를 위해 JWT 토큰 방식을 도입합니다.
기존 세션 쿠키 방식에서 마이크로서비스 환경에 적합한
Stateless 인증으로 전환합니다.

Constraint: stateless 인증 흐름 유지
Rejected: 세션 저장소 기반 인증
Confidence: medium
Scope-risk: module
Reversibility: moderate
Directive: refresh token 정책은 후속 SPEC으로 분리
Tested: JWT 발급 및 검증 통합 테스트
Not-tested: 토큰 로테이션 장애 복구
Related: SPEC-AUTH-001

🐙 Autopus <noreply@autopus.co>
```

## 작성 지침

1. Subject는 Lore 타입 프리픽스로 시작한다. 예: `fix(cli): ...`
2. Body는 배경과 의도를 간결하게 남긴다.
3. 새 Lore 트레일러는 `Constraint` 중심 프로토콜을 사용한다.
4. `Why` / `Decision` / `Alternatives` 예시는 더 이상 표준이 아니다.

## 자동 검사

`auto check --lore` 실행 시 다음을 검사합니다:
- 커밋 메시지 형식 준수 여부
- Autopus 사인오프 존재 여부
- 타입은 `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `merge`, `revert` 중 하나여야 하며 스코프를 포함해야 합니다.

`auto lore validate` 실행 시 다음을 검사합니다:
- `required_trailers` 충족 여부
- `Confidence`, `Scope-risk`, `Reversibility` 값 유효성
- `Scope-risk`는 `local`, `module`, `system` 중 하나여야 합니다.
