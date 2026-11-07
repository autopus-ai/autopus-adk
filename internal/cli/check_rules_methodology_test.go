package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// methodologyGateConfig는 methodology 설정만 바꾼 유효한 Full 설정을 만든다.
// quality preset과 review gate는 SKIP 경로로 두어 방법론 판정만 관측한다.
func methodologyGateConfig(mode string, enforce bool) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("methodology-gate")
	cfg.Quality.Default = ""
	cfg.Spec.ReviewGate.Enabled = false
	cfg.Methodology.Mode = mode
	cfg.Methodology.Enforce = enforce
	return cfg
}

// TestEvaluateMethodologyGate는 설정 조합별 게이트 판정을 고정한다.
func TestEvaluateMethodologyGate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		mode       string
		enforce    bool
		wantStatus methodologyGateStatus
		wantDetail []string
	}{
		{
			// The verification harness ships no methodology skill, so a
			// v0.50.134 config that enforces tdd has nothing to deliver; it is
			// reported, not failed, so `auto check` keeps passing after update.
			name:       "tdd enforced is retired, not failed",
			mode:       "tdd",
			enforce:    true,
			wantStatus: methodologyGateSkip,
			wantDetail: []string{"tdd", "methodology.mode: none"},
		},
		{
			name:       "ddd declared without enforce is retired too",
			mode:       "ddd",
			enforce:    false,
			wantStatus: methodologyGateSkip,
			wantDetail: []string{"ddd", "methodology.mode: none"},
		},
		{
			name:       "none without enforce claims nothing",
			mode:       "none",
			enforce:    false,
			wantStatus: methodologyGateSkip,
			wantDetail: []string{"none", "전달되는 규칙 없음"},
		},
		{
			name:       "empty mode is none",
			mode:       "",
			enforce:    false,
			wantStatus: methodologyGateSkip,
			wantDetail: []string{"none"},
		},
		{
			name:       "enforce without a mode fails",
			mode:       "none",
			enforce:    true,
			wantStatus: methodologyGateFail,
			wantDetail: []string{"enforce_without_mode", "enforce: true"},
		},
		{
			name:       "unknown mode fails when enforced",
			mode:       "kanban",
			enforce:    true,
			wantStatus: methodologyGateFail,
			wantDetail: []string{"unknown_mode", "kanban"},
		},
		{
			name:       "unknown mode fails even when not enforced",
			mode:       "kanban",
			enforce:    false,
			wantStatus: methodologyGateFail,
			wantDetail: []string{"unknown_mode", "kanban"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gate := evaluateMethodologyGate(methodologyGateConfig(tc.mode, tc.enforce))
			assert.Equal(t, tc.wantStatus, gate.Status)
			for _, fragment := range tc.wantDetail {
				assert.Contains(t, gate.Detail, fragment)
			}
		})
	}
}

// TestEvaluateMethodologyGate_NilConfig는 설정을 읽지 못한 경우 실패시키지 않고
// 검증 대상이 없음을 보고함을 고정한다.
func TestEvaluateMethodologyGate_NilConfig(t *testing.T) {
	t.Parallel()

	gate := evaluateMethodologyGate(nil)
	assert.Equal(t, methodologyGateSkip, gate.Status)
}

// TestCheckMethodology_NoConfig는 초기화되지 않은 디렉터리를 통과시킴을 고정한다.
func TestCheckMethodology_NoConfig(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	assert.True(t, checkMethodology(t.TempDir(), &out, false))
	assert.Contains(t, out.String(), "autopus.yaml 없음")
}

// TestCheckMethodology_ValidAndBrokenConfig는 auto check 표면이 저장된 설정에
// 대해 fail-closed임을 고정한다.
func TestCheckMethodology_ValidAndBrokenConfig(t *testing.T) {
	t.Parallel()

	valid := t.TempDir()
	require.NoError(t, config.Save(valid, methodologyGateConfig("tdd", true)))
	var validOut bytes.Buffer
	assert.True(t, checkMethodology(valid, &validOut, false))
	assert.Contains(t, validOut.String(), "methodology.mode: none")

	broken := t.TempDir()
	require.NoError(t, config.Save(broken, methodologyGateConfig("kanban", true)))
	var brokenOut bytes.Buffer
	assert.False(t, checkMethodology(broken, &brokenOut, false))
	assert.Contains(t, brokenOut.String(), "kanban")

	// quiet 모드에서도 실패는 보고된다.
	var quietOut bytes.Buffer
	assert.False(t, checkMethodology(broken, &quietOut, true))
	assert.Contains(t, quietOut.String(), "unknown_mode")
}

// TestMethodologyGate_DoctorSurfaceParity는 doctor의 text 표면과 JSON 표면이
// 같은 판정을 보고함을 고정한다.
func TestMethodologyGate_DoctorSurfaceParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		cfg        *config.HarnessConfig
		wantTextOK bool
		wantStatus string
	}{
		{
			name:       "retired methodology",
			cfg:        methodologyGateConfig("tdd", true),
			wantTextOK: true,
			wantStatus: "skip",
		},
		{
			name:       "unknown mode",
			cfg:        methodologyGateConfig("kanban", true),
			wantTextOK: false,
			wantStatus: "fail",
		},
		{
			name:       "enforce without mode",
			cfg:        methodologyGateConfig("none", true),
			wantTextOK: false,
			wantStatus: "fail",
		},
		{
			name:       "methodology disabled",
			cfg:        methodologyGateConfig("none", false),
			wantTextOK: true,
			wantStatus: "skip",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			assert.Equal(t, tc.wantTextOK, checkQualityGate(&out, tc.cfg))

			report := doctorJSONReport{status: jsonStatusOK}
			report.collectQualityGateChecks(tc.cfg)
			check, found := getCheck(report.checks, "doctor.methodology.mode")
			require.True(t, found)
			assert.Equal(t, tc.wantStatus, check.Status)

			// 두 표면의 근거 문구는 같은 판정에서 나온다.
			gate := evaluateMethodologyGate(tc.cfg)
			assert.Equal(t, gate.Detail, check.Detail)
			assert.Contains(t, out.String(), gate.Detail)
		})
	}
}
