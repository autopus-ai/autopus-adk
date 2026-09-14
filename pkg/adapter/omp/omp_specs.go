package omp

type workflowSpec struct {
	Name        string
	Description string
	SkillPath   string
}

var workflowSpecs = []workflowSpec{
	{
		Name:        "auto",
		Description: "Autopus 명령 라우터 — OpenCode helper 및 update workflow 서브커맨드를 해석합니다",
		SkillPath:   "claude/commands/auto-router.md.tmpl",
	},
	{
		Name:        "auto-setup",
		Description: "프로젝트 컨텍스트 생성 — 코드베이스를 분석하고 ARCHITECTURE.md 및 .autopus/project 문서를 생성합니다",
		SkillPath:   "codex/skills/auto-setup.md.tmpl",
	},
	{
		Name:        "auto-status",
		Description: "SPEC 대시보드 — 현재 프로젝트와 서브모듈의 SPEC 상태를 표시합니다",
	},
	{
		Name:        "auto-goal",
		Description: "goal 관리 — 현재 세션이 지원하는 목표 생성, 상태 확인, 완료·차단 처리를 연결합니다",
	},
	{
		Name:        "auto-update",
		Description: "하네스 업데이트 — 현재 repo 또는 메타 workspace를 감지해 하네스 surface를 갱신합니다",
	},
	{
		Name:        "auto-plan",
		Description: "SPEC 작성 — 코드베이스 분석 후 EARS 요구사항, 구현 계획, 인수 기준을 생성합니다",
		SkillPath:   "codex/skills/auto-plan.md.tmpl",
	},
	{
		Name:        "auto-go",
		Description: "SPEC 구현 — SPEC 문서를 기반으로 코드를 구현합니다",
		SkillPath:   "codex/skills/auto-go.md.tmpl",
	},
	{
		Name:        "auto-fix",
		Description: "버그 수정 — 재현과 최소 수정 중심으로 문제를 해결합니다",
		SkillPath:   "codex/skills/auto-fix.md.tmpl",
	},
	{
		Name:        "auto-review",
		Description: "코드 리뷰 — TRUST 5 기준으로 변경된 코드를 리뷰합니다",
		SkillPath:   "codex/skills/auto-review.md.tmpl",
	},
	{
		Name:        "auto-sync",
		Description: "문서 동기화 — 구현 이후 SPEC, CHANGELOG, 문서를 반영합니다",
		SkillPath:   "codex/skills/auto-sync.md.tmpl",
	},
	{
		Name:        "auto-idea",
		Description: "아이디어 브레인스토밍 — 멀티 프로바이더 토론과 ICE 평가로 아이디어를 정리합니다",
		SkillPath:   "codex/skills/auto-idea.md.tmpl",
	},
	{
		Name:        "auto-map",
		Description: "코드베이스 분석 — 구조, 엔트리포인트, 의존성을 빠르게 요약합니다",
	},
	{
		Name:        "auto-why",
		Description: "의사결정 근거 조회 — Lore, SPEC, ARCHITECTURE에서 이유를 추적합니다",
	},
	{
		Name:        "auto-verify",
		Description: "프론트엔드 UX 검증 — Playwright 기반 비주얼 검증을 실행합니다",
	},
	{
		Name:        "auto-secure",
		Description: "보안 감사 — OWASP Top 10 관점에서 변경 범위를 점검합니다",
	},
	{
		Name:        "auto-test",
		Description: "E2E 시나리오 실행 — scenarios.md 기반 검증을 수행합니다",
	},
	{
		Name:        "auto-qa",
		Description: "QAMESH project QA mesh — plan, run, report, and publish deterministic QA evidence",
		SkillPath:   "codex/skills/auto-qa.md.tmpl",
	},
	{
		Name:        "auto-dev",
		Description: "풀 사이클 개발 — plan → go → sync를 순차 실행합니다",
	},
	{
		Name:        "auto-canary",
		Description: "배포 검증 — build, E2E, 브라우저 건강 검진을 실행합니다",
		SkillPath:   "codex/skills/auto-canary.md.tmpl",
	},
	{
		Name:        "auto-doctor",
		Description: "상태 진단 — 하네스 설치 상태와 플랫폼 wiring을 점검합니다",
	},
}
