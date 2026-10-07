package healthband_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Untrusted Input Contract item 7: names are filtered to [A-Za-z0-9 ._:+-]
// and at most 80 characters; a changed name gets #<h8 of the raw name>.
func TestSanitizeIdentifier_FiltersAndMarksChangedNames(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 100)
	for _, tc := range []struct {
		name, raw, want string
		changed         bool
	}{
		{name: "plain name is unchanged", raw: "CI", want: "CI"},
		{name: "space and punctuation are allowed", raw: "Security Scan", want: "Security Scan"},
		{name: "canary target charset", raw: "api.example.com:8080+app.example.com", want: "api.example.com:8080+app.example.com"},
		{name: "S4 escape sequence", raw: "Lint\x1b[2J", want: "Lint2J#c1b4753b", changed: true},
		{name: "over 80 characters", raw: long, want: strings.Repeat("a", 80) + "#" + healthband.H8(long), changed: true},
		{name: "nothing allowed survives", raw: "日本", want: "#" + healthband.H8("日本"), changed: true},
		{name: "empty name", raw: "", want: "#e3b0c442", changed: true},
		{name: "hash sign is not an identifier byte", raw: "a#b", want: "ab#" + healthband.H8("a#b"), changed: true},
	} {
		got, changed := healthband.SanitizeIdentifier(tc.raw)
		assert.Equal(t, tc.want, got, tc.name)
		assert.Equal(t, tc.changed, changed, tc.name)
	}
}

// acceptance.md: the <h8> of ci.failure_rate:CI is c6d37d0a.
func TestH8_IsTheFirstEightHexOfSHA256(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "c6d37d0a", healthband.H8("ci.failure_rate:CI"))
	assert.Equal(t, "c1b4753b", healthband.H8("Lint\x1b[2J"))
}

// S4: only filtered IDs become series; the reason is reported to the caller.
func TestSeriesIDs_BuildFilteredSeries(t *testing.T) {
	t.Parallel()
	ci, ciChanged := healthband.CISeriesID("Lint\x1b[2J")
	assert.Equal(t, "ci.failure_rate:Lint2J#c1b4753b", ci)
	assert.True(t, ciChanged)
	plain, plainChanged := healthband.CISeriesID("Security Scan")
	assert.Equal(t, "ci.failure_rate:Security Scan", plain)
	assert.False(t, plainChanged)
	canary, canaryChanged := healthband.CanarySeriesID("api.example.com:8080+app.example.com")
	assert.Equal(t, "canary.failure_rate:api.example.com:8080+app.example.com", canary)
	assert.False(t, canaryChanged)
}

// Every ID the filter can emit is accepted by the store, and nothing else is.
func TestValidSeriesID_AcceptsExactlyFilterOutput(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"CI", "Lint\x1b[2J", strings.Repeat("Ω", 50), "", "a#b", "\n\t", strings.Repeat("z", 81)} {
		ci, _ := healthband.CISeriesID(raw)
		canary, _ := healthband.CanarySeriesID(raw)
		assert.True(t, healthband.ValidSeriesID(ci), "ci %q", raw)
		assert.True(t, healthband.ValidSeriesID(canary), "canary %q", raw)
	}
	for _, series := range []string{
		"cpu.load:CI", "ci.failure_rate:", "ci.failure_rate:Lint\x1b[2J", "ci.failure_rate:a#C1B4753B",
		"ci.failure_rate:a#c1b4753", "ci.failure_rate:a#c1b4753b#c1b4753b", "ci.failure_rate:" + strings.Repeat("z", 81),
	} {
		assert.False(t, healthband.ValidSeriesID(series), "%q", series)
	}
}
