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
		Name:        "auto-update",
		Description: "하네스 업데이트 — 현재 repo 또는 메타 workspace를 감지해 하네스 surface를 갱신합니다",
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
		Name:        "auto-verify",
		Description: "프론트엔드 UX 검증 — Playwright 기반 비주얼 검증을 실행합니다",
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
		Name:        "auto-doctor",
		Description: "상태 진단 — 하네스 설치 상태와 플랫폼 wiring을 점검합니다",
	},
}
