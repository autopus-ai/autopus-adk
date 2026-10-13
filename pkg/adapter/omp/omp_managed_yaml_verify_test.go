package omp

// Security review F1/F2 of SPEC-MODELUX-003, defense in depth: a managed
// edit is re-read before it can be written, and any splice that changes more
// than the managed keys, or stops parsing, is managed_key_conflict.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyOMPManagedEdit_RefusesEverythingButTheEdit(t *testing.T) {
	remove := []ompManagedEdit{{path: ompDisabledAgentsPath, remove: true}}
	set := func(value ...string) []ompManagedEdit {
		return []ompManagedEdit{{path: ompDisabledAgentsPath, value: value}}
	}
	for name, row := range map[string]struct {
		before, after string
		edits         []ompManagedEdit
		ok            bool
	}{
		"removal that kept a bare sequence": {
			claimIndentless + "other: 2\n", "theme: dark\ntask:\n  - scout\n  - security-reviewer\n  - sonic\nother: 2\n", remove, false,
		},
		"removal that no longer parses": {
			"task:\n  enabled: true\n  disabledAgents:\n  - scout\n", "task:\n  enabled: true\n  - scout\n", remove, false,
		},
		"removal that changed a sibling": {
			"theme: dark\ntask:\n  disabledAgents: [a]\n", "theme: light\n", remove, false,
		},
		"removal with its empty parent": {"theme: dark\ntask:\n  disabledAgents: [a]\n", "theme: dark\n", remove, true},
		"removal beside a kept key":     {"task:\n  keep: yes\n  disabledAgents: [a]\n", "task:\n  keep: yes\n", remove, true},
		"insert":                        {"theme: dark\n", "theme: dark\ntask:\n  disabledAgents:\n    - scout\n", set("scout"), true},
		"insert of another value":       {"theme: dark\n", "theme: dark\ntask:\n  disabledAgents:\n    - scout\n", set("sonic"), false},
		"replace that doubled the value": {
			claimIndentlessFirst, "task:\n  disabledAgents:\n    - scout\n    - security-reviewer\n    - sonic\n  - scout\n", set("scout", "security-reviewer", "sonic"), false,
		},
		"second document": {"theme: dark\n", "theme: dark\n---\nother: 1\n", remove, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := verifyOMPManagedEdit([]byte(row.before), []byte(row.after), row.edits)

			if row.ok {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "managed_key_conflict")
		})
	}
}
