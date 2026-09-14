package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two contracts for one SPEC is an unresolved decision, not a choice the
// runtime gets to make alphabetically.
func TestFindChangeContract_DuplicateContractsForOneSpecAreRefused(t *testing.T) {
	t.Run("spec directory and sibling", func(t *testing.T) {
		root, specDir := contractProject(t)
		writeContractAt(t, root, testContractSpecID, validTestContract().render())
		writeContractAt(t, root, "CHG-ONE-01", validTestContract().render())

		_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

		require.Error(t, err)
		assert.False(t, found)
		assert.Contains(t, err.Error(), "2 change contracts")
		assert.False(t, authorize(t, specDir, "pkg/a/x.go").Authorized)
	})

	t.Run("two siblings", func(t *testing.T) {
		root, specDir := contractProject(t)
		writeContractAt(t, root, "CHG-AAA-01", validTestContract().render())
		writeContractAt(t, root, "CHG-BBB-02", validTestContract().render())

		_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

		require.Error(t, err)
		assert.False(t, found)
	})
}

func TestFindChangeContract_SiblingForAnotherSpecIsNotFound(t *testing.T) {
	root, specDir := contractProject(t)
	other := validTestContract()
	other.specID, other.binding = "SPEC-OTHER-002", "SPEC-OTHER-002"
	writeContractAt(t, root, "CHG-OTHER-01", other.render())

	_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

	require.NoError(t, err)
	assert.False(t, found)
	assert.Contains(t, authorize(t, specDir).Reason, "no compact change contract")
}

func TestFindChangeContract_AbsentContractIsNotAnError(t *testing.T) {
	_, specDir := contractProject(t)

	_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

	require.NoError(t, err)
	assert.False(t, found)
}

// A document that cannot be trusted is refused with a named reason rather
// than read past. Every row here is a document that would otherwise have
// authorized a shortened run on incomplete evidence.
func TestFindChangeContract_MalformedDocumentsAreRefused(t *testing.T) {
	valid := validTestContract()
	mutated := func(mutate func(tc *testContract)) string {
		contract := valid
		mutate(&contract)
		return contract.render()
	}
	cases := map[string]string{
		"foreign schema":           "---\nschema: other.v1\nspec_id: " + testContractSpecID + "\n---\n",
		"unterminated header":      "---\nschema: " + ChangeContractSchema + "\n",
		"non key value line":       "---\nschema " + ChangeContractSchema + "\n---\n",
		"unknown tier":             mutated(func(tc *testContract) { tc.tier = "moderate" }),
		"unknown decision":         mutated(func(tc *testContract) { tc.decision = "probably_fine" }),
		"unknown class":            mutated(func(tc *testContract) { tc.declared = "smallish" }),
		"non boolean new contract": mutated(func(tc *testContract) { tc.newContract = "maybe" }),
		"duplicate header key":     mutated(func(tc *testContract) { tc.headerExtra = "risk_tier: high\n" }),
		"no acceptance criteria":   mutated(func(tc *testContract) { tc.acceptance = nil }),
		"no intended surface":      mutated(func(tc *testContract) { tc.surface = nil }),
		"no verification plan":     mutated(func(tc *testContract) { tc.verify = nil }),
		"body binds another spec":  mutated(func(tc *testContract) { tc.binding = "SPEC-OTHER-002" }),
		"surface climbs out": mutated(func(tc *testContract) {
			tc.surface = []string{"../secrets/key.pem"}
		}),
		"absolute surface entry": mutated(func(tc *testContract) {
			tc.surface = []string{"/etc/passwd"}
		}),
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			root, specDir := contractProject(t)
			writeContractAt(t, root, testContractSpecID, document)

			_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

			require.Error(t, err)
			assert.False(t, found)
			decision := authorize(t, specDir, "pkg/a/x.go")
			assert.False(t, decision.Authorized)
			assert.Contains(t, decision.Reason, "change contract unusable")
		})
	}
}

// A contract reached through a symlink is a contract whose contents live
// somewhere the SPEC directory does not govern.
func TestFindChangeContract_SymlinkedContractIsRefused(t *testing.T) {
	root, specDir := contractProject(t)
	target := filepath.Join(root, "elsewhere.md")
	require.NoError(t, os.WriteFile(target, []byte(validTestContract().render()), 0o600))
	if err := os.Symlink(target, filepath.Join(specDir, ChangeContractFile)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

	require.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "symlink")
}

func TestFindChangeContract_OversizedContractIsRefused(t *testing.T) {
	root, specDir := contractProject(t)
	document := validTestContract().render() + strings.Repeat("padding padding padding\n", 4000)
	writeContractAt(t, root, testContractSpecID, document)

	_, found, err := FindChangeContract(specDir, testContractSpecID, nil)

	require.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "byte limit")
}
