package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/capture"
	"github.com/insajin/autopus-adk/pkg/qa/journey"
)

func matrixIndex(steps ...capture.Step) capture.Index {
	return capture.Index{Steps: steps}
}

// Guards the empty-matrix short circuit: with nothing declared the oracle must
// report no gaps rather than inventing findings from observed steps.
func TestEvaluateCaptureScreenMatrixEmptyDeclaration(t *testing.T) {
	t.Parallel()

	screens, actions := evaluateCaptureScreenMatrix(
		matrixIndex(capture.Step{ScreenRef: "home", Actions: []capture.Action{{API: "click"}}}),
		nil,
	)
	assert.Empty(t, screens)
	assert.Empty(t, actions)
}

// Guards the core gap report: a declared screen with no captured evidence is a
// missing screen, and a captured screen missing a required action is reported
// as a missing action rather than being folded into the screen gap.
func TestEvaluateCaptureScreenMatrixReportsGaps(t *testing.T) {
	t.Parallel()

	index := matrixIndex(
		capture.Step{ScreenRef: "home", Actions: []capture.Action{{API: "click"}}},
	)
	rows := []journey.GUIScreenMatrixRow{
		{ID: "home", RequiredActions: []string{"click", "fill"}},
		{ID: "settings", RequiredActions: []string{"click"}},
	}

	screens, actions := evaluateCaptureScreenMatrix(index, rows)
	assert.Equal(t, []string{"settings"}, screens)
	assert.Equal(t, []string{"home:fill"}, actions)
}

// Guards the normalization rules the oracle relies on: action names compare
// case-insensitively and trimmed, so producer whitespace or casing cannot
// fabricate a gap, and blank screen refs are ignored instead of keyed as "".
func TestEvaluateCaptureScreenMatrixNormalizesNames(t *testing.T) {
	t.Parallel()

	index := matrixIndex(
		capture.Step{ScreenRef: "   ", Actions: []capture.Action{{API: "click"}}},
		capture.Step{ScreenRef: " home ", Actions: []capture.Action{{API: "  CLICK "}}},
	)
	rows := []journey.GUIScreenMatrixRow{{ID: " home ", RequiredActions: []string{"Click"}}}

	screens, actions := evaluateCaptureScreenMatrix(index, rows)
	assert.Empty(t, screens, "trimmed screen ref must match the trimmed declaration")
	assert.Empty(t, actions)

	// The blank-ref step must not register a screen keyed by the empty string.
	blankRows := []journey.GUIScreenMatrixRow{{ID: ""}}
	blankScreens, _ := evaluateCaptureScreenMatrix(index, blankRows)
	assert.Equal(t, []string{""}, blankScreens)
}

// Guards the ID/Path precedence: Path is only the fallback key, so a row with
// both must be matched by ID and not silently satisfied by a path-keyed step.
func TestEvaluateCaptureScreenMatrixPathFallback(t *testing.T) {
	t.Parallel()

	index := matrixIndex(capture.Step{ScreenRef: "/checkout"})

	byPath, _ := evaluateCaptureScreenMatrix(
		index, []journey.GUIScreenMatrixRow{{Path: "/checkout"}},
	)
	assert.Empty(t, byPath)

	byID, _ := evaluateCaptureScreenMatrix(
		index, []journey.GUIScreenMatrixRow{{ID: "checkout", Path: "/checkout"}},
	)
	assert.Equal(t, []string{"checkout"}, byID)
}

// Guards that evidence for one screen spread across several steps accumulates
// instead of the last step overwriting earlier observed actions.
func TestEvaluateCaptureScreenMatrixAccumulatesAcrossSteps(t *testing.T) {
	t.Parallel()

	index := matrixIndex(
		capture.Step{ScreenRef: "home", Actions: []capture.Action{{API: "click"}}},
		capture.Step{ScreenRef: "home", Actions: []capture.Action{{API: "fill"}}},
	)

	screens, actions := evaluateCaptureScreenMatrix(
		index, []journey.GUIScreenMatrixRow{{ID: "home", RequiredActions: []string{"click", "fill"}}},
	)
	require.Empty(t, screens)
	assert.Empty(t, actions)
}

// Guards the blocked-request key used in the guard receipt summary: a blank
// origin must degrade to an explicit marker so findings never read as ":".
func TestOrUnknownOriginPlaceholder(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unresolved", orUnknownOrigin(""))
	assert.Equal(t, "unresolved", orUnknownOrigin("   "))
	assert.Equal(t, "https://example.test", orUnknownOrigin("https://example.test"))
}
