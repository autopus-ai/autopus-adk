package gates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscalateOutsideSurface(t *testing.T) {
	low := AssessChange(KindBugfix, Classify([]string{"internal/cli/spec_timing.go"}, nil), false)
	assert.False(t, low.Escalated())

	same := low.EscalateOutsideSurface(nil)
	assert.Equal(t, low, same)

	raised := low.EscalateOutsideSurface([]string{"pkg/spec/profile.go"})
	assert.True(t, raised.Escalated())
	assert.Equal(t, KindFeature, raised.EffectiveClass)
	assert.Equal(t, RiskHigh, raised.Tier)
	assert.Contains(t, raised.Reasons, EscalationLeanSurfaceExceeded)
	assert.Empty(t, low.Reasons, "the original record is not mutated")

	security := AssessChange(KindBugfix, Classify([]string{"internal/auth/token.go"}, nil), false)
	assert.Equal(t, KindSecurityOrData, security.EscalateOutsideSurface([]string{"x/y.go"}).EffectiveClass,
		"a higher effective class is never lowered")
}

func TestPathsOutsideSurface_DirectoryCoversDescendantsAndHarnessStateIsIgnored(t *testing.T) {
	surface := []string{"internal/cli/spec_timing.go", "docs"}
	changed := []string{
		"internal/cli/spec_timing.go",
		"docs/guide/timing.md",
		".autopus/specs/SPEC-X-001/review.md",
		"internal/cli/spec_review.go",
	}
	assert.Equal(t, []string{"internal/cli/spec_review.go"}, PathsOutsideSurface(surface, changed))
}
