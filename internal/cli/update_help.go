package cli

// updateCmdLong is the help of `auto update`; the composition step it
// describes is SPEC-MODELUX-001 and SPEC-MODELUX-002.
const updateCmdLong = "설치된 하네스 파일을 업데이트합니다.\n\n" +
	"보존 범위는 파일 종류에 따라 다릅니다.\n" +
	"  - 마커 밖 사용자 내용(AGENTS.md, CLAUDE.md 등): 보존됩니다. AUTOPUS 마커 구간만 교체합니다.\n" +
	"  - 마커 안 내용: 보존되지 않습니다. 매 업데이트마다 재생성됩니다.\n" +
	"  - 그 외 생성 파일(대부분 overwrite 정책): 사용자 수정이 보존되지 않고 덮어쓰입니다.\n" +
	"    덮어쓰기 전 원본은 .autopus/backup/<timestamp>/transaction/<platform>/<경로>에 복사됩니다.\n" +
	"  - 사용자가 삭제한 생성 파일: 다시 만들지 않고 건너뜁니다.\n\n" +
	"--preview(--plan/--dry-run)로 변경 예정 파일과 각 파일의 처리 방식을 먼저 확인할 수 있습니다.\n\n" +
	"에이전트 모델 구성: 대화형 터미널에서는 프로젝트 설정 질문 뒤, 플랫폼 파일 생성 전에 에이전트×플랫폼\n" +
	"표와 프리셋 상태를 보여 주고 매번 \"구성을 바꿀까요?\"를 묻습니다. Enter, 입력 끝, 잘못된 답 3번은\n" +
	"autopus.yaml을 그대로 둡니다. u는 이전 버전 기본 프리셋 갱신, g는 역할 그룹 4개, a는 에이전트별 등급이고\n" +
	"(auto quality --help), 바뀐 칸에 *가 붙은 미리보기를 y로 확인해야만 쓰며 같은 실행의 생성 파일이 그 미리보기와\n" +
	"같습니다. --yes(-y), --auto, --check, CI(비어 있지 않고 0·false가 아닌 값), TTY가 아닌 입력, --workspace는\n" +
	"묻지도 바꾸지도 않고 이전 버전 기본값 안내만 한 줄 씁니다. --plan/--preview/--dry-run에는 이 단계가 없습니다."
