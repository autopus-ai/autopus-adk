package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

func TestDecideDoctorVerdict(t *testing.T) {
	for _, tc := range []struct {
		name        string
		in          doctorVerdictInput
		want        doctorVerdictKind
		contains    []string
		notContains []string
	}{
		{
			name: "clean report passes",
			in:   doctorVerdictInput{allOK: true},
			want: doctorVerdictPass, contains: []string{"All checks passed"},
		},
		{
			// The original defect: degraded OMP routing set the platform
			// failure flag, so the box said FAIL and advised `auto update`
			// over a page with no ERROR line.
			name: "warnings without an ERROR line warn and name their sections",
			in:   doctorVerdictInput{unhealthy: []string{"omp", "Hygiene"}},
			want: doctorVerdictWarn, contains: []string{"nothing failed", "omp, Hygiene"},
			notContains: []string{"auto update", "FAIL"},
		},
		{
			name: "an ERROR line fails even when no check marked the report unhealthy",
			in:   doctorVerdictInput{allOK: true, errorLines: 1},
			want: doctorVerdictFail, contains: []string{"review the [ERROR] lines"},
		},
		{
			name: "a platform ERROR names the installer",
			in:   doctorVerdictInput{errorLines: 2, updateFixes: true, unhealthy: []string{"claude-code"}},
			want: doctorVerdictFail, contains: []string{"auto update"},
		},
		{
			name: "a warn-only retired surface still names its repair",
			in: doctorVerdictInput{unhealthy: []string{"Retired Orchestra Surface"},
				updateFixes: true, retiredUpdateFixes: true},
			want: doctorVerdictWarn, contains: []string{"auto update", "Retired Orchestra Surface"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decideDoctorVerdict(tc.in)
			assert.Equal(t, tc.want, got.kind)
			for _, s := range tc.contains {
				assert.Contains(t, got.message, s)
			}
			for _, s := range tc.notContains {
				assert.NotContains(t, got.message, s)
			}
		})
	}
}

func TestOMPDoctorChecksFailed_DistinguishesWarnFromFail(t *testing.T) {
	degraded := []jsonCheck{{ID: "a", Status: "pass"}, {ID: "b", Status: "warn"}}
	assert.False(t, ompDoctorChecksHealthy(degraded))
	assert.False(t, ompDoctorChecksFailed(degraded),
		"degraded model routing warns; it is not a managed-surface failure")

	broken := append(degraded, jsonCheck{ID: "c", Status: "fail"})
	assert.True(t, ompDoctorChecksFailed(broken))
}

func TestOMPModelDoctorCheckLevel_SessionModelIsNotAFailure(t *testing.T) {
	_, status := ompModelDoctorCheckLevel(omp.OMPReaSessionModelStatus)
	assert.Equal(t, "pass", status)
	_, status = ompModelDoctorCheckLevel("blocked")
	assert.Equal(t, "fail", status)
}
