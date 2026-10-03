package triage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// Review finding 6: a TypeError raised in any path the diff guard treats as a
// test is a test defect, and an ancestor directory outside the project that
// happens to be named "test" does not make product code a test.
func TestClassify_TestDefectUsesTheSharedTestPathPredicate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, text, project string
		class               Class
	}{
		{"helper under tests/", "TypeError: session is undefined\n    at login (tests/helpers/session.ts:4:2)\n", "", ClassTestDefect},
		{"support under e2e/", "TypeError: cy is undefined\n    at e2e/support/commands.ts:9:3\n", "", ClassTestDefect},
		{"pytest helper module", "E       TypeError: 'NoneType' object is not callable\ntests/helpers.py:12: TypeError\n", "", ClassTestDefect},
		{"absolute path inside the project", "TypeError: x is undefined\n    at /work/shop/tests/helpers/session.ts:4:2\n", "/work/shop", ClassTestDefect},
		{"ancestor named test is not a test dir", "TypeError: total is not a function\n    at Object.total (/home/ci/test/shop/src/cart.ts:8:10)\n", "/home/ci/test/shop", ClassUnknown},
		{"absolute path outside the project", "TypeError: total is not a function\n    at /opt/test/lib/cart.ts:8:10\n", "/work/shop", ClassUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verdict := Classify(Input{JourneyID: "cart", FailureText: tc.text, ProjectDir: tc.project})
			assert.Equal(t, tc.class, verdict.Class, verdict.Signal)
		})
	}
}

// Review finding 7: a second, different failure on the same screen is
// progress, not the same failure again.
func TestVerdictFingerprint_SeparatesStepsAndSpecs(t *testing.T) {
	t.Parallel()
	base := Verdict{JourneyID: "login", Class: ClassTestDrift, SpecPath: "e2e/autopus-generated/login.spec.ts", Line: 29,
		Step: &scenario.StepRef{Screen: "sign-in", Index: 0, Kind: scenario.StepKindAction}}
	nextStep := base
	nextStep.Step = &scenario.StepRef{Screen: "sign-in", Index: 2, Kind: scenario.StepKindAction}
	otherSpec := base
	otherSpec.SpecPath = "e2e/autopus-generated/login-mobile.spec.ts"
	same := base
	same.Line, same.Signal = 41, "stepmap:action reworded"

	assert.NotEqual(t, base.Fingerprint(), nextStep.Fingerprint(), "a different step index is a different failure")
	assert.NotEqual(t, base.Fingerprint(), otherSpec.Fingerprint(), "a different spec is a different failure")
	assert.Equal(t, base.Fingerprint(), same.Fingerprint(), "a moved line or reworded signal is the same failure")
	assert.Contains(t, base.Fingerprint(), "login|test_drift|")
}
