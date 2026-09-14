package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/desktopobserve"
	"github.com/insajin/autopus-adk/pkg/qa/evidence"
)

// A journey with no desktop evidence must not grow an empty desktop section,
// which would read as an observation that ran and found nothing.
func TestDesktopViewAbsentWithoutDesktopEvidence(t *testing.T) {
	t.Parallel()
	assert.Nil(t, desktopView(evidence.OracleResults{}))
}

// A timeout classification is the only desktop signal a run that never produced
// a projection has, so it must still surface.
func TestDesktopViewReportsTimeoutClassWithoutProjection(t *testing.T) {
	t.Parallel()
	view := desktopView(evidence.OracleResults{
		Desktop: &evidence.DesktopOracle{TimeoutClassification: "provider_timeout"},
	})
	require.NotNil(t, view)
	assert.Equal(t, "provider_timeout", view.TimeoutClass)
	assert.Zero(t, view.NodeCount)
	assert.Zero(t, view.CheckCount)
	assert.Empty(t, view.Digest)
}

// An observation without a semantic projection still carries deterministic
// checks; reporting zero checks there would hide the evidence that exists.
func TestDesktopViewCountsChecksWithoutProjection(t *testing.T) {
	t.Parallel()
	view := desktopView(evidence.OracleResults{
		DesktopObservation: &desktopobserve.ObservationEvidence{
			DeterministicChecks: []desktopobserve.DeterministicCheck{{}, {}, {}},
		},
	})
	require.NotNil(t, view)
	assert.Equal(t, 3, view.CheckCount)
	assert.Zero(t, view.NodeCount, "no projection means no node count")
	assert.Empty(t, view.RootRole)
}

// The projection identity is what makes a desktop observation auditable, and
// the node count must include every descendant, not just the root's children.
func TestDesktopViewProjectsIdentityAndCountsWholeTree(t *testing.T) {
	t.Parallel()
	view := desktopView(evidence.OracleResults{
		Desktop: &evidence.DesktopOracle{TimeoutClassification: "none"},
		DesktopObservation: &desktopobserve.ObservationEvidence{
			DeterministicChecks: []desktopobserve.DeterministicCheck{{}},
			SemanticProjection: &desktopobserve.SemanticProjection{
				SchemaVersion: "qamesh.desktop.v1",
				ProviderRef:   "macos-ax",
				AppRef:        "com.example.app",
				WindowRef:     "window-1",
				StateRef:      "state-1",
				Digest:        "sha256:abc",
				Root: desktopobserve.SemanticNode{
					Role: desktopobserve.Role("window"),
					Children: []desktopobserve.SemanticNode{
						{Role: desktopobserve.Role("group"), Children: []desktopobserve.SemanticNode{
							{Role: desktopobserve.Role("button")},
							{Role: desktopobserve.Role("text")},
						}},
						{Role: desktopobserve.Role("button")},
					},
				},
			},
		},
	})
	require.NotNil(t, view)
	assert.Equal(t, "qamesh.desktop.v1", view.SchemaVersion)
	assert.Equal(t, "macos-ax", view.ProviderRef)
	assert.Equal(t, "com.example.app", view.AppRef)
	assert.Equal(t, "window-1", view.WindowRef)
	assert.Equal(t, "state-1", view.StateRef)
	assert.Equal(t, "sha256:abc", view.Digest)
	assert.Equal(t, "window", view.RootRole)
	assert.Equal(t, "none", view.TimeoutClass)
	assert.Equal(t, 1, view.CheckCount)
	assert.Equal(t, 5, view.NodeCount)
}

// A mobile run is only reproducible with the flow, build digest, and device it
// ran against, so those refs must reach the source view.
func TestSourceViewCarriesMobileRefs(t *testing.T) {
	t.Parallel()
	plain := sourceView(evidence.SourceRefs{SourceSpec: "SPEC-1", AcceptanceRefs: []string{"AC-1"}})
	assert.Equal(t, "SPEC-1", plain.SourceSpec)
	assert.Empty(t, plain.MobileFlowID)
	assert.Empty(t, plain.MobileDigest)
	assert.Empty(t, plain.MobileDeviceRef)

	mobile := sourceView(evidence.SourceRefs{
		SourceSpec: "SPEC-1",
		Mobile: &evidence.MobileRefs{
			FlowID: "checkout", AppArtifactDigest: "sha256:def", DeviceRef: "pixel-8-api34",
		},
	})
	assert.Equal(t, "checkout", mobile.MobileFlowID)
	assert.Equal(t, "sha256:def", mobile.MobileDigest)
	assert.Equal(t, "pixel-8-api34", mobile.MobileDeviceRef)
}

// A journey that ran no accessibility oracle must not show zero violations,
// which would read as a clean a11y result.
func TestA11yViewAbsentWithoutOracle(t *testing.T) {
	t.Parallel()
	assert.Nil(t, a11yView(nil))

	view := a11yView(&evidence.A11yOracle{CriticalCount: 2, SeriousCount: 3, FailedTargets: []string{"#pay"}})
	require.NotNil(t, view)
	assert.Equal(t, 2, view.CriticalCount)
	assert.Equal(t, 3, view.SeriousCount)
	assert.Equal(t, []string{"#pay"}, view.FailedTargets)
}
