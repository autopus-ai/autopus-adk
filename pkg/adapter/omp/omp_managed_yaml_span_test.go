package omp

// Security review F1/F2 of SPEC-MODELUX-003: a managed key's entry is the
// key's lines up to the next key of its mapping in the node tree, so an
// indentless sequence (`key:\n- item`, the PyYAML, yaml.v2, and kubectl
// style) and four-space layouts stay whole when ADK replaces or removes the
// key.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestRemoveOMPManagedPath_KeepsEveryLayoutWhole(t *testing.T) {
	for name, row := range map[string]struct{ in, want string }{
		"indentless between root keys": {
			"theme: dark\ntask:\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\nother: 2\n",
			"theme: dark\nother: 2\n",
		},
		"indentless after a sibling": {
			"task:\n  enabled: true\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\n",
			"task:\n  enabled: true\n",
		},
		"indentless before a sibling": {
			"task:\n  disabledAgents:\n  - scout\n  - sonic\n  keep: yes\n",
			"task:\n  keep: yes\n",
		},
		"comment between indentless items": {
			"task:\n  disabledAgents:\n  - scout\n  # mine\n  - sonic\n  keep: yes\n",
			"task:\n  keep: yes\n",
		},
		"comment of the next sibling stays": {
			"task:\n  disabledAgents:\n  - scout\n  # about keep\n  keep: yes\n",
			"task:\n  # about keep\n  keep: yes\n",
		},
		"comment after the block stays": {
			"task:\n  disabledAgents:\n  - scout\n# tail\nother: 1\n",
			"# tail\nother: 1\n",
		},
		"four-space indents": {
			"task:\n    disabledAgents:\n        - scout\n        - sonic\n    keep: yes\n",
			"task:\n    keep: yes\n",
		},
		"four-space indentless": {
			"task:\n    disabledAgents:\n    - scout\n    - sonic\n    keep: yes\n",
			"task:\n    keep: yes\n",
		},
		"blank line before the next root key": {
			"task:\n  disabledAgents:\n    - scout\n\nother: 1\n",
			"other: 1\n",
		},
		"flow value": {
			"task:\n  disabledAgents: [scout, security-reviewer, sonic] # mine\n  keep: yes\n",
			"task:\n  keep: yes\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := removeOMPManagedPath([]byte(row.in), ompDisabledAgentsPath)

			require.NoError(t, err)
			assert.Equal(t, row.want, string(got))
		})
	}
}

// F2: the first claim over a default set in indentless form, and every
// replace or insert, renders the key canonically and keeps the rest.
func TestSetOMPDisabledAgents_RendersTheKeyInEveryLayout(t *testing.T) {
	for name, row := range map[string]struct{ in, want string }{
		"indentless default": {
			"task:\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\n",
			claimSurfaceOnly,
		},
		"indentless value before a sibling": {
			"task:\n  disabledAgents:\n  - browser\n  keep: yes\n",
			"task:\n  disabledAgents:\n    - scout\n    - security-reviewer\n    - sonic\n  keep: yes\n",
		},
		"insert into a four-space mapping": {
			"theme: dark\ntask:\n    keep: yes\n",
			"theme: dark\ntask:\n    keep: yes\n    disabledAgents:\n      - scout\n      - security-reviewer\n      - sonic\n",
		},
		"insert after an indentless sibling": {
			"task:\n  hidden:\n  - browser\n",
			"task:\n  hidden:\n  - browser\n  disabledAgents:\n    - scout\n    - security-reviewer\n    - sonic\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := setOMPDisabledAgents([]byte(row.in), config.OMPAutopusDisabledAgents(), true)

			require.NoError(t, err)
			assert.Equal(t, row.want, string(got))
		})
	}
}

// The release preimage of a reformatted claim drops only the key.
func TestApplyOMPAgentSurfacePrior_RemovesAnIndentlessClaim(t *testing.T) {
	in := "theme: dark\ntask:\n  disabledAgents:\n  - scout\n  - security-reviewer\n  - sonic\nother: 2\n"

	got, err := applyOMPAgentSurfacePrior([]byte(in), OMPMissingManagedValueFingerprint())

	require.NoError(t, err)
	assert.Equal(t, "theme: dark\nother: 2\n", string(got))
}
