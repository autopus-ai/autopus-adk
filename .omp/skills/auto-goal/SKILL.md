---
name: auto-goal
description: goal 관리 — 현재 세션이 지원하는 목표 생성, 상태 확인, 완료·차단 처리를 연결합니다
compatibility: omp
---

# auto-goal

## OMP Invocation

- `/auto goal ...`
- `/auto-goal ...`
- Load detail skill `auto-goal` for either entrypoint.

## 실행 계약

goal 관리 — 현재 세션이 지원하는 목표 생성, 상태 확인, 완료·차단 처리를 연결합니다

1. 대상 디렉터리와 전달된 인자를 확인합니다.
2. 현재 OMP 세션의 목표 의도를 확인하고 지원되는 goal surface만 사용합니다.
3. 미지원 persisted state를 만들지 않고 제약을 명시합니다.
