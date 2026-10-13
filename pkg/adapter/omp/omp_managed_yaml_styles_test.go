package omp

// Security review F1/F2 of SPEC-MODELUX-003 through the adapter: release,
// clean, first claim, and the canonical anchor keep a user's indentless or
// four-space layout valid and its other keys intact.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	claimIndentless      = "theme: dark\ntask:\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\n"
	claimIndentlessFirst = "task:\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\n"
	claimFourSpace       = "theme: dark\ntask:\n    keep: yes\n"
)

func claimYAML(t *testing.T, data string) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(data), &document), data)
	return document
}

// The user rewrote ADK's claim as an indentless sequence; going native
// through Generate or Update releases exactly that key.
func TestOMPClaimSet_ReleasesAnIndentlessClaim(t *testing.T) {
	for _, operation := range []string{"generate", "update"} {
		t.Run(operation, func(t *testing.T) {
			p := newClaimProject(t)
			p.write(configFile, "theme: dark\n")
			p.mustRun(operation, configForOMP())
			p.write(configFile, claimIndentless)

			p.mustRun(operation, nativeConfigForOMP())

			after, _ := p.read(configFile)
			assert.Equal(t, "theme: dark\n", after)
			_, ledger := p.ledger()
			assert.False(t, ledger)
		})
	}
}

// An outside edit next to the reformatted claim stays when it is released.
func TestOMPClaimSet_ReleasesAnIndentlessClaimBesideAUserKey(t *testing.T) {
	p := newClaimProject(t)
	p.write(configFile, "theme: dark\n")
	p.mustRun("generate", configForOMP())
	p.write(configFile, "theme: dark\ntask:\n  enabled: true\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\n")

	p.mustRun("generate", nativeConfigForOMP())

	after, _ := p.read(configFile)
	assert.Equal(t, "theme: dark\ntask:\n  enabled: true\n", after)
}

// Autopus mode keeps claiming a reformatted claim beside an outside edit.
func TestOMPClaimSet_KeepsClaimingAnIndentlessClaim(t *testing.T) {
	p := newClaimProject(t)
	p.write(configFile, "theme: dark\n")
	p.mustRun("generate", configForOMP())
	p.write(configFile, claimIndentless+"extra: 1\n")

	p.mustRun("update", configForOMP())

	after, _ := p.read(configFile)
	document := claimYAML(t, after)
	assert.Equal(t, "dark", document["theme"])
	assert.Equal(t, 1, document["extra"])
	assert.Equal(t, []any{"scout", "security-reviewer", "sonic"}, claimDisabledAgents(t, after))
}

// Clean restores the preimage of a reformatted claim.
func TestOMPClaimSet_CleanReleasesAnIndentlessClaim(t *testing.T) {
	p := newClaimProject(t)
	p.write(configFile, "theme: dark\n")
	p.mustRun("generate", configForOMP())
	p.write(configFile, claimIndentless)

	require.NoError(t, p.adapter().Clean(context.Background()))

	after, _ := p.read(configFile)
	assert.Equal(t, "theme: dark\n", after)
}

// F2: the first claim over the default set in indentless form writes valid
// YAML with the same value, and going native keeps the key.
func TestOMPClaimSet_FirstClaimOverAnIndentlessDefault(t *testing.T) {
	p := newClaimProject(t)
	p.write(configFile, claimIndentlessFirst)

	p.mustRun("generate", configForOMP())

	claimed, _ := p.read(configFile)
	assert.Equal(t, []any{"scout", "security-reviewer", "sonic"}, claimDisabledAgents(t, claimed))
	p.mustRun("generate", nativeConfigForOMP())
	released, _ := p.read(configFile)
	assert.Equal(t, []any{"scout", "security-reviewer", "sonic"}, claimDisabledAgents(t, released))
}

// A four-space project gets the claim inside its own mapping and gets its
// bytes back on release.
func TestOMPClaimSet_FourSpaceLayoutRoundTrips(t *testing.T) {
	p := newClaimProject(t)
	p.write(configFile, claimFourSpace)

	p.mustRun("generate", configForOMP())

	claimed, _ := p.read(configFile)
	assert.Equal(t, claimFourSpace+"    disabledAgents:\n      - scout\n      - security-reviewer\n      - sonic\n", claimed)
	p.mustRun("generate", nativeConfigForOMP())
	released, _ := p.read(configFile)
	assert.Equal(t, claimFourSpace, released)
}

// The canonical anchor reads an edit of the claim alone in either layout:
// the value is the user's from then on, and release leaves it.
func TestOMPClaimSet_AnchorReadsAnIndentlessEditOfTheClaimAlone(t *testing.T) {
	for name, row := range map[string]struct{ start, edited string }{
		"two-space":  {"theme: dark\n", "theme: dark\ntask:\n  disabledAgents:\n  - browser\n"},
		"four-space": {claimFourSpace, claimFourSpace + "    disabledAgents:\n    - browser\n"},
	} {
		t.Run(name, func(t *testing.T) {
			p := newClaimProject(t)
			p.write(configFile, row.start)
			p.mustRun("generate", configForOMP())
			p.write(configFile, row.edited)

			p.mustRun("generate", configForOMP())

			ownership, ledger := p.ledger()
			require.True(t, ledger)
			assert.True(t, ownership.AgentSurfaceUserOwned)
			after, _ := p.read(configFile)
			assert.Equal(t, row.edited, after)
			p.mustRun("generate", nativeConfigForOMP())
			released, _ := p.read(configFile)
			assert.Equal(t, row.edited, released)
		})
	}
}
