package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A bare path matches any query or hash the app appends; a path that states
// its own query is matched exactly. v1 output is pinned by the golden tests.
func TestCompileV2_URLStepsMatchBarePathsLoosely(t *testing.T) {
	s := Scenario{
		SchemaVersion: SchemaVersionV2, ID: "nav", Title: "Nav", Journey: "web",
		IntentSource: IntentAcceptance, Spec: "SPEC-NAV-001", AcceptanceRef: []string{"AC-NAV-001"},
		Origin: "http://127.0.0.1:4173", Path: "nav.yaml",
		Screens: []Screen{{ID: "home", Path: "/", Steps: []Step{
			{Click: &Target{Role: "link", Name: "Welcome"}},
			{WaitURL: "/welcome"},
			{ExpectURL: "/welcome", Ac: "AC-NAV-001"},
			{ExpectURL: "/search?q=a", Ac: "AC-NAV-001"},
		}}},
	}
	out, _, err := CompileWithMap(s, Options{FixtureImport: "../fixture.cjs"})
	require.NoError(t, err)
	spec := string(out)
	assert.Contains(t, spec, "function onPath(path: string): RegExp {")
	assert.Contains(t, spec, `await page.waitForURL(onPath("/welcome"));`)
	assert.Contains(t, spec, `await expect(page).toHaveURL(onPath("/welcome"));`)
	assert.Contains(t, spec, `await expect(page).toHaveURL(ORIGIN + "/search?q=a");`)
}

func TestCompileV2_NoURLStepsOmitsHelper(t *testing.T) {
	s := Scenario{
		SchemaVersion: SchemaVersionV2, ID: "home", Title: "Home", Journey: "web",
		IntentSource: IntentBaseline, Origin: "http://127.0.0.1:4173", Path: "home.yaml",
		Screens: []Screen{{ID: "home", Path: "/", Steps: []Step{{ExpectTitle: "Home"}}}},
	}
	out, _, err := CompileWithMap(s, Options{FixtureImport: "../fixture.cjs"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "onPath")
}
