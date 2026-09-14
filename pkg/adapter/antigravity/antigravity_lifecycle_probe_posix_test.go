//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// probeGrandchildLifetime is how long a probe fixture stays alive — the
// backgrounded grandchild that holds the inherited stdout pipe, and the hung
// child below. It doubles as the readiness-probe ceiling injected through
// validateWithin. Every bound under test returns far sooner, so the elapsed
// assertions keep a wide margin instead of measuring how busy the machine is:
// the healthy paths finish in well under a second, while either regression —
// draining until the grandchild closes the pipe, or ignoring the caller
// deadline — runs to the full lifetime. The contract is that those bounds are
// alive, not that validation is quick.
const probeGrandchildLifetime = 30 * time.Second

func TestValidateReadinessInheritedPipeDegradesWithinBound(t *testing.T) {
	a := generatedAntigravityAdapterForProbe(t)
	installAgyProbe(t, "#!/bin/sh\n(/bin/sleep 30) &\nprintf '1.1.26\\n'\n")
	started := time.Now()

	errs, err := a.validateWithin(context.Background(), probeGrandchildLifetime)

	require.NoError(t, err)
	assert.False(t, containsAntigravityCapabilityWarning(errs),
		"a probe bounded by the inherited-pipe grace must not invent a capability finding")
	assert.Less(t, time.Since(started), probeGrandchildLifetime/3,
		"validation must return on the inherited-pipe bound, not on a grandchild that inherited the probe pipes")
}

func TestValidateReadinessHonorsCallerContext(t *testing.T) {
	a := generatedAntigravityAdapterForProbe(t)
	installAgyProbe(t, "#!/bin/sh\n/bin/sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()

	errs, err := a.validateWithin(ctx, probeGrandchildLifetime)

	require.NoError(t, err)
	assert.False(t, containsAntigravityCapabilityWarning(errs))
	assert.Less(t, time.Since(started), probeGrandchildLifetime/3,
		"the caller deadline must stop the readiness probe well before its own ceiling")
}

// A CLI that answers is believed; a CLI below the verified floor is reported
// rather than assumed capable.
func TestValidateReadinessReportsOnlyUnverifiedVersions(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantWarning bool
	}{
		{name: "verified floor", version: "1.1.26", wantWarning: false},
		{name: "newer release", version: "1.2.2", wantWarning: false},
		{name: "below floor", version: "1.0.9", wantWarning: true},
		{name: "unparseable banner", version: "unknown build", wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := generatedAntigravityAdapterForProbe(t)
			installAgyProbe(t, "#!/bin/sh\nprintf '%s\\n' '"+tt.version+"'\n")

			errs, err := a.Validate(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.wantWarning, containsAntigravityCapabilityWarning(errs))
		})
	}
}

// An absent CLI is not a project defect, so validation stays silent about it.
func TestValidateReadinessSilentWithoutTheCLI(t *testing.T) {
	a := generatedAntigravityAdapterForProbe(t)
	t.Setenv("PATH", t.TempDir())

	errs, err := a.Validate(context.Background())

	require.NoError(t, err)
	assert.False(t, containsAntigravityCapabilityWarning(errs))
}

func generatedAntigravityAdapterForProbe(t *testing.T) *Adapter {
	t.Helper()
	a := NewWithRoot(t.TempDir())
	_, err := a.Generate(context.Background(), config.DefaultFullConfig("probe-test"))
	require.NoError(t, err)
	return a
}

func installAgyProbe(t *testing.T, content string) {
	t.Helper()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, cliBinary), []byte(content), 0o755))
	t.Setenv("PATH", binDir)
}

func containsAntigravityCapabilityWarning(errs []adapter.ValidationError) bool {
	for _, validationErr := range errs {
		if validationErr.File == antigravityPluginDir && validationErr.Level == "warning" {
			return true
		}
	}
	return false
}
