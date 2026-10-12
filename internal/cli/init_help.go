package cli

// initCmdLong is the help of `auto init`; the composition step it describes
// is SPEC-MODELUX-001 and SPEC-MODELUX-002.
const initCmdLong = "코딩 CLI에 Autopus 하네스를 설치합니다. autopus.yaml을 생성하고 플랫폼 파일을 설치합니다.\n\n" +
	"대화형 wizard의 품질 단계는 Ultra, Balanced, 역할별 직접 구성 중 하나를 고르고, 어느 쪽이든\n" +
	"플랫폼 파일을 쓰기 전에 에이전트×플랫폼 구성 표를 보여 줍니다. 역할별 직접 구성은 balanced\n" +
	"기본값에서 시작해 역할 그룹 4개(g: 설계·판단, 구현, 검증·리뷰, 탐색·보조)나 에이전트별(a)\n" +
	"등급을 고르고, 미리보기를 y로 확인하면 그 구성을 autopus.yaml에 쓰고 같은 표대로 생성합니다.\n" +
	"유지하거나 취소하면 balanced로 진행합니다. --yes, TTY가 아닌 입력, --quality <preset>은\n" +
	"이 단계를 건너뜁니다.\n\n" +
	"대화형 터미널에서는 역할별 직접 구성이 화살표와 Enter 폼입니다. 메뉴의 구성 편집은 도구 탭(모든 도구 같게,\n" +
	"Claude Code, Codex, 이번 init에서 고른 platforms에 omp가 있으면 OMP)을 열고, 선택지는 그 도구의 실제 모델과\n" +
	"effort를 보여 줍니다(Claude 라벨은 slug opus-5-5, 표 칸은 frontmatter 값 opus). Claude Code·Codex 탭은\n" +
	"quality.presets.<provider>-custom과 quality.providers.<provider>를, OMP 탭은 init 설정의 role_model_policy를\n" +
	"정하고 init이 파일 생성 때 함께 씁니다. OMP catalog를 읽지 못하면 안내 뒤 init을 계속합니다. 폼에서 Ctrl+C는\n" +
	"init cancelled.를 쓰고 아무 파일도 만들지 않습니다. 파이프나 스크립트 입력은 줄 질문 그대로입니다."
