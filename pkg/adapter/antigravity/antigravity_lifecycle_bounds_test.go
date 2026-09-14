package antigravity

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// 두 경계는 순서가 있어야 의미가 있다. 프로브 상한이 배수 유예보다 크지 않으면
// "inherited-pipe bound가 상한보다 먼저 반환한다"는 계약이 성립하지 않고, 유예가
// 실제로 필요해진 실행에서 프로브가 상한에 걸려 실패한다. 실패는 조용하다 -
// best-effort 저하가 경고를 삼켜서, 경고를 기대하는 테스트가 "경고 없음"으로
// 뒤집힌다. 값 하나를 옮길 때 다른 하나를 잊는 것을 이 테스트가 막는다.
func TestReadinessCeilingDominatesTheDrainGrace(t *testing.T) {
	assert.Greater(t, antigravityProbeTimeout, processprobe.DefaultWaitDelay,
		"readiness ceiling must exceed processprobe.DefaultWaitDelay; "+
			"equal or smaller makes the inherited-pipe bound unreachable inside the ceiling")

	// ProbeAntigravityReadiness applies the same ordering when no explicit
	// ceiling is injected, so both defaults move together.
	defaults := normalizeAntigravityReadinessOptions(AntigravityReadinessOptions{})
	assert.Greater(t, defaults.Timeout, processprobe.DefaultWaitDelay)
}
