package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

// methodologyGateStatus는 방법론 게이트 판정이다.
type methodologyGateStatus string

const (
	// methodologyGatePass는 방법론 정의가 로드되고 단계 순서까지 검증된 상태이다.
	methodologyGatePass methodologyGateStatus = "pass"
	// methodologyGateFail은 설정이 검증되지 않은 상태이다.
	methodologyGateFail methodologyGateStatus = "fail"
	// methodologyGateSkip은 방법론을 쓰지 않으므로 검증 대상이 없는 상태이다.
	methodologyGateSkip methodologyGateStatus = "skip"
)

// methodologyGate는 방법론 설정 검증 결과이다.
// auto check, auto doctor(text), auto doctor(JSON) 세 표면이 같은 판정을 쓰도록
// 계산은 evaluateMethodologyGate 한 곳에만 둔다.
type methodologyGate struct {
	// Status는 게이트 판정이다.
	Status methodologyGateStatus
	// Detail은 사람이 읽는 판정 근거이다.
	Detail string
}

// evaluateMethodologyGate는 methodology 설정을 fail-closed로 검증한다.
//
// mode가 none(또는 빈 값)이고 enforce가 false이면 하네스가 아무 방법론도
// 주장하지 않으므로 skip이다. 그 외에는 mode가 로드 가능한 정의로 해석되고
// required_before 체인이 순환·미해결 참조·중복 이름 없는 선형 순서를 이루어야
// 하며, 그렇지 않으면 원인 단계를 이름으로 지목해 실패한다.
// enforce: true인데 mode가 none이면 전달할 규칙이 없으므로 실패이다.
func evaluateMethodologyGate(cfg *config.HarnessConfig) methodologyGate {
	if cfg == nil {
		return methodologyGate{Status: methodologyGateSkip, Detail: "methodology: 설정을 읽을 수 없습니다"}
	}

	mode := content.NormalizeMethodologyMode(cfg.Methodology.Mode)
	enforce := cfg.Methodology.Enforce

	def, err := content.ResolveEnforcedMethodology(contentfs.FS, mode, enforce)
	var defect *content.MethodologyError
	if errors.As(err, &defect) && defect.Defect == content.DefectMissingDelivery {
		// The verification harness ships no methodology skill. A config an
		// older release wrote still names one; nothing reads it any more, so
		// it is reported instead of failing every `auto check` after update.
		return methodologyGate{
			Status: methodologyGateSkip,
			Detail: fmt.Sprintf("methodology: %s — 이 하네스는 방법론 스킬을 설치하지 않아 전달되는 규칙이 없습니다; "+
				"methodology.mode: none, enforce: false로 바꾸면 이 안내가 사라집니다", mode),
		}
	}
	if err != nil {
		return methodologyGate{
			Status: methodologyGateFail,
			Detail: fmt.Sprintf("methodology: %s (enforce: %v) — %v", mode, enforce, err),
		}
	}
	if def == nil {
		return methodologyGate{
			Status: methodologyGateSkip,
			Detail: fmt.Sprintf("methodology: %s (enforce: false, 전달되는 규칙 없음)", mode),
		}
	}
	return methodologyGate{
		Status: methodologyGatePass,
		Detail: fmt.Sprintf("methodology: %s (enforce: %v, 강제 규칙 %d개, 단계 순서 검증: %s)",
			def.Name, enforce, len(def.EnforceRules), methodologyStageOrder(def)),
	}
}

// methodologyStageOrder는 검증된 단계 순서를 사람이 읽는 표기로 잇는다.
func methodologyStageOrder(def *content.MethodologyDef) string {
	names := make([]string, 0, len(def.Stages))
	for _, stage := range def.Stages {
		names = append(names, stage.Name)
	}
	return strings.Join(names, " → ")
}

// report는 판정을 체크리스트 표면에 출력한다.
func (g methodologyGate) report(out io.Writer) {
	switch g.Status {
	case methodologyGateFail:
		tui.FAIL(out, g.Detail)
	case methodologyGateSkip:
		tui.SKIP(out, g.Detail)
	default:
		tui.OK(out, g.Detail)
	}
}

// checkMethodology는 auto check의 방법론 게이트이다.
// 설정이 없는 디렉터리(훅이 초기화되지 않은 프로젝트에서 실행되는 경우)는
// 검증 대상이 아니므로 통과시킨다.
func checkMethodology(dir string, out io.Writer, quiet bool) bool {
	if !quiet {
		tui.SectionHeader(out, "methodology: 설정 검증")
	}

	if !config.Exists(dir) {
		if !quiet {
			tui.SKIP(out, "methodology: autopus.yaml 없음")
		}
		return true
	}

	cfg, err := config.LoadPreview(dir)
	if err != nil {
		tui.FAIL(out, fmt.Sprintf("methodology: autopus.yaml 로드 실패: %v", err))
		return false
	}

	gate := evaluateMethodologyGate(cfg)
	if !quiet || gate.Status == methodologyGateFail {
		gate.report(out)
	}
	return gate.Status != methodologyGateFail
}
