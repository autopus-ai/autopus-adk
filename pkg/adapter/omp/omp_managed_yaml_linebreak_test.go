package omp

// Security re-check N1 of SPEC-MODELUX-003: yaml.v3 also breaks lines at a
// lone CR, NEL, LS, and PS, which the managed editor's byte offsets do not
// count. A project config holding one is never edited: the first claim skips
// it, a claim already recorded is kept as it stands, and clean refuses to
// restore it, each leaving the file's bytes unchanged.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ompYAMLOnlyBreaks puts each break yaml.v3 counts, and the editor does not,
// inside a quoted value before a key.
var ompYAMLOnlyBreaks = map[string]string{
	"LS":       "note: \"a b\"\ntheme: dark\n",
	"PS":       "note: \"a b\"\ntheme: dark\n",
	"NEL":      "note: \"a\u0085b\"\ntheme: dark\n",
	"lone CR":  "note: \"a\rb\"\ntheme: dark\n",
	"bare LS":  "theme: dark other: 1\n",
	"bare NEL": "theme: dark\u0085other: 1\n",
}

func TestOMPClaimSet_FirstClaimSkipsAConfigWithAYAMLOnlyLineBreak(t *testing.T) {
	for name, content := range ompYAMLOnlyBreaks {
		t.Run(name, func(t *testing.T) {
			p := newClaimProject(t)
			p.write(configFile, content)

			p.mustRun("generate", configForOMP())
			p.mustRun("update", configForOMP())

			after, _ := p.read(configFile)
			assert.Equal(t, content, after)
			_, ledger := p.ledger()
			assert.False(t, ledger)
			report, err := InspectOMPAgentSurface(p.root, configForOMP())
			require.NoError(t, err)
			assert.True(t, report.ProjectConfigUnclaimable)
		})
	}
}

// A recorded claim over a file the user then gave such a break is kept as it
// stands, in autopus and native mode, until the file can be read again.
func TestOMPClaimSet_KeepsARecordedClaimOverAYAMLOnlyLineBreak(t *testing.T) {
	for name, content := range ompYAMLOnlyBreaks {
		t.Run(name, func(t *testing.T) {
			p := newClaimProject(t)
			p.write(configFile, "theme: dark\n")
			p.mustRun("generate", configForOMP())
			claimed, _ := p.read(configFile)
			ledgerBefore, _ := p.read(OMPModelProjectOwnershipRelativePath)
			edited := content + claimed[len("theme: dark\n"):]
			p.write(configFile, edited)

			for _, run := range []struct {
				operation string
				native    bool
			}{{"generate", false}, {"update", false}, {"update", true}, {"generate", true}} {
				cfg := configForOMP()
				if run.native {
					cfg = nativeConfigForOMP()
				}
				p.mustRun(run.operation, cfg)
				after, _ := p.read(configFile)
				assert.Equal(t, edited, after, "%s native=%v", run.operation, run.native)
				ledgerAfter, ok := p.read(OMPModelProjectOwnershipRelativePath)
				assert.True(t, ok)
				assert.Equal(t, ledgerBefore, ledgerAfter)
			}
			report, err := InspectOMPAgentSurface(p.root, configForOMP())
			require.NoError(t, err)
			assert.True(t, report.ProjectConfigUnclaimable)
			assert.True(t, report.DisabledAgentsClaimed)

			p.write(configFile, claimed)
			p.mustRun("generate", nativeConfigForOMP())
			released, _ := p.read(configFile)
			assert.Equal(t, "theme: dark\n", released, "the claim is released once the file reads again")
		})
	}
}

// Clean cannot restore what it cannot read: it refuses and changes nothing.
func TestOMPClean_RefusesAClaimedConfigWithAYAMLOnlyLineBreak(t *testing.T) {
	p := newClaimProject(t)
	p.write(configFile, "theme: dark\n")
	p.mustRun("generate", configForOMP())
	claimed, _ := p.read(configFile)
	edited := ompYAMLOnlyBreaks["LS"] + claimed[len("theme: dark\n"):]
	p.write(configFile, edited)
	before := claimSnapshot(t, p)

	err := p.adapter().Clean(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_config_unclaimable")
	assert.Equal(t, before, claimSnapshot(t, p))
}
